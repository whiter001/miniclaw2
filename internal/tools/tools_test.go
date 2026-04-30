package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestExecHonorsContextCancellation(t *testing.T) {
	cfg := config.Config{Workspace: t.TempDir()}
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "ping -n 3 127.0.0.1 >NUL"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	_, err := ExecuteWithContext(ctx, ToolUse{Name: "exec", Input: map[string]string{"command": command}}, cfg)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("exec cancellation took too long: %s", elapsed)
	}
}

func TestCappedOutputBufferTruncates(t *testing.T) {
	buffer := newCappedOutputBuffer(4)
	if n, err := buffer.Write([]byte("abcdef")); err != nil || n != 6 {
		t.Fatalf("unexpected write result: n=%d err=%v", n, err)
	}
	if !buffer.Truncated() {
		t.Fatal("expected buffer to be marked truncated")
	}
	got := trimExecOutput(buffer.String(), buffer.Truncated())
	if got != "abcd\n... (truncated)" {
		t.Fatalf("unexpected output: %q", got)
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
