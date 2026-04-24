package minimax

import (
	"encoding/json"
	"testing"

	"miniclaw2/internal/config"
)

func TestResolveAnthropicMessagesURLNormalizesBaseEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.minimaxi.com/anthropic":            "https://api.minimaxi.com/anthropic/messages",
		"https://api.minimaxi.com/anthropic/":           "https://api.minimaxi.com/anthropic/messages",
		"https://api.minimaxi.com/anthropic/messages":   "https://api.minimaxi.com/anthropic/messages",
		"https://api.minimaxi.com/anthropic/v1/messages": "https://api.minimaxi.com/anthropic/v1/messages",
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

func TestRequestContentBlockMarshalToolUseIncludesEmptyInputObject(t *testing.T) {
	data, err := json.Marshal(requestContentBlock{
		Type:  "tool_use",
		ID:    "call_function_1",
		Name:  "mmx_quota",
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
