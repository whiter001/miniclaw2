package session

import (
	"os"
	"strings"
	"testing"
)

func TestRecorderAppendsMessagesAndTools(t *testing.T) {
	recorder, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.AppendMessage("message", "user", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := recorder.AppendTool("list_dir", "tool-1", "done", false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(recorder.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "\"role\":\"user\"") || !strings.Contains(content, "\"tool_name\":\"list_dir\"") {
		t.Fatalf("unexpected recorder output: %s", content)
	}
}
