package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	TaskDirName             = "cron"
	DefaultTaskTimeout      = 10 * time.Minute
	DefaultPollInterval     = 15 * time.Second
	DefaultServeStateLayout = "state/cron"
)

type Task struct {
	ID                string
	Description       string
	Schedule          string
	Prompt            string
	Enabled           bool
	SkipIfRunning     bool
	Timeout           time.Duration
	MaxToolIterations int
	EnableMCP         *bool
	FilePath          string
}

type taskFile struct {
	ID                string `json:"id"`
	Description       string `json:"description,omitempty"`
	Schedule          string `json:"schedule"`
	Prompt            string `json:"prompt"`
	Enabled           *bool  `json:"enabled,omitempty"`
	SkipIfRunning     *bool  `json:"skip_if_running,omitempty"`
	TimeoutSeconds    int    `json:"timeout_seconds,omitempty"`
	MaxToolIterations int    `json:"max_tool_iterations,omitempty"`
	EnableMCP         *bool  `json:"enable_mcp,omitempty"`
}

func LoadTasks(workspace string) ([]Task, error) {
	taskDir := filepath.Join(workspace, TaskDirName)
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	tasks := make([]Task, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(taskDir, entry.Name())
		task, err := loadTaskFile(path)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks, nil
}

func loadTaskFile(path string) (Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Task{}, err
	}
	var raw taskFile
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return Task{}, fmt.Errorf("invalid cron task %s: %w", path, err)
	}
	task := Task{
		ID:                strings.TrimSpace(raw.ID),
		Description:       strings.TrimSpace(raw.Description),
		Schedule:          strings.TrimSpace(raw.Schedule),
		Prompt:            strings.TrimSpace(raw.Prompt),
		Enabled:           raw.Enabled == nil || *raw.Enabled,
		SkipIfRunning:     raw.SkipIfRunning == nil || *raw.SkipIfRunning,
		Timeout:           DefaultTaskTimeout,
		MaxToolIterations: raw.MaxToolIterations,
		EnableMCP:         raw.EnableMCP,
		FilePath:          path,
	}
	if raw.TimeoutSeconds > 0 {
		task.Timeout = time.Duration(raw.TimeoutSeconds) * time.Second
	}
	if task.ID == "" {
		task.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if err := ValidateTask(task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func ValidateTask(task Task) error {
	if strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("invalid cron task %s: missing id", task.FilePath)
	}
	if strings.TrimSpace(task.Schedule) == "" {
		return fmt.Errorf("invalid cron task %s: missing schedule", task.FilePath)
	}
	if strings.TrimSpace(task.Prompt) == "" {
		return fmt.Errorf("invalid cron task %s: missing prompt", task.FilePath)
	}
	if task.Timeout <= 0 {
		return fmt.Errorf("invalid cron task %s: timeout must be positive", task.FilePath)
	}
	if task.MaxToolIterations < 0 {
		return fmt.Errorf("invalid cron task %s: max_tool_iterations must be non-negative", task.FilePath)
	}
	if _, err := NextRunAfter(task.Schedule, time.Unix(0, 0).UTC()); err != nil {
		return fmt.Errorf("invalid cron task %s: %w", task.FilePath, err)
	}
	return nil
}
