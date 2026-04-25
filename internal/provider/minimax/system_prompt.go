package minimax

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/memory"
	"miniclaw2/internal/skills"
)

type PromptContext struct {
	Prompt string
	Skills []skills.Skill
}

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

func BuildPromptContextForQuery(cfg config.Config, query string) PromptContext {
	loaded := skills.Discover(filepath.Join(cfg.Workspace, "skills"))
	selected := skills.Select(query, cfg.SkillSelectionLimit, loaded)
	base := []string{"You are MiniClaw, a local AI agent. When you need workspace information, prefer using tools instead of guessing. Only access files inside the workspace."}
	if systemPrompt := LoadSystemPrompt(cfg); strings.TrimSpace(systemPrompt) != "" {
		base = append(base, systemPrompt)
	}
	if relevantSkills := skills.BuildContext(selected); strings.TrimSpace(relevantSkills) != "" {
		base = append(base, relevantSkills)
	}
	return PromptContext{Prompt: strings.Join(base, "\n\n"), Skills: selected}
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
	return BuildPromptContextForQuery(cfg, "").Prompt
}

func BuildSystemPromptForQuery(cfg config.Config, query string) string {
	return BuildPromptContextForQuery(cfg, query).Prompt
}
