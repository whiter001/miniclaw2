package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miniclaw2/internal/config"
)

func TestDiscoverAndSelectRelevantSkills(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "testing", "# Testing\n\nUse Go unit tests and table-driven coverage for handler changes.")
	writeSkill(t, root, "deploy", "# Deploy\n\nUse systemd and deployment notes for server rollout.")

	loaded := Discover(root)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(loaded))
	}
	selected := Select("add go unit tests for the handler", 2, loaded)
	if len(selected) != 1 {
		t.Fatalf("expected 1 selected skill, got %d", len(selected))
	}
	if selected[0].Name != "testing" {
		t.Fatalf("expected testing skill, got %s", selected[0].Name)
	}
}

func TestBuildTurnContextReturnsEmptyWhenNoMatch(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "deploy", "# Deploy\n\nUse systemd and deployment notes for server rollout.")

	got := BuildTurnContext("refactor tokenizer internals", 2, root)
	if got != "" {
		t.Fatalf("expected empty turn context, got %s", got)
	}
}

func TestBuildTurnContextIncludesRelevantSkillContent(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "testing", "# Testing\n\nUse Go unit tests and table-driven coverage for handler changes.")

	got := BuildTurnContext("please add go unit tests", 2, root)
	if !strings.Contains(got, "## Relevant Skills") {
		t.Fatalf("expected relevant skills heading, got %s", got)
	}
	if !strings.Contains(got, "testing") {
		t.Fatalf("expected skill name in context, got %s", got)
	}
	if !strings.Contains(got, "table-driven coverage") {
		t.Fatalf("expected skill content in context, got %s", got)
	}
}

func TestSelectPrefersHigherScoredSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "testing-low", "# Testing\n\nUse Go unit tests and table-driven coverage for handler changes.")
	writeSkill(t, root, "testing-high", "# Testing\n\nUse Go unit tests and table-driven coverage for handler changes.")
	if err := writeSkillMetadata(filepath.Join(root, "testing-high", skillMetadataFileName), SkillMetadata{Score: 90, SuccessCount: 3}); err != nil {
		t.Fatal(err)
	}

	selected := Select("please add go unit tests for this handler", 2, Discover(root))
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected skills, got %d", len(selected))
	}
	if selected[0].Name != "testing-high" {
		t.Fatalf("expected higher scored skill first, got %s", selected[0].Name)
	}
}

func TestUpdateSelectedSkillScoresCreatesMetadataForManualSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "testing", "# Testing\n\nUse Go unit tests and table-driven coverage for handler changes.")
	loaded := Discover(root)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(loaded))
	}
	cfg := config.Default()
	cfg.EnableSkillScoring = true
	if err := UpdateSelectedSkillScores(cfg, loaded, true); err != nil {
		t.Fatal(err)
	}
	reloaded := Discover(root)
	if reloaded[0].Metadata.SelectedCount != 1 || reloaded[0].Metadata.SuccessCount != 1 {
		t.Fatalf("unexpected metadata: %+v", reloaded[0].Metadata)
	}
	if reloaded[0].Metadata.Score <= 0 {
		t.Fatalf("expected positive score, got %+v", reloaded[0].Metadata)
	}
}

func TestAutoCaptureSessionCreatesAndUpdatesSkill(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.EnableAutoSkills = true
	cfg.AutoSkillMinToolCalls = 2
	cfg.AutoSkillMaxExamples = 2

	writeSession(t, filepath.Join(workspace, "sessions", "session-1.jsonl"), []string{
		jsonLine(t, map[string]any{"kind": "message", "role": "user", "content": "add go unit tests for handler"}),
		jsonLine(t, map[string]any{"kind": "tool", "tool_name": "read_file", "content": "done"}),
		jsonLine(t, map[string]any{"kind": "tool", "tool_name": "write_file", "content": "done"}),
		jsonLine(t, map[string]any{"kind": "message", "role": "assistant", "content": "added tests"}),
	})
	if err := AutoCaptureSession(cfg, filepath.Join(workspace, "sessions", "session-1.jsonl"), "", ""); err != nil {
		t.Fatal(err)
	}
	writeSession(t, filepath.Join(workspace, "sessions", "session-2.jsonl"), []string{
		jsonLine(t, map[string]any{"kind": "message", "role": "user", "content": "add go unit tests for parser"}),
		jsonLine(t, map[string]any{"kind": "tool", "tool_name": "read_file", "content": "done"}),
		jsonLine(t, map[string]any{"kind": "tool", "tool_name": "write_file", "content": "done"}),
		jsonLine(t, map[string]any{"kind": "message", "role": "assistant", "content": "added parser tests"}),
	})
	if err := AutoCaptureSession(cfg, filepath.Join(workspace, "sessions", "session-2.jsonl"), "", ""); err != nil {
		t.Fatal(err)
	}
	writeSession(t, filepath.Join(workspace, "sessions", "session-3.jsonl"), []string{
		jsonLine(t, map[string]any{"kind": "message", "role": "user", "content": "add go unit tests for runtime"}),
		jsonLine(t, map[string]any{"kind": "tool", "tool_name": "read_file", "content": "done"}),
		jsonLine(t, map[string]any{"kind": "tool", "tool_name": "write_file", "content": "done"}),
		jsonLine(t, map[string]any{"kind": "message", "role": "assistant", "content": "added runtime tests"}),
	})
	if err := AutoCaptureSession(cfg, filepath.Join(workspace, "sessions", "session-3.jsonl"), "", ""); err != nil {
		t.Fatal(err)
	}

	loaded := Discover(filepath.Join(workspace, "skills"))
	if len(loaded) != 1 {
		t.Fatalf("expected 1 autoskill, got %d", len(loaded))
	}
	if !loaded[0].Metadata.Auto {
		t.Fatalf("expected autoskill metadata, got %+v", loaded[0].Metadata)
	}
	if loaded[0].Metadata.CaptureCount != 3 {
		t.Fatalf("expected 3 captures, got %+v", loaded[0].Metadata)
	}
	if len(loaded[0].Metadata.Examples) != 2 {
		t.Fatalf("expected capped examples, got %+v", loaded[0].Metadata)
	}
	if !strings.Contains(loaded[0].Content, "Current score") {
		t.Fatalf("expected rendered autoskill content, got %s", loaded[0].Content)
	}
	if !strings.Contains(strings.Join(loaded[0].Metadata.Tools, ","), "read_file") {
		t.Fatalf("expected tool metadata, got %+v", loaded[0].Metadata)
	}
}

func TestCreateOptimizeAndDeleteSkill(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace

	created, err := CreateSkill(cfg, "Testing Skill", "Use read_file before editing and validate with go test.", false)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "testing-skill" {
		t.Fatalf("unexpected created skill name: %s", created.Name)
	}
	if created.Metadata.Score <= 0 {
		t.Fatalf("expected score for created skill, got %+v", created.Metadata)
	}
	optimized, err := OptimizeSkill(cfg, "Testing Skill")
	if err != nil {
		t.Fatal(err)
	}
	if len(optimized.Metadata.Tools) == 0 || optimized.Metadata.Tools[0] != "read_file" {
		t.Fatalf("expected detected tools after optimize, got %+v", optimized.Metadata)
	}
	listed := ListSkills(cfg)
	if len(listed) != 1 || listed[0].Name != "testing-skill" {
		t.Fatalf("unexpected listed skills: %+v", listed)
	}
	if err := DeleteSkill(cfg, "testing skill"); err != nil {
		t.Fatal(err)
	}
	if _, err := FindSkill(cfg, "testing-skill"); err == nil {
		t.Fatal("expected skill to be deleted")
	}
}

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSession(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func jsonLine(t *testing.T, payload map[string]any) string {
	t.Helper()
	parts := make([]string, 0, len(payload))
	for key, value := range payload {
		switch typed := value.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("\"%s\":\"%s\"", key, strings.ReplaceAll(typed, "\"", "\\\"")))
		case bool:
			parts = append(parts, fmt.Sprintf("\"%s\":%t", key, typed))
		default:
			t.Fatal("unsupported jsonLine payload type")
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}