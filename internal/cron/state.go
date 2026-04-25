package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TaskState struct {
	TaskID              string    `json:"task_id"`
	Running             bool      `json:"running,omitempty"`
	LastScheduledAt     time.Time `json:"last_scheduled_at,omitempty"`
	LastStartedAt       time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt      time.Time `json:"last_finished_at,omitempty"`
	LastDurationSeconds int       `json:"last_duration_seconds,omitempty"`
	LastStatus          string    `json:"last_status,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
	LastSessionFile     string    `json:"last_session_file,omitempty"`
	NextRunAt           time.Time `json:"next_run_at,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

type TaskStatus struct {
	Task  Task
	State TaskState
	Due   bool
}

func InspectTasks(workspace string, now time.Time) ([]TaskStatus, error) {
	tasks, err := LoadTasks(workspace)
	if err != nil {
		return nil, err
	}
	statuses := make([]TaskStatus, 0, len(tasks))
	for _, task := range tasks {
		state, err := loadTaskState(workspace, task.ID)
		if err != nil {
			return nil, err
		}
		state = normalizeTaskState(workspace, task, state, now)
		statuses = append(statuses, TaskStatus{
			Task:  task,
			State: state,
			Due:   task.Enabled && !state.Running && !state.NextRunAt.After(now),
		})
	}
	return statuses, nil
}

func UpdateHeartbeat(workspace string, now time.Time) error {
	statuses, err := InspectTasks(workspace, now)
	if err != nil {
		return err
	}
	content := strings.Builder{}
	content.WriteString("# Periodic Tasks\n\n")
	if len(statuses) == 0 {
		content.WriteString("(no cron tasks found)\n")
		return writeAtomicFile(filepath.Join(workspace, "HEARTBEAT.md"), content.String())
	}
	content.WriteString("| Task | Enabled | Due | Running | Next Run | Last Status | Last Finished |\n")
	content.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, status := range statuses {
		content.WriteString(fmt.Sprintf("| %s | %t | %t | %t | %s | %s | %s |\n",
			status.Task.ID,
			status.Task.Enabled,
			status.Due,
			status.State.Running,
			formatTaskTime(status.State.NextRunAt),
			firstNonEmpty(status.State.LastStatus, "-"),
			formatTaskTime(status.State.LastFinishedAt),
		))
	}
	return writeAtomicFile(filepath.Join(workspace, "HEARTBEAT.md"), content.String())
}

func loadTaskState(workspace, taskID string) (TaskState, error) {
	path := taskStateFilePath(workspace, taskID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TaskState{TaskID: taskID}, nil
		}
		return TaskState{}, err
	}
	var state TaskState
	if err := json.Unmarshal(data, &state); err != nil {
		return TaskState{}, fmt.Errorf("invalid cron state %s: %w", path, err)
	}
	if state.TaskID == "" {
		state.TaskID = taskID
	}
	return state, nil
}

func saveTaskState(workspace string, state TaskState) error {
	if strings.TrimSpace(state.TaskID) == "" {
		return fmt.Errorf("task state is missing task_id")
	}
	if err := os.MkdirAll(taskStateDir(workspace), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(taskStateFilePath(workspace, state.TaskID), string(data)+"\n")
}

func taskStateDir(workspace string) string {
	return filepath.Join(workspace, DefaultServeStateLayout, "tasks")
}

func taskStateFilePath(workspace, taskID string) string {
	return filepath.Join(taskStateDir(workspace), taskID+".json")
}

func normalizeTaskState(workspace string, task Task, state TaskState, now time.Time) TaskState {
	state.TaskID = task.ID
	if state.Running {
		if lock, err := readRunnerLock(workspace); err != nil || lock.TaskID != task.ID || lock.ExpiresAt.Before(now) {
			state.Running = false
		}
	}
	if state.NextRunAt.IsZero() {
		state.NextRunAt = initialNextRun(task, now)
	}
	return state
}

func initialNextRun(task Task, now time.Time) time.Time {
	if strings.HasPrefix(strings.TrimSpace(task.Schedule), "@every ") {
		return now
	}
	rounded := now.In(now.Location()).Truncate(time.Minute)
	if matches, err := MatchesSchedule(task.Schedule, rounded); err == nil && matches {
		return rounded
	}
	next, err := NextRunAfter(task.Schedule, now)
	if err != nil {
		return now
	}
	return next
}

func writeAtomicFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func formatTaskTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
