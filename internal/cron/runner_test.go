package cron

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/session"
)

func TestRunDueTasksOnceRunsSerially(t *testing.T) {
	workspace := t.TempDir()
	taskDir := filepath.Join(workspace, TaskDirName)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	files := map[string]string{
		"b.json": `{"schedule":"@every 1m","prompt":"beta"}`,
		"a.json": `{"schedule":"@every 1m","prompt":"alpha"}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(taskDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	now := time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)
	for _, taskID := range []string{"a", "b"} {
		if err := saveTaskState(workspace, TaskState{TaskID: taskID, NextRunAt: now.Add(-time.Minute)}); err != nil {
			t.Fatalf("seed state for %s: %v", taskID, err)
		}
	}
	original := executeAgent
	defer func() { executeAgent = original }()
	var mu sync.Mutex
	order := []string{}
	concurrent := 0
	maxConcurrent := 0
	executeAgent = func(ctx context.Context, cfg config.Config, prompt string, recorder *session.Recorder) (string, error) {
		mu.Lock()
		concurrent++
		if concurrent > maxConcurrent {
			maxConcurrent = concurrent
		}
		order = append(order, prompt)
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		concurrent--
		mu.Unlock()
		return "ok:" + prompt, nil
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.HomeDir = workspace
	summary, err := RunDueTasksOnce(context.Background(), cfg, now)
	if err != nil {
		t.Fatalf("RunDueTasksOnce: %v", err)
	}
	if summary.Ran != 2 || summary.Failed != 0 || summary.Skipped != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if maxConcurrent != 1 {
		t.Fatalf("max concurrent = %d, want 1", maxConcurrent)
	}
	if fmt.Sprint(order) != "[alpha beta]" {
		t.Fatalf("order = %v, want [alpha beta]", order)
	}
}

func TestRunTaskByIDUsesExistingNextRunForForcedExecution(t *testing.T) {
	workspace := t.TempDir()
	taskDir := filepath.Join(workspace, TaskDirName)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "daily.json"), []byte(`{"schedule":"0 9 * * *","prompt":"daily"}`), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
	now := time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)
	future := time.Date(2026, 4, 26, 9, 0, 0, 0, time.UTC)
	if err := saveTaskState(workspace, TaskState{TaskID: "daily", NextRunAt: future}); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	original := executeAgent
	defer func() { executeAgent = original }()
	executeAgent = func(ctx context.Context, cfg config.Config, prompt string, recorder *session.Recorder) (string, error) {
		return "ok", nil
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	result, err := RunTaskByID(context.Background(), cfg, "daily", now)
	if err != nil {
		t.Fatalf("RunTaskByID: %v", err)
	}
	if !result.NextRunAt.Equal(future) {
		t.Fatalf("next run = %s, want %s", result.NextRunAt, future)
	}
}
