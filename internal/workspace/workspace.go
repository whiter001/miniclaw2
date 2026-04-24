package workspace

import (
	"os"
	"path/filepath"

	"miniclaw2/internal/config"
)

var Directories = []string{
	"sessions",
	"memory",
	"state",
	"cron",
	"skills",
}

const (
	agentsTemplate    = "# MiniClaw Agent Guide\n"
	userTemplate      = "# User Preferences\n"
	heartbeatTemplate = "# Periodic Tasks\n"
	memoryTemplate    = "# Memory\n\n"
	summaryTemplate   = "# Summary\n\n"
)

func Ensure(cfg config.Config) error {
	if err := os.MkdirAll(cfg.HomeDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Workspace, 0o755); err != nil {
		return err
	}
	for _, name := range Directories {
		if err := os.MkdirAll(filepath.Join(cfg.Workspace, name), 0o755); err != nil {
			return err
		}
	}
	if err := ensureFile(filepath.Join(cfg.Workspace, "AGENTS.md"), agentsTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(cfg.Workspace, "USER.md"), userTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(cfg.Workspace, "HEARTBEAT.md"), heartbeatTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(cfg.Workspace, "memory", "MEMORY.md"), memoryTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(cfg.Workspace, "memory", "SUMMARY.md"), summaryTemplate); err != nil {
		return err
	}
	return nil
}

func ensureFile(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
