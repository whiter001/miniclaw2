package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miniclaw2/internal/config"
)

func TestMemoryStoreWriteReadAndContext(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	if err := store.WriteLongTerm("User prefers concise answers."); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSummary("Keep answers short."); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendToday("### User\nRemember this."); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(store.ReadLongTerm(), "User prefers concise answers.") {
		t.Fatal("long-term memory missing content")
	}
	if !strings.Contains(store.ReadSummary(), "Keep answers short.") {
		t.Fatal("summary missing content")
	}
	if !strings.Contains(store.ReadToday(), "Remember this.") {
		t.Fatal("today note missing content")
	}
	context := store.Context()
	if !strings.Contains(context, "Long-term Memory") || !strings.Contains(context, "Summary") || !strings.Contains(context, "Recent Daily Notes") {
		t.Fatalf("unexpected context: %s", context)
	}
}

func TestLoadSystemPromptLikeBehavior(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("# Agent Notes\nUse memory well."), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(workspace)
	if err := store.WriteLongTerm("Remember the user likes markdown."); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Workspace: workspace}
	settings := SettingsFromConfig(cfg)
	prompt := strings.Join([]string{strings.TrimSpace(string(mustReadFile(t, filepath.Join(workspace, "AGENTS.md")))), store.ContextWithSettings(settings)}, "\n\n---\n\n")
	if !strings.Contains(prompt, "Agent Notes") || !strings.Contains(prompt, "Remember the user likes markdown.") || !strings.Contains(prompt, "Long-term Memory") {
		t.Fatalf("unexpected prompt: %s", prompt)
	}
}

func TestMemoryCompactDeduplicatesLongTermEntries(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	if err := store.WriteLongTerm("# Memory\n\nUser prefers concise answers.\nUser prefers concise answers.\n"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompactLongTerm(); err != nil {
		t.Fatal(err)
	}
	content := store.ReadLongTerm()
	if strings.Count(content, "User prefers concise answers.") != 1 {
		t.Fatalf("expected deduplicated content, got %s", content)
	}
}

func TestMemorySummaryFiltersChatterAndKeepsSignals(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	dayEntry := "## 2026-03-19 10:00:00\n\n### User\nhello\n\n### Assistant\nI can help with that.\n\n### User\nMiniClaw: always keep answers short.\n\n### Assistant\nI will keep answers short and avoid filler."
	if err := store.AppendSummaryExcerpt(dayEntry); err != nil {
		t.Fatal(err)
	}
	summary := store.ReadSummary()
	if !strings.Contains(summary, "[important] User: MiniClaw: always keep answers short.") {
		t.Fatalf("expected important summary line, got %s", summary)
	}
	if !strings.Contains(summary, "Assistant: I will keep answers short and avoid filler.") {
		t.Fatalf("expected assistant summary line, got %s", summary)
	}
	if strings.Contains(summary, "I can help with that.") || strings.Contains(summary, "hello") {
		t.Fatalf("expected chatter to be filtered, got %s", summary)
	}
}

func TestDaysBetweenDateKeysHandlesCrossYearBoundaries(t *testing.T) {
	if DaysBetweenDateKeys("20260101", "20251231") != 1 || DaysBetweenDateKeys("20251231", "20260101") != 1 || DaysBetweenDateKeys("20260319", "20260318") != 1 {
		t.Fatal("unexpected date delta")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
