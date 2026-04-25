package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"miniclaw2/internal/config"
)

func TestLoadCommandToolsFromRaw(t *testing.T) {
	toolsList := loadCommandToolsFromRaw(rawMCPConfig{
		Tools: map[string]rawCommandTool{
			"mmx_search": {
				Type:        "command",
				Description: "Search via MiniMax CLI",
				Command:     "mmx",
				Args:        []string{"search", "query", "--q", "{{query}}", "--output", "json"},
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}},
			},
		},
	}, config.Config{RequestTimeout: 5})
	if len(toolsList) != 1 {
		t.Fatalf("expected 1 command tool, got %d", len(toolsList))
	}
	if toolsList[0].tool.Name != "mmx_search" || toolsList[0].command != "mmx" {
		t.Fatalf("unexpected command tool: %+v", toolsList[0])
	}
	if toolsList[0].timeout != 5*time.Second {
		t.Fatalf("unexpected timeout: %v", toolsList[0].timeout)
	}
}

func TestLoadCommandToolsFromRawIncludesMMXImage(t *testing.T) {
	toolsList := loadCommandToolsFromRaw(rawMCPConfig{
		Tools: map[string]rawCommandTool{
			"mmx_image": {
				Type:        "command",
				Description: "Generate images via MiniMax CLI",
				Command:     "mmx",
				Args:        []string{"image", "generate", "--prompt", "{{prompt}}", "--output", "json"},
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string"}}, "required": []string{"prompt"}},
			},
		},
	}, config.Config{RequestTimeout: 7})
	if len(toolsList) != 1 {
		t.Fatalf("expected 1 command tool, got %d", len(toolsList))
	}
	if toolsList[0].tool.Name != "mmx_image" {
		t.Fatalf("unexpected command tool name: %+v", toolsList[0].tool)
	}
	joined := strings.Join(toolsList[0].args, " ")
	for _, want := range []string{"image", "generate", "--prompt", "{{prompt}}"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in %q", want, joined)
		}
	}
	if toolsList[0].timeout != 7*time.Second {
		t.Fatalf("unexpected timeout: %v", toolsList[0].timeout)
	}
}

func TestCommandToolCallRendersTemplates(t *testing.T) {
	workspace := t.TempDir()
	tool := &commandTool{
		tool:    Tool{Name: "helper"},
		command: os.Args[0],
		args:    []string{"-test.run=TestCommandToolHelperProcess", "--", "{{query}}", "{{MINICLAW_MODEL}}"},
		env: map[string]string{
			"GO_WANT_HELPER_PROCESS": "1",
			"MINIMAX_API_KEY":        "{{MINICLAW_API_KEY}}",
			"MINICLAW_WORKSPACE":     "{{MINICLAW_WORKSPACE}}",
		},
		timeout: 5 * time.Second,
	}
	result, err := tool.call(context.Background(), config.Config{
		Workspace: workspace,
		APIKey:    "sk-test-key",
		BaseURL:   "https://api.minimaxi.com/anthropic",
		Model:     "MiniMax-M2.7",
	}, map[string]any{"query": "MiniMax CLI"})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("failed to parse helper output: %v\n%s", err, result)
	}
	if payload["api_key"] != "sk-test-key" {
		t.Fatalf("unexpected api key: %+v", payload)
	}
	if payload["workspace"] != workspace {
		t.Fatalf("unexpected workspace: %+v", payload)
	}
	expectedPWD := filepath.Clean(workspace)
	if resolved, err := filepath.EvalSymlinks(workspace); err == nil {
		expectedPWD = filepath.Clean(resolved)
	}
	if payload["pwd"] != expectedPWD {
		t.Fatalf("unexpected working directory: %+v", payload)
	}
	args, ok := payload["args"].([]any)
	if !ok || len(args) < 2 {
		t.Fatalf("unexpected args payload: %+v", payload)
	}
	joined := make([]string, 0, len(args))
	for _, arg := range args {
		joined = append(joined, arg.(string))
	}
	flat := strings.Join(joined, " ")
	if !strings.Contains(flat, "MiniMax CLI") || !strings.Contains(flat, "MiniMax-M2.7") {
		t.Fatalf("unexpected rendered args: %v", joined)
	}
}

func TestRenderCommandTemplateRejectsMissingVariables(t *testing.T) {
	_, err := renderCommandTemplate("{{missing}}", map[string]string{"query": "ok"})
	if err == nil || !strings.Contains(err.Error(), "missing template variable") {
		t.Fatalf("expected missing variable error, got %v", err)
	}
}

func TestInjectDefaultCommandEnvForMMX(t *testing.T) {
	env := map[string]string{}
	injectDefaultCommandEnv("mmx", nil, env, config.Config{APIKey: "sk-test", BaseURL: "https://api.minimaxi.com/anthropic"})
	if env["MINIMAX_API_KEY"] != "sk-test" {
		t.Fatalf("unexpected MINIMAX_API_KEY: %+v", env)
	}
	if env["MINIMAX_REGION"] != "cn" {
		t.Fatalf("unexpected MINIMAX_REGION: %+v", env)
	}
}

func TestInjectDefaultCommandArgsForMMX(t *testing.T) {
	args := injectDefaultCommandArgs("mmx", []string{"search", "query", "--q", "MiniMax"}, config.Config{
		APIKey:  "sk-test",
		BaseURL: "https://api.minimaxi.com/anthropic",
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{"--non-interactive", "--quiet", "--region cn", "--api-key sk-test"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in %q", want, joined)
		}
	}
}

func TestBuildCommandTemplateVariablesUseCurrentMiniClawConfig(t *testing.T) {
	variables := buildCommandTemplateVariables(config.Config{
		APIKey:    "sk-from-miniclaw",
		BaseURL:   "https://api.minimaxi.com/anthropic/v1/messages",
		Model:     "MiniMax-M2.5",
		Workspace: "D:/work/github/miniclaw2",
	}, map[string]any{"query": "MiniMax MCP"})
	if variables["MINICLAW_API_KEY"] != "sk-from-miniclaw" {
		t.Fatalf("unexpected MINICLAW_API_KEY: %+v", variables)
	}
	if variables["MINICLAW_REGION"] != "cn" {
		t.Fatalf("unexpected MINICLAW_REGION: %+v", variables)
	}
	if variables["query"] != "MiniMax MCP" {
		t.Fatalf("unexpected query variable: %+v", variables)
	}
}

func TestCommandToolHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	cwd, _ := os.Getwd()
	payload := map[string]any{
		"args":      os.Args,
		"api_key":   os.Getenv("MINIMAX_API_KEY"),
		"workspace": os.Getenv("MINICLAW_WORKSPACE"),
		"pwd":       filepath.Clean(cwd),
	}
	_ = json.NewEncoder(os.Stdout).Encode(payload)
	os.Exit(0)
}
