package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/tools"
)

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Manager struct {
	cfg          config.Config
	client       *http.Client
	builtinTools []Tool
	commandTools []*commandTool
	servers      []*stdioServer
}

func NewManager(cfg config.Config) *Manager {
	manager := &Manager{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeout) * time.Second,
		},
	}
	if cfg.EnableMCP {
		manager.builtinTools = builtinTools()
		manager.commandTools = loadCommandTools(cfg)
		manager.servers = loadAndStartStdioServers(cfg)
	}
	return manager
}

func (m *Manager) StopAll() {
	for _, server := range m.servers {
		server.close()
	}
}

func (m *Manager) Tools() []Tool {
	copied := make([]Tool, 0, len(m.builtinTools)+len(m.commandTools)+len(m.servers)*2)
	copied = append(copied, m.builtinTools...)
	for _, tool := range m.commandTools {
		copied = append(copied, tool.tool)
	}
	for _, server := range m.servers {
		copied = append(copied, server.tools...)
	}
	return copied
}

func (m *Manager) HasTool(name string) bool {
	for _, tool := range m.Tools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func (m *Manager) CallTool(ctx context.Context, name string, input map[string]any) (string, error) {
	switch name {
	case "web_search":
		return m.callWebSearch(ctx, input)
	case "understand_image":
		return m.callUnderstandImage(ctx, input)
	default:
		for _, tool := range m.commandTools {
			if tool.tool.Name == name {
				return tool.call(ctx, m.cfg, input)
			}
		}
		for _, server := range m.servers {
			if server.hasTool(name) {
				return server.callTool(ctx, name, input)
			}
		}
		return "", fmt.Errorf("MCP tool %q not found", name)
	}
}

func builtinTools() []Tool {
	return []Tool{
		{
			Name:        "web_search",
			Description: "Perform web searches and return structured organic results with related search queries.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string", "description": "The search query. Aim for 3-5 keywords for best results."}}, "required": []string{"query"}},
		},
		{
			Name:        "understand_image",
			Description: "Analyze an image from a URL, data URL, or local file path based on a text prompt.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "description": "A text prompt describing what to analyze or extract from the image."}, "image_source": map[string]any{"type": "string", "description": "HTTP/HTTPS URL, data URL, relative local path, or absolute local path to the image."}, "image_url": map[string]any{"type": "string", "description": "Legacy alias for image_source."}, "path": map[string]any{"type": "string", "description": "Legacy alias for a local image path."}}, "required": []string{"prompt", "image_source"}},
		},
	}
}

func (m *Manager) callWebSearch(ctx context.Context, input map[string]any) (string, error) {
	query := strings.TrimSpace(toString(input["query"]))
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	response, err := m.callCodingPlanAPI(ctx, "/v1/coding_plan/search", map[string]any{"q": query})
	if err != nil {
		return "", err
	}
	formatted, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

func (m *Manager) callCodingPlanAPI(ctx context.Context, endpoint string, payload any) (map[string]any, error) {
	if strings.TrimSpace(m.cfg.APIKey) == "" {
		return nil, fmt.Errorf("MINICLAW_API_KEY is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	requestURL := deriveCodingPlanAPIHost(m.cfg.BaseURL) + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MM-API-Source", "Minimax-MCP")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("request failed: status %d%s: %s", resp.StatusCode, formatTraceID(resp.Header.Get("Trace-Id")), strings.TrimSpace(string(responseBody)))
	}
	var parsed map[string]any
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil, err
	}
	if err := validateCodingPlanResponse(parsed, resp.Header.Get("Trace-Id")); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (m *Manager) callUnderstandImage(ctx context.Context, input map[string]any) (string, error) {
	prompt := strings.TrimSpace(toString(input["prompt"]))
	imageSource := firstNonEmpty(
		strings.TrimSpace(toString(input["image_source"])),
		strings.TrimSpace(toString(input["image_url"])),
		strings.TrimSpace(toString(input["path"])),
	)
	if prompt == "" {
		prompt = "Please describe this image and highlight the most important visual details."
	}
	if imageSource == "" {
		return "", fmt.Errorf("image source is required")
	}
	processedImageURL, err := m.processImageSource(ctx, imageSource)
	if err != nil {
		return "", err
	}
	response, err := m.callCodingPlanAPI(ctx, "/v1/coding_plan/vlm", map[string]any{
		"prompt":    prompt,
		"image_url": processedImageURL,
	})
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(toString(response["content"]))
	if content == "" {
		return "", fmt.Errorf("No content returned from VLM API")
	}
	return content, nil
}

func (m *Manager) processImageSource(ctx context.Context, imageSource string) (string, error) {
	trimmed := strings.TrimSpace(imageSource)
	if strings.HasPrefix(trimmed, "@") {
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	if trimmed == "" {
		return "", fmt.Errorf("image source is required")
	}
	if strings.HasPrefix(trimmed, "data:") {
		return trimmed, nil
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return m.downloadImageAsDataURL(ctx, trimmed)
	}
	return m.readLocalImageAsDataURL(trimmed)
}

func (m *Manager) downloadImageAsDataURL(ctx context.Context, imageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Failed to download image from URL: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Failed to download image from URL: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("Failed to download image from URL: %v", err)
	}
	format := detectImageFormatFromContentType(resp.Header.Get("Content-Type"))
	return buildImageDataURL(format, data), nil
}

func (m *Manager) readLocalImageAsDataURL(imagePath string) (string, error) {
	resolved := imagePath
	if !filepath.IsAbs(imagePath) {
		var err error
		resolved, err = tools.ResolveWorkspacePath(m.cfg.Workspace, imagePath)
		if err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(resolved); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("Local image file does not exist: %s", imagePath)
		}
		return "", fmt.Errorf("Failed to read local image file: %v", err)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("Failed to read local image file: %v", err)
	}
	format := detectImageFormatFromPath(resolved)
	return buildImageDataURL(format, data), nil
}

func deriveCodingPlanAPIHost(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}
	return parsed.Scheme + "://" + parsed.Host
}

func validateCodingPlanResponse(response map[string]any, traceID string) error {
	baseResp, _ := response["base_resp"].(map[string]any)
	statusCode := jsonInt(baseResp["status_code"])
	if statusCode == 0 {
		return nil
	}
	statusMsg := strings.TrimSpace(toString(baseResp["status_msg"]))
	suffix := formatTraceID(traceID)
	switch statusCode {
	case 1004:
		return fmt.Errorf("API Error: %s, please check your API key and API host.%s", statusMsg, suffix)
	case 2038:
		return fmt.Errorf("API Error: %s, should complete real-name verification on the open-platform(https://platform.minimaxi.com/user-center/basic-information).%s", statusMsg, suffix)
	default:
		return fmt.Errorf("API Error: %d-%s%s", statusCode, statusMsg, suffix)
	}
}

func jsonInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func formatTraceID(traceID string) string {
	trimmed := strings.TrimSpace(traceID)
	if trimmed == "" {
		return ""
	}
	return " Trace-Id: " + trimmed
}

func detectImageFormatFromContentType(contentType string) string {
	lower := strings.ToLower(contentType)
	switch {
	case strings.Contains(lower, "jpeg"), strings.Contains(lower, "jpg"):
		return "jpeg"
	case strings.Contains(lower, "png"):
		return "png"
	case strings.Contains(lower, "webp"):
		return "webp"
	default:
		return "jpeg"
	}
}

func detectImageFormatFromPath(imagePath string) string {
	switch strings.ToLower(filepath.Ext(imagePath)) {
	case ".png":
		return "png"
	case ".webp":
		return "webp"
	case ".jpg", ".jpeg":
		return "jpeg"
	default:
		return "jpeg"
	}
}

func buildImageDataURL(imageFormat string, data []byte) string {
	return fmt.Sprintf("data:image/%s;base64,%s", imageFormat, base64.StdEncoding.EncodeToString(data))
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case json.Number:
		return typed.String()
	case nil:
		return ""
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
