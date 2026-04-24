package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miniclaw2/internal/config"
)

func TestResolveWorkspacePathPreventsEscape(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveWorkspacePath(workspace, "../outside"); err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestWriteAndReadToolRoundtrip(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Config{Workspace: workspace}
	writeTool := ToolUse{Name: "write_file", Input: map[string]string{"path": "sample.txt", "content": "alpha\nbeta"}}
	if _, err := Execute(writeTool, cfg); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	readTool := ToolUse{Name: "read_file", Input: map[string]string{"path": "sample.txt"}}
	content, err := Execute(readTool, cfg)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if content != "alpha\nbeta" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestExecBlocksDangerousCommand(t *testing.T) {
	cfg := config.Config{Workspace: t.TempDir()}
	tool := ToolUse{Name: "exec", Input: map[string]string{"command": "rm -rf tmp-danger"}}
	if _, err := Execute(tool, cfg); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked error, got %v", err)
	}
}

func TestGrepSearchFindsMatches(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Workspace: workspace}
	tool := ToolUse{Name: "grep_search", Input: map[string]string{"query": "beta"}}
	output, err := Execute(tool, cfg)
	if err != nil {
		t.Fatalf("grep_search failed: %v", err)
	}
	if !strings.Contains(output, "notes.txt:2:beta") {
		t.Fatalf("unexpected grep output: %s", output)
	}
}
