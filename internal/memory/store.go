package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miniclaw2/internal/config"
)

const (
	promptRecentDays          = 2
	promptRecentChars         = 1600
	summaryMaxLinesDefault    = 20
	summaryMaxCharsDefault    = 2000
	dailyEntryMaxCharsDefault = 500
	compactMaxLineLen         = 240
	excerptTruncateLen        = 200
)

type Settings struct {
	RecentDays            int
	RecentChars           int
	SummaryMaxLines       int
	SummaryMaxChars       int
	DailyEntryMaxChars    int
	SignificanceThreshold int
	PruneKeepDays         int
}

type Store struct {
	Workspace   string
	MemoryDir   string
	MemoryFile  string
	SummaryFile string
}

func NewStore(workspace string) Store {
	memoryDir := filepath.Join(workspace, "memory")
	_ = os.MkdirAll(memoryDir, 0o755)
	return Store{
		Workspace:   workspace,
		MemoryDir:   memoryDir,
		MemoryFile:  filepath.Join(memoryDir, "MEMORY.md"),
		SummaryFile: filepath.Join(memoryDir, "SUMMARY.md"),
	}
}

func DefaultSettings() Settings {
	return Settings{
		RecentDays:            promptRecentDays,
		RecentChars:           promptRecentChars,
		SummaryMaxLines:       summaryMaxLinesDefault,
		SummaryMaxChars:       summaryMaxCharsDefault,
		DailyEntryMaxChars:    dailyEntryMaxCharsDefault,
		SignificanceThreshold: defaultSignificanceThreshold(),
		PruneKeepDays:         14,
	}
}

func SettingsFromConfig(cfg config.Config) Settings {
	settings := DefaultSettings()
	if cfg.MemoryRecentDays > 0 {
		settings.RecentDays = cfg.MemoryRecentDays
	}
	if cfg.MemoryRecentChars > 0 {
		settings.RecentChars = cfg.MemoryRecentChars
	}
	if cfg.MemorySummaryMaxLines > 0 {
		settings.SummaryMaxLines = cfg.MemorySummaryMaxLines
	}
	if cfg.MemorySummaryMaxChars > 0 {
		settings.SummaryMaxChars = cfg.MemorySummaryMaxChars
	}
	if cfg.MemoryDailyEntryMaxChars > 0 {
		settings.DailyEntryMaxChars = cfg.MemoryDailyEntryMaxChars
	}
	if cfg.MemorySignificanceThreshold > 0 {
		settings.SignificanceThreshold = cfg.MemorySignificanceThreshold
	}
	if cfg.MemoryPruneKeepDays >= 0 {
		settings.PruneKeepDays = cfg.MemoryPruneKeepDays
	}
	return settings
}

func (s Store) EnsureDefaults() error {
	if err := os.MkdirAll(s.MemoryDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(s.MemoryFile); err != nil {
		if err := writeFileAtomic(s.MemoryFile, "# Memory\n\n"); err != nil {
			return err
		}
	}
	if _, err := os.Stat(s.SummaryFile); err != nil {
		if err := writeFileAtomic(s.SummaryFile, "# Summary\n\n"); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) ReadLongTerm() string {
	return readOptionalFile(s.MemoryFile)
}

func (s Store) WriteLongTerm(content string) error {
	if err := s.EnsureDefaults(); err != nil {
		return err
	}
	return writeFileAtomic(s.MemoryFile, normalizeMarkdownDocument(content, "Memory"))
}

func (s Store) AppendLongTerm(content string) error {
	if err := s.EnsureDefaults(); err != nil {
		return err
	}
	existing := strings.TrimSpace(s.ReadLongTerm())
	addition := strings.TrimSpace(content)
	if addition == "" {
		return nil
	}
	if existing == "" {
		existing = "# Memory\n\n" + addition
	} else {
		if !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		existing += "\n" + addition
	}
	return s.WriteLongTerm(existing)
}

func (s Store) ReadSummary() string {
	return readOptionalFile(s.SummaryFile)
}

func (s Store) WriteSummary(content string) error {
	if err := s.EnsureDefaults(); err != nil {
		return err
	}
	return writeFileAtomic(s.SummaryFile, normalizeMarkdownDocument(content, "Summary"))
}

func (s Store) UpdateSummary(content string) error {
	return s.WriteSummary(content)
}

func (s Store) AppendSummaryExcerpt(content string) error {
	return s.AppendSummaryExcerptWithSettings(content, DefaultSettings())
}

func (s Store) AppendSummaryExcerptWithSettings(content string, settings Settings) error {
	entry := ExtractMemorableMemoryExcerptWithThreshold(content, settings.SummaryMaxLines, settings.SummaryMaxChars, settings.SignificanceThreshold)
	if entry == "" {
		return nil
	}
	existing := strings.TrimSpace(s.ReadSummary())
	parts := make([]string, 0, 2)
	if existing != "" {
		parts = append(parts, existing)
	}
	parts = append(parts, entry)
	combined := SummarizeMemoryTextWithThreshold(strings.Join(parts, "\n"), settings.SummaryMaxLines, settings.SummaryMaxChars, settings.SignificanceThreshold)
	if combined == "" {
		return nil
	}
	return s.WriteSummary("# Summary\n\n" + combined + "\n")
}

func (s Store) TodayFile() string {
	return s.DailyFileForDate(time.Now())
}

func (s Store) DailyFileForDate(t time.Time) string {
	dateKey := memoryDateKey(t)
	monthDir := dateKey[:6]
	return filepath.Join(s.MemoryDir, monthDir, dateKey+".md")
}

func (s Store) ReadToday() string {
	return readOptionalFile(s.TodayFile())
}

func (s Store) AppendToday(content string) error {
	entry := strings.TrimSpace(content)
	if entry == "" {
		return nil
	}
	now := time.Now()
	todayFile := s.DailyFileForDate(now)
	if err := os.MkdirAll(filepath.Dir(todayFile), 0o755); err != nil {
		return err
	}
	existing := strings.TrimRight(readOptionalFileRaw(todayFile), "\n")
	var next string
	if existing == "" {
		next = "# " + memoryDateLabel(now) + "\n\n" + entry
	} else {
		next = existing + "\n\n" + entry
	}
	return writeFileAtomic(todayFile, strings.TrimRight(next, "\n")+"\n")
}

func (s Store) RecentDailyNotes(days, maxChars int) string {
	if days <= 0 || maxChars <= 0 {
		return ""
	}
	now := time.Now()
	parts := []string{}
	remaining := maxChars
	for offset := 0; offset < days; offset++ {
		date := now.AddDate(0, 0, -offset)
		filePath := s.DailyFileForDate(date)
		if data, err := os.ReadFile(filePath); err == nil {
			trimmed := strings.TrimSpace(string(data))
			if trimmed == "" {
				continue
			}
			text := LimitText(trimmed, remaining)
			if text == "" {
				break
			}
			parts = append(parts, text)
			remaining -= len(text)
			if remaining <= 0 {
				break
			}
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func (s Store) Context() string {
	return s.ContextWithSettings(DefaultSettings())
}

func (s Store) ContextWithSettings(settings Settings) string {
	return s.ContextWithBudget(settings.RecentDays, settings.RecentChars)
}

func (s Store) ContextWithBudget(recentDays, recentChars int) string {
	longTerm := strings.TrimSpace(s.ReadLongTerm())
	summary := strings.TrimSpace(s.ReadSummary())
	recent := s.RecentDailyNotes(recentDays, recentChars)
	parts := []string{}
	if longTerm != "" {
		parts = append(parts, "## Long-term Memory\n\n"+longTerm)
	}
	if summary != "" {
		parts = append(parts, "## Summary\n\n"+summary)
	}
	if recent != "" {
		parts = append(parts, "## Recent Daily Notes\n\n"+recent)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func (s Store) SummarizeRecentNotes(days int) (string, error) {
	return s.SummarizeRecentNotesWithSettings(days, DefaultSettings())
}

func (s Store) SummarizeRecentNotesWithSettings(days int, settings Settings) (string, error) {
	notes := s.RecentDailyNotes(days, settings.SummaryMaxChars*2)
	return ExtractMemorableMemoryExcerptWithThreshold(notes, settings.SummaryMaxLines, settings.SummaryMaxChars, settings.SignificanceThreshold), nil
}

func (s Store) CompactLongTerm() error {
	content := strings.TrimSpace(s.ReadLongTerm())
	if content == "" {
		return s.WriteLongTerm("# Memory\n\n")
	}
	return s.WriteLongTerm(compactMemoryText(content))
}

func (s Store) PruneDailyNotes(keepDays int) (int, error) {
	if keepDays < 0 {
		return 0, fmt.Errorf("keepDays must be non-negative")
	}
	if _, err := os.Stat(s.MemoryDir); err != nil {
		return 0, nil
	}
	nowKey := memoryDateKey(time.Now())
	removed := 0
	entries, err := os.ReadDir(s.MemoryDir)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		monthPath := filepath.Join(s.MemoryDir, entry.Name())
		files, err := os.ReadDir(monthPath)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") || len(file.Name()) <= 3 {
				continue
			}
			dateKey := strings.TrimSuffix(file.Name(), ".md")
			if len(dateKey) != 8 {
				continue
			}
			if DaysBetweenDateKeys(nowKey, dateKey) > keepDays {
				if err := os.Remove(filepath.Join(monthPath, file.Name())); err != nil {
					return removed, err
				}
				removed++
			}
		}
		remaining, err := os.ReadDir(monthPath)
		if err == nil && len(remaining) == 0 {
			_ = os.Remove(monthPath)
		}
	}
	return removed, nil
}

func readOptionalFile(path string) string {
	if data, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

func readOptionalFileRaw(path string) string {
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}
	return ""
}

func writeFileAtomic(path, content string) error {
	tmpPath := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())
	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func normalizeMarkdownDocument(content, defaultTitle string) string {
	normalized := strings.TrimSpace(content)
	if normalized == "" {
		return "# " + defaultTitle + "\n\n"
	}
	if !strings.HasPrefix(normalized, "#") {
		normalized = "# " + defaultTitle + "\n\n" + normalized
	}
	return strings.TrimRight(normalized, "\n") + "\n"
}

func compactMemoryText(content string) string {
	lines := []string{}
	seen := map[string]bool{}
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		if len(line) > compactMaxLineLen {
			line = line[:compactMaxLineLen]
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "# Memory\n\n"
	}
	return "# Memory\n\n" + strings.Join(lines, "\n") + "\n"
}

func ExtractMemorableMemoryExcerpt(content string, maxLines, maxChars int) string {
	return ExtractMemorableMemoryExcerptWithThreshold(content, maxLines, maxChars, defaultSignificanceThreshold())
}

func ExtractMemorableMemoryExcerptWithThreshold(content string, maxLines, maxChars, threshold int) string {
	if content == "" || maxLines <= 0 || maxChars <= 0 {
		return ""
	}
	lines := []string{}
	chars := 0
	seen := map[string]bool{}
	section := ""
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "### ") {
			section = strings.TrimSpace(line[4:])
			continue
		}
		if strings.HasPrefix(line, "## ") {
			section = ""
			continue
		}
		score := memoryLineImportanceScore(section, line)
		if score < threshold {
			continue
		}
		candidate := compactMemoryExcerptLine(section, line)
		if candidate == "" {
			continue
		}
		candidate = scoredMemoryExcerptLine(score, candidate)
		selected := candidate
		if len(selected) > excerptTruncateLen {
			selected = selected[:excerptTruncateLen]
		}
		if seen[selected] {
			continue
		}
		if chars+len(selected) > maxChars {
			break
		}
		seen[selected] = true
		lines = append(lines, selected)
		chars += len(selected)
		if len(lines) >= maxLines {
			break
		}
	}
	return strings.Join(lines, "\n")
}

func defaultSignificanceThreshold() int {
	return 3
}

func memoryLineImportanceScore(section, line string) int {
	score := 0
	lower := strings.ToLower(line)
	if lower == "" {
		return 0
	}
	if strings.HasPrefix(lower, "- ") || strings.HasPrefix(lower, "* ") || strings.HasPrefix(lower, "1. ") || strings.HasPrefix(lower, "2. ") || strings.HasPrefix(lower, "3. ") || strings.HasPrefix(lower, "4. ") {
		score++
	}
	if strings.Contains(lower, ":") && len(lower) <= 60 {
		score++
	}
	for _, keyword := range strongSignalKeywords() {
		if strings.Contains(lower, keyword) {
			score += 3
			break
		}
	}
	for _, keyword := range weakSignalKeywords() {
		if strings.Contains(lower, keyword) {
			score++
		}
	}
	for _, phrase := range noisePhrases() {
		if strings.Contains(lower, phrase) {
			score -= 3
			break
		}
	}
	if section != "" {
		sectionLower := strings.ToLower(section)
		if sectionLower == "user" {
			score++
		}
		if sectionLower == "assistant" && (strings.Contains(lower, "remember") || strings.Contains(lower, "keep") || strings.Contains(lower, "will")) {
			score++
		}
	}
	if len(lower) < 12 {
		score--
	}
	return score
}

func compactMemoryExcerptLine(section, line string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(line, "\t", " "), "  ", " "))
	if normalized == "" {
		return ""
	}
	if section != "" {
		normalized = section + ": " + normalized
	}
	return normalized
}

func scoredMemoryExcerptLine(score int, line string) string {
	if score >= 8 {
		return "[critical] " + line
	}
	if score >= 5 {
		return "[important] " + line
	}
	return line
}

func strongSignalKeywords() []string {
	return []string{
		"prefer", "remember", "must", "should", "need to", "important", "decision", "decided", "summary", "fix", "bug", "error", "issue", "fail", "todo", "completed", "done", "keep", "always", "never",
		"偏好", "记住", "必须", "需要", "不要", "应该", "决定", "完成", "总结", "修复", "报错", "问题", "结论", "重要", "配置", "工作区", "工具",
	}
}

func weakSignalKeywords() []string {
	return []string{"keep", "done", "completed", "todo", "配置", "工作区", "记忆", "总结"}
}

func noisePhrases() []string {
	return []string{"i can help", "let me know", "happy to help", "sounds good", "thanks", "sure", "当然", "没问题", "好的"}
}

func SummarizeMemoryText(content string, maxLines, maxChars int) string {
	return SummarizeMemoryTextWithThreshold(content, maxLines, maxChars, defaultSignificanceThreshold())
}

func SummarizeMemoryTextWithThreshold(content string, maxLines, maxChars, threshold int) string {
	return ExtractMemorableMemoryExcerptWithThreshold(content, maxLines, maxChars, threshold)
}

func LimitText(content string, maxChars int) string {
	if maxChars <= 0 || len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n... (truncated)"
}

func DaysBetweenDateKeys(todayKey, dateKey string) int {
	if len(todayKey) != 8 || len(dateKey) != 8 {
		return 0
	}
	today, errToday := time.Parse("20060102", todayKey)
	other, errOther := time.Parse("20060102", dateKey)
	if errToday != nil || errOther != nil {
		return 0
	}
	delta := int(today.Sub(other).Hours() / 24)
	if delta < 0 {
		return -delta
	}
	return delta
}

func memoryDateKey(t time.Time) string {
	return t.Format("20060102")
}

func memoryDateLabel(t time.Time) string {
	return t.Format("2006-01-02")
}
