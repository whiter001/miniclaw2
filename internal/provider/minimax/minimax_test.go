package minimax

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miniclaw2/internal/config"
	"miniclaw2/internal/mcp"
)

func TestResolveAnthropicMessagesURLNormalizesBaseEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.minimaxi.com/anthropic":             "https://api.minimaxi.com/anthropic/v1/messages",
		"https://api.minimaxi.com/anthropic/":            "https://api.minimaxi.com/anthropic/v1/messages",
		"https://api.minimaxi.com/anthropic/messages":    "https://api.minimaxi.com/anthropic/v1/messages",
		"https://api.minimaxi.com/anthropic/v1/messages": "https://api.minimaxi.com/anthropic/v1/messages",
		"https://api.minimaxi.com/anthropic/v1":          "https://api.minimaxi.com/anthropic/v1/messages",
	}
	for input, want := range cases {
		if got := ResolveAnthropicMessagesURL(input); got != want {
			t.Fatalf("ResolveAnthropicMessagesURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildToolIterationLimitErrorContainsContext(t *testing.T) {
	err := buildToolIterationLimitError(8, "need more workspace exploration", []ToolUse{{Name: "list_dir"}, {Name: "read_file"}})
	if err == "" || err[:len(ToolIterationErrorPrefix)] != ToolIterationErrorPrefix {
		t.Fatalf("unexpected error prefix: %s", err)
	}
	if want := "after 8 rounds"; !contains(err, want) {
		t.Fatalf("expected %q in %q", want, err)
	}
	if want := "list_dir, read_file"; !contains(err, want) {
		t.Fatalf("expected %q in %q", want, err)
	}
}

func TestBuildDefaultSystemPromptUsesDefaultText(t *testing.T) {
	cfg := config.Config{Workspace: t.TempDir()}
	got := BuildDefaultSystemPrompt(cfg)
	if !contains(got, "You are MiniClaw, a local AI agent") {
		t.Fatalf("unexpected system prompt: %s", got)
	}
}

func TestBuildSystemPromptForQueryIncludesRelevantSkills(t *testing.T) {
	workspace := t.TempDir()
	skillPath := filepath.Join(workspace, "skills", "testing", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("# Testing\n\nUse Go unit tests and table-driven coverage for handler changes."), 0o644); err != nil {
		t.Fatal(err)
	}

	got := BuildSystemPromptForQuery(config.Config{Workspace: workspace}, "please add go unit tests for this handler")
	if !contains(got, "## Relevant Skills") {
		t.Fatalf("expected relevant skills in prompt, got %s", got)
	}
	if !contains(got, "table-driven coverage") {
		t.Fatalf("expected skill content in prompt, got %s", got)
	}
}

func TestBuildSystemPromptForQuerySkipsUnrelatedSkills(t *testing.T) {
	workspace := t.TempDir()
	skillPath := filepath.Join(workspace, "skills", "deploy", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("# Deploy\n\nUse systemd and deployment notes for server rollout."), 0o644); err != nil {
		t.Fatal(err)
	}

	got := BuildSystemPromptForQuery(config.Config{Workspace: workspace}, "refactor tokenizer internals")
	if contains(got, "## Relevant Skills") {
		t.Fatalf("did not expect relevant skills in prompt, got %s", got)
	}
	if contains(got, "systemd and deployment notes") {
		t.Fatalf("did not expect unrelated skill content in prompt, got %s", got)
	}
}

func TestRequestContentBlockMarshalToolUseIncludesEmptyInputObject(t *testing.T) {
	data, err := json.Marshal(requestContentBlock{
		Type: "tool_use",
		ID:   "call_function_1",
		Name: "mmx_quota",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), `"input":{}`) {
		t.Fatalf("expected empty input object in %s", string(data))
	}
	if !contains(string(data), `"name":"mmx_quota"`) {
		t.Fatalf("expected tool name in %s", string(data))
	}

	data, err = json.Marshal(requestContentBlock{
		Type:  "tool_use",
		ID:    "call_function_2",
		Name:  "mmx_quota",
		Input: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), `"input":{}`) {
		t.Fatalf("expected empty input object in %s", string(data))
	}
}

func TestRequestContentBlockMarshalTextOmitsInput(t *testing.T) {
	data, err := json.Marshal(requestContentBlock{
		Type: "text",
		Text: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(data), `"input"`) {
		t.Fatalf("did not expect input in %s", string(data))
	}
	if !contains(string(data), `"text":"hello"`) {
		t.Fatalf("expected text payload in %s", string(data))
	}
}

func TestExecuteEffectiveToolDoesNotFallbackForLocalToolErrors(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(configPath, []byte(`{"tools":{"exec":{"type":"command","command":"go","args":["version"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Workspace: t.TempDir(), EnableMCP: true, MCPConfigPath: configPath, RequestTimeout: 5}
	manager := mcp.NewManager(cfg)
	defer manager.StopAll()
	if !manager.HasTool("exec") {
		t.Fatalf("expected MCP command tool to be registered")
	}

	result, err := executeEffectiveTool(context.Background(), ToolUse{Name: "exec", Input: map[string]any{"command": "rm -rf ."}}, cfg, manager)
	if err == nil {
		t.Fatalf("expected local exec guard error, got result %q", result)
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked command error, got %v", err)
	}
	if strings.Contains(result, "go version") {
		t.Fatalf("expected no fallback result from MCP command tool, got %q", result)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (stringIndex(haystack, needle) >= 0))
}

func stringIndex(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}
