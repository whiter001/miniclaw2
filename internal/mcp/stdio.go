package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"miniclaw2/internal/config"
)

type stdioServer struct {
	name      string
	command   string
	args      []string
	env       map[string]string
	cmd       *exec.Cmd
	stdin     ioWriteCloser
	incoming  chan map[string]any
	errCh     chan error
	tools     []Tool
	requestID int
	mu        sync.Mutex
}

type ioWriteCloser interface {
	Write([]byte) (int, error)
	Close() error
}

type rawMCPConfig struct {
	Servers map[string]rawMCPServer   `json:"servers"`
	Tools   map[string]rawCommandTool `json:"tools"`
}

type rawMCPServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

type rawCommandTool struct {
	Type              string            `json:"type"`
	Description       string            `json:"description"`
	Command           string            `json:"command"`
	Args              []string          `json:"args"`
	Env               map[string]string `json:"env"`
	InputSchema       map[string]any    `json:"input_schema"`
	InputSchemaAlt    map[string]any    `json:"inputSchema"`
	TimeoutSeconds    int               `json:"timeout_seconds"`
	TimeoutSecondsAlt int               `json:"timeoutSeconds"`
}

type serverConfig struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

func loadAndStartStdioServers(cfg config.Config) []*stdioServer {
	configs := loadServerConfigs(cfg)
	servers := make([]*stdioServer, 0, len(configs))
	for _, serverCfg := range configs {
		server, err := startStdioServer(serverCfg)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		if err := server.initialize(ctx); err != nil {
			cancel()
			server.close()
			continue
		}
		tools, err := server.listTools(ctx)
		cancel()
		if err != nil {
			server.close()
			continue
		}
		server.tools = tools
		servers = append(servers, server)
	}
	return servers
}

func loadServerConfigs(cfg config.Config) []serverConfig {
	raw := loadRawMCPConfig(cfg)
	return loadServerConfigsFromRaw(raw)
}

func loadServerConfigsFromRaw(raw rawMCPConfig) []serverConfig {
	configs := make([]serverConfig, 0, len(raw.Servers))
	for name, server := range raw.Servers {
		if server.Type != "stdio" || strings.TrimSpace(server.Command) == "" {
			continue
		}
		args := append([]string(nil), server.Args...)
		commandBase := strings.ToLower(filepathBase(server.Command))
		if commandBase == "npx" || commandBase == "npx.cmd" {
			hasYes := false
			for _, arg := range args {
				if arg == "-y" || arg == "--yes" {
					hasYes = true
					break
				}
			}
			if !hasYes {
				args = append([]string{"-y"}, args...)
			}
		}
		configs = append(configs, serverConfig{Name: name, Command: server.Command, Args: args, Env: server.Env})
	}
	return configs
}

func loadRawMCPConfig(cfg config.Config) rawMCPConfig {
	if strings.TrimSpace(cfg.MCPConfigPath) == "" {
		return rawMCPConfig{}
	}
	data, err := os.ReadFile(cfg.MCPConfigPath)
	if err != nil {
		return rawMCPConfig{}
	}
	var raw rawMCPConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return rawMCPConfig{}
	}
	return raw
}

func (r rawCommandTool) normalizedInputSchema() map[string]any {
	if len(r.InputSchema) > 0 {
		return r.InputSchema
	}
	return r.InputSchemaAlt
}

func (r rawCommandTool) normalizedTimeoutSeconds() int {
	if r.TimeoutSeconds > 0 {
		return r.TimeoutSeconds
	}
	return r.TimeoutSecondsAlt
}

func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func startStdioServer(cfg serverConfig) (*stdioServer, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	cmd.Env = os.Environ()
	for key, value := range cfg.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	server := &stdioServer{
		name:     cfg.Name,
		command:  cfg.Command,
		args:     append([]string(nil), cfg.Args...),
		env:      cfg.Env,
		cmd:      cmd,
		stdin:    stdin,
		incoming: make(chan map[string]any, 32),
		errCh:    make(chan error, 2),
	}
	go server.readLoop(stdout)
	go server.readLoop(stderr)
	return server, nil
}

func (s *stdioServer) readLoop(reader interface{ Read([]byte) (int, error) }) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			continue
		}
		if method, ok := message["method"].(string); ok && method == "roots/list" {
			id := numericID(message["id"])
			_ = s.sendRaw(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"roots": []any{}}})
			continue
		}
		s.incoming <- message
	}
	if err := scanner.Err(); err != nil {
		s.errCh <- err
	}
}

func (s *stdioServer) initialize(ctx context.Context) error {
	_, err := s.sendRequest(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{"roots": map[string]any{"listChanged": true}},
		"clientInfo":      map[string]any{"name": "miniclaw-go", "version": "v0.1.0"},
	})
	if err != nil {
		return err
	}
	return s.sendNotification("notifications/initialized", map[string]any{})
}

func (s *stdioServer) listTools(ctx context.Context) ([]Tool, error) {
	response, err := s.sendRequest(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid MCP tools/list response")
	}
	rawTools, ok := result["tools"].([]any)
	if !ok {
		return nil, nil
	}
	toolsList := make([]Tool, 0, len(rawTools))
	for _, rawTool := range rawTools {
		toolMap, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		name, _ := toolMap["name"].(string)
		description, _ := toolMap["description"].(string)
		inputSchema, _ := toolMap["inputSchema"].(map[string]any)
		if strings.TrimSpace(name) == "" {
			continue
		}
		toolsList = append(toolsList, Tool{Name: name, Description: description, InputSchema: inputSchema})
	}
	return toolsList, nil
}

func (s *stdioServer) callTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	response, err := s.sendRequest(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return "", err
	}
	if errValue, ok := response["error"].(map[string]any); ok {
		message, _ := errValue["message"].(string)
		if strings.TrimSpace(message) == "" {
			message = "MCP call failed"
		}
		return "", fmt.Errorf("%s", message)
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid MCP call response")
	}
	if text, ok := result["text"].(string); ok && strings.TrimSpace(text) != "" {
		return text, nil
	}
	content, ok := result["content"].([]any)
	if !ok {
		return "(empty result)", nil
	}
	parts := []string{}
	for _, rawBlock := range content {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		if block["type"] == "text" {
			if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
	}
	if len(parts) == 0 {
		return "(empty result)", nil
	}
	return strings.Join(parts, "\n"), nil
}

func (s *stdioServer) hasTool(name string) bool {
	for _, tool := range s.tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func (s *stdioServer) sendRequest(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestID++
	id := s.requestID
	request := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	if err := s.sendRaw(request); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-s.errCh:
			return nil, err
		case message := <-s.incoming:
			if numericID(message["id"]) == id {
				return message, nil
			}
		}
	}
}

func (s *stdioServer) sendNotification(method string, params map[string]any) error {
	request := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		request["params"] = params
	}
	return s.sendRaw(request)
}

func (s *stdioServer) sendRaw(value map[string]any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.stdin.Write(append(data, '\n'))
	return err
}

func (s *stdioServer) close() {
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
}

func numericID(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		return 0
	}
}
