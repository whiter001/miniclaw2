package minimax

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/memory"
)

func LoadSystemPrompt(cfg config.Config) string {
	store := memory.NewStore(cfg.Workspace)
	settings := memory.SettingsFromConfig(cfg)
	parts := []string{}
	agentsPath := filepath.Join(cfg.Workspace, "AGENTS.md")
	if content, err := os.ReadFile(agentsPath); err == nil {
		trimmed := strings.TrimSpace(string(content))
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	memoryContext := store.ContextWithSettings(settings)
	if strings.TrimSpace(memoryContext) != "" {
		parts = append(parts, memoryContext)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func AppendDailyMemoryEntry(cfg config.Config, prompt, response string) error {
	store := memory.NewStore(cfg.Workspace)
	settings := memory.SettingsFromConfig(cfg)
	now := time.Now()
	entry := []string{
		"## " + now.Format("2006-01-02") + " " + now.Format("15:04:05"),
		"",
		"### User",
		memory.LimitText(strings.TrimSpace(prompt), settings.DailyEntryMaxChars),
		"",
		"### Assistant",
		memory.LimitText(strings.TrimSpace(response), settings.DailyEntryMaxChars),
	}
	dayEntry := strings.Join(entry, "\n")
	if err := store.AppendToday(dayEntry); err != nil {
		return err
	}
	if err := store.AppendSummaryExcerptWithSettings(dayEntry, settings); err != nil {
		return err
	}
	return nil
}

func BuildDefaultSystemPrompt(cfg config.Config) string {
	systemPrompt := LoadSystemPrompt(cfg)
	defaultSystem := "You are MiniClaw, a local AI agent. When you need workspace information, prefer using tools instead of guessing. Only access files inside the workspace."
	if strings.TrimSpace(systemPrompt) == "" {
		return defaultSystem
	}
	return fmt.Sprintf("%s\n\n%s", defaultSystem, systemPrompt)
}
