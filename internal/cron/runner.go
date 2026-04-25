package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/provider/minimax"
	"miniclaw2/internal/session"
)

var ErrWorkspaceLocked = errors.New("cron workspace lock is held")

var executeAgent = minimax.RunAgentInSession

type RunResult struct {
	TaskID      string
	Status      string
	Message     string
	StartedAt   time.Time
	FinishedAt  time.Time
	NextRunAt   time.Time
	SessionFile string
}

type RunSummary struct {
	Evaluated int
	Ran       int
	Skipped   int
	Failed    int
	Results   []RunResult
}

type runnerLock struct {
	TaskID    string    `json:"task_id"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type workspaceLock struct {
	path string
}

func RunDueTasksOnce(ctx context.Context, cfg config.Config, now time.Time) (RunSummary, error) {
	tasks, err := LoadTasks(cfg.Workspace)
	if err != nil {
		return RunSummary{}, err
	}
	summary := RunSummary{Evaluated: len(tasks), Results: make([]RunResult, 0, len(tasks))}
	var firstErr error
	for _, task := range tasks {
		result, err := runTask(ctx, cfg, task, now, false)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if result.TaskID == "" {
			continue
		}
		summary.Results = append(summary.Results, result)
		switch result.Status {
		case "success":
			summary.Ran++
		case "failed":
			summary.Failed++
		case "skipped":
			summary.Skipped++
		}
	}
	if err := UpdateHeartbeat(cfg.Workspace, now); err != nil && firstErr == nil {
		firstErr = err
	}
	return summary, firstErr
}

func RunTaskByID(ctx context.Context, cfg config.Config, taskID string, now time.Time) (RunResult, error) {
	tasks, err := LoadTasks(cfg.Workspace)
	if err != nil {
		return RunResult{}, err
	}
	for _, task := range tasks {
		if task.ID == taskID {
			result, runErr := runTask(ctx, cfg, task, now, true)
			if heartbeatErr := UpdateHeartbeat(cfg.Workspace, now); heartbeatErr != nil && runErr == nil {
				runErr = heartbeatErr
			}
			return result, runErr
		}
	}
	return RunResult{}, fmt.Errorf("unknown cron task: %s", taskID)
}

func Serve(ctx context.Context, cfg config.Config, pollInterval time.Duration, logf func(string)) error {
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	if logf != nil {
		logf(fmt.Sprintf("cron serve started (workspace=%s, poll=%s)", cfg.Workspace, pollInterval))
	}
	for {
		now := time.Now()
		summary, err := RunDueTasksOnce(ctx, cfg, now)
		if logf != nil {
			logf(fmt.Sprintf("cron tick: evaluated=%d ran=%d skipped=%d failed=%d", summary.Evaluated, summary.Ran, summary.Skipped, summary.Failed))
		}
		if err != nil && logf != nil {
			logf("cron tick error: " + err.Error())
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func runTask(ctx context.Context, cfg config.Config, task Task, now time.Time, force bool) (RunResult, error) {
	state, err := loadTaskState(cfg.Workspace, task.ID)
	if err != nil {
		return RunResult{TaskID: task.ID, Status: "failed", Message: err.Error()}, err
	}
	state = normalizeTaskState(cfg.Workspace, task, state, now)
	if err := saveTaskState(cfg.Workspace, state); err != nil {
		return RunResult{TaskID: task.ID, Status: "failed", Message: err.Error()}, err
	}
	if !force {
		if !task.Enabled {
			return RunResult{TaskID: task.ID, Status: "idle", Message: "task disabled", NextRunAt: state.NextRunAt}, nil
		}
		if state.Running || state.NextRunAt.After(now) {
			return RunResult{TaskID: task.ID, Status: "idle", Message: "not due", NextRunAt: state.NextRunAt}, nil
		}
	}
	lock, err := acquireWorkspaceLock(ctx, cfg.Workspace, task, now)
	if err != nil {
		if errors.Is(err, ErrWorkspaceLocked) {
			return RunResult{TaskID: task.ID, Status: "skipped", Message: err.Error(), NextRunAt: state.NextRunAt}, nil
		}
		return RunResult{TaskID: task.ID, Status: "failed", Message: err.Error(), NextRunAt: state.NextRunAt}, err
	}
	defer lock.Release()

	state, err = loadTaskState(cfg.Workspace, task.ID)
	if err != nil {
		return RunResult{TaskID: task.ID, Status: "failed", Message: err.Error()}, err
	}
	state = normalizeTaskState(cfg.Workspace, task, state, now)
	if !force {
		if !task.Enabled {
			return RunResult{TaskID: task.ID, Status: "idle", Message: "task disabled", NextRunAt: state.NextRunAt}, nil
		}
		if state.Running || state.NextRunAt.After(now) {
			return RunResult{TaskID: task.ID, Status: "idle", Message: "not due", NextRunAt: state.NextRunAt}, nil
		}
	}
	priorNextRun := state.NextRunAt
	scheduledAt := now
	if !force && !priorNextRun.IsZero() {
		scheduledAt = priorNextRun
	}
	startedAt := time.Now()
	state.Running = true
	state.LastScheduledAt = scheduledAt
	state.LastStartedAt = startedAt
	state.LastFinishedAt = time.Time{}
	state.LastDurationSeconds = 0
	state.LastStatus = "running"
	state.LastError = ""
	state.UpdatedAt = startedAt
	if err := saveTaskState(cfg.Workspace, state); err != nil {
		return RunResult{TaskID: task.ID, Status: "failed", Message: err.Error(), NextRunAt: priorNextRun}, err
	}

	runCfg := cfg
	if task.MaxToolIterations > 0 {
		runCfg.MaxToolIterations = task.MaxToolIterations
	}
	if task.EnableMCP != nil {
		runCfg.EnableMCP = *task.EnableMCP
	}
	recorder, err := session.New(runCfg.Workspace)
	if err != nil {
		state.Running = false
		state.LastStatus = "failed"
		state.LastError = err.Error()
		state.LastFinishedAt = time.Now()
		state.UpdatedAt = state.LastFinishedAt
		_ = saveTaskState(runCfg.Workspace, state)
		return RunResult{TaskID: task.ID, Status: "failed", Message: err.Error(), NextRunAt: priorNextRun}, err
	}
	runCtx := ctx
	cancel := func() {}
	if task.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, task.Timeout)
	}
	response, runErr := executeAgent(runCtx, runCfg, task.Prompt, recorder)
	cancel()
	finishedAt := time.Now()
	nextRunAt, nextErr := nextFutureRun(task, priorNextRun, scheduledAt, finishedAt, force)
	if nextErr != nil && runErr == nil {
		runErr = nextErr
	}
	state.Running = false
	state.LastFinishedAt = finishedAt
	state.LastDurationSeconds = int(finishedAt.Sub(startedAt) / time.Second)
	state.LastSessionFile = recorder.FilePath
	state.UpdatedAt = finishedAt
	state.NextRunAt = nextRunAt
	if runErr != nil {
		state.LastStatus = "failed"
		state.LastError = runErr.Error()
	} else {
		state.LastStatus = "success"
		state.LastError = ""
	}
	if err := saveTaskState(runCfg.Workspace, state); err != nil && runErr == nil {
		runErr = err
	}
	result := RunResult{
		TaskID:      task.ID,
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
		NextRunAt:   nextRunAt,
		SessionFile: recorder.FilePath,
	}
	if runErr != nil {
		result.Status = "failed"
		result.Message = runErr.Error()
		return result, runErr
	}
	result.Status = "success"
	result.Message = response
	return result, nil
}

func nextFutureRun(task Task, priorNextRun, scheduledAt, now time.Time, force bool) (time.Time, error) {
	if force && priorNextRun.After(now) {
		return priorNextRun, nil
	}
	anchor := scheduledAt
	if anchor.IsZero() {
		anchor = now
	}
	nextRun, err := NextRunAfter(task.Schedule, anchor)
	if err != nil {
		return time.Time{}, err
	}
	for !nextRun.After(now) {
		nextRun, err = NextRunAfter(task.Schedule, nextRun)
		if err != nil {
			return time.Time{}, err
		}
	}
	return nextRun, nil
}

func acquireWorkspaceLock(ctx context.Context, workspace string, task Task, now time.Time) (*workspaceLock, error) {
	for {
		lock, err := tryAcquireWorkspaceLock(workspace, task, now)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, ErrWorkspaceLocked) {
			return nil, err
		}
		if task.SkipIfRunning {
			return nil, err
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func tryAcquireWorkspaceLock(workspace string, task Task, now time.Time) (*workspaceLock, error) {
	path := runnerLockPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	record := runnerLock{
		TaskID:    task.ID,
		PID:       os.Getpid(),
		StartedAt: now,
		ExpiresAt: now.Add(task.Timeout + time.Minute),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		defer f.Close()
		if _, writeErr := f.Write(append(data, '\n')); writeErr != nil {
			_ = os.Remove(path)
			return nil, writeErr
		}
		return &workspaceLock{path: path}, nil
	}
	if !os.IsExist(err) {
		return nil, err
	}
	existing, readErr := readRunnerLock(workspace)
	if readErr == nil && existing.ExpiresAt.Before(now) {
		_ = os.Remove(path)
		return tryAcquireWorkspaceLock(workspace, task, now)
	}
	return nil, ErrWorkspaceLocked
}

func readRunnerLock(workspace string) (runnerLock, error) {
	data, err := os.ReadFile(runnerLockPath(workspace))
	if err != nil {
		return runnerLock{}, err
	}
	var record runnerLock
	if err := json.Unmarshal(data, &record); err != nil {
		return runnerLock{}, err
	}
	return record, nil
}

func runnerLockPath(workspace string) string {
	return filepath.Join(workspace, DefaultServeStateLayout, "runner.lock")
}

func (l *workspaceLock) Release() {
	if l == nil || l.path == "" {
		return
	}
	_ = os.Remove(l.path)
}
