package cron

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTasksAppliesDefaultsAndSorts(t *testing.T) {
	workspace := t.TempDir()
	taskDir := filepath.Join(workspace, TaskDirName)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	files := map[string]string{
		"b.json": `{"schedule":"*/5 * * * *","prompt":"beta"}`,
		"a.json": `{"id":"alpha","schedule":"@daily","prompt":"alpha","enabled":true}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(taskDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	tasks, err := LoadTasks(workspace)
	if err != nil {
		t.Fatalf("LoadTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
	if tasks[0].ID != "alpha" || tasks[1].ID != "b" {
		t.Fatalf("unexpected task order: %#v", tasks)
	}
	if !tasks[1].Enabled {
		t.Fatalf("expected enabled default to true")
	}
	if !tasks[1].SkipIfRunning {
		t.Fatalf("expected skip_if_running default to true")
	}
	if tasks[1].Timeout != DefaultTaskTimeout {
		t.Fatalf("timeout = %s, want %s", tasks[1].Timeout, DefaultTaskTimeout)
	}
}

func TestLoadTasksRejectsInvalidTask(t *testing.T) {
	workspace := t.TempDir()
	taskDir := filepath.Join(workspace, TaskDirName)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	path := filepath.Join(taskDir, "bad.json")
	if err := os.WriteFile(path, []byte(`{"schedule":"bad cron","prompt":"x"}`), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
	if _, err := LoadTasks(workspace); err == nil {
		t.Fatalf("expected invalid task error")
	}
}

func TestNextRunAfterSupportsCronAndSpecialForms(t *testing.T) {
	base := time.Date(2026, 4, 25, 10, 7, 13, 0, time.FixedZone("UTC+8", 8*60*60))
	tests := []struct {
		name string
		spec string
		want time.Time
	}{
		{
			name: "cron every five minutes",
			spec: "*/5 * * * *",
			want: time.Date(2026, 4, 25, 10, 10, 0, 0, base.Location()),
		},
		{
			name: "daily macro",
			spec: "@daily",
			want: time.Date(2026, 4, 26, 0, 0, 0, 0, base.Location()),
		},
		{
			name: "every duration",
			spec: "@every 90s",
			want: time.Date(2026, 4, 25, 10, 8, 43, 0, base.Location()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NextRunAfter(tt.spec, base)
			if err != nil {
				t.Fatalf("NextRunAfter error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("NextRunAfter(%q) = %s, want %s", tt.spec, got, tt.want)
			}
		})
	}
}

func TestNextRunAfterRejectsInvalidSpecs(t *testing.T) {
	invalid := []string{"", "* * *", "61 * * * *", "@every nope"}
	for _, spec := range invalid {
		if _, err := NextRunAfter(spec, time.Now()); err == nil {
			t.Fatalf("expected error for %q", spec)
		}
	}
}
