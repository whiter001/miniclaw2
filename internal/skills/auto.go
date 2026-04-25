package skills

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/memory"
)

const (
	skillMetadataFileName      = "skill.json"
	defaultAutoSkillExamples   = 5
	maxExamplePromptChars      = 240
	maxExampleResponseChars    = 360
	minimumExistingSkillMatch  = 4
)

type SkillMetadata struct {
	Slug          string         `json:"slug,omitempty"`
	Auto          bool           `json:"auto,omitempty"`
	Score         int            `json:"score,omitempty"`
	CaptureCount  int            `json:"capture_count,omitempty"`
	SelectedCount int            `json:"selected_count,omitempty"`
	SuccessCount  int            `json:"success_count,omitempty"`
	FailureCount  int            `json:"failure_count,omitempty"`
	Keywords      []string       `json:"keywords,omitempty"`
	Tools         []string       `json:"tools,omitempty"`
	Examples      []SkillExample `json:"examples,omitempty"`
	CreatedAt     string         `json:"created_at,omitempty"`
	UpdatedAt     string         `json:"updated_at,omitempty"`
}

type SkillExample struct {
	Prompt    string   `json:"prompt,omitempty"`
	Response  string   `json:"response,omitempty"`
	ToolNames []string `json:"tool_names,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

type sessionLine struct {
	Kind     string `json:"kind"`
	Role     string `json:"role,omitempty"`
	Content  string `json:"content,omitempty"`
	ToolName string `json:"tool_name,omitempty"`
	IsError  bool   `json:"is_error,omitempty"`
}

type sessionExperience struct {
	Prompt              string
	Response            string
	ToolNames           []string
	SuccessfulToolCount int
	FailedToolCount     int
	Keywords            []string
}

func UpdateSelectedSkillScores(cfg config.Config, selected []Skill, success bool) error {
	if !cfg.EnableSkillScoring || len(selected) == 0 {
		return nil
	}
	for _, skill := range selected {
		meta := skill.Metadata
		meta.SelectedCount++
		if success {
			meta.SuccessCount++
		} else {
			meta.FailureCount++
		}
		meta.Keywords = mergeOrdered(meta.Keywords, tokenize(skill.Name+" "+skill.Description))
		meta.Tools = mergeOrdered(meta.Tools, skill.Metadata.Tools)
		meta.Score = calculateSkillScore(meta)
		meta.UpdatedAt = time.Now().Format(time.RFC3339)
		if meta.CreatedAt == "" {
			meta.CreatedAt = meta.UpdatedAt
		}
		if err := writeSkillMetadata(sidecarPath(skill.Path), meta); err != nil {
			return err
		}
	}
	return nil
}

func AutoCaptureSession(cfg config.Config, sessionPath, prompt, response string) error {
	if !cfg.EnableAutoSkills {
		return nil
	}
	experience, err := readSessionExperience(sessionPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prompt) != "" {
		experience.Prompt = strings.TrimSpace(prompt)
	}
	if strings.TrimSpace(response) != "" {
		experience.Response = strings.TrimSpace(response)
	}
	experience.Keywords = tokenize(experience.Prompt)
	if strings.TrimSpace(experience.Prompt) == "" || strings.TrimSpace(experience.Response) == "" {
		return nil
	}
	if experience.SuccessfulToolCount < cfg.AutoSkillMinToolCalls {
		return nil
	}
	return upsertAutoSkill(cfg, experience)
}

func readSkillMetadata(path string) (SkillMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillMetadata{}, err
	}
	meta := SkillMetadata{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return SkillMetadata{}, err
	}
	meta.Keywords = compactStrings(meta.Keywords)
	meta.Tools = compactStrings(meta.Tools)
	return meta, nil
}

func writeSkillMetadata(path string, meta SkillMetadata) error {
	meta.Keywords = compactStrings(meta.Keywords)
	meta.Tools = compactStrings(meta.Tools)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func readSessionExperience(path string) (sessionExperience, error) {
	file, err := os.Open(path)
	if err != nil {
		return sessionExperience{}, err
	}
	defer file.Close()

	experience := sessionExperience{}
	seenTools := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := sessionLine{}
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		switch {
		case line.Role == "user" && experience.Prompt == "":
			experience.Prompt = strings.TrimSpace(line.Content)
		case line.Role == "assistant" && strings.TrimSpace(line.Content) != "":
			experience.Response = strings.TrimSpace(line.Content)
		case line.Kind == "tool" && strings.TrimSpace(line.ToolName) != "":
			if strings.EqualFold(strings.TrimSpace(line.Content), "invoked") {
				continue
			}
			if line.IsError {
				experience.FailedToolCount++
				continue
			}
			experience.SuccessfulToolCount++
			if _, ok := seenTools[line.ToolName]; ok {
				continue
			}
			seenTools[line.ToolName] = struct{}{}
			experience.ToolNames = append(experience.ToolNames, line.ToolName)
		}
	}
	if err := scanner.Err(); err != nil {
		return sessionExperience{}, err
	}
	return experience, nil
}

func upsertAutoSkill(cfg config.Config, experience sessionExperience) error {
	root := filepath.Join(cfg.Workspace, "skills")
	loaded := Discover(root)
	target := findBestAutoSkill(loaded, experience)
	now := time.Now().Format(time.RFC3339)
	maxExamples := cfg.AutoSkillMaxExamples
	if maxExamples <= 0 {
		maxExamples = defaultAutoSkillExamples
	}

	meta := target.Metadata
	name := target.Name
	skillPath := target.Path
	if skillPath == "" {
		slug := buildSkillSlug(experience.Keywords)
		if slug == "" {
			slug = "autoskill-general-workflow"
		}
		name = slug
		skillPath = filepath.Join(root, slug, "SKILL.md")
		meta = SkillMetadata{Slug: slug, Auto: true, CreatedAt: now}
	}
	meta.Auto = true
	meta.CaptureCount++
	meta.UpdatedAt = now
	meta.Keywords = mergeOrdered(meta.Keywords, experience.Keywords)
	meta.Tools = mergeOrdered(meta.Tools, experience.ToolNames)
	meta.Examples = compactExamples(append([]SkillExample{{
		Prompt:    memory.LimitText(strings.TrimSpace(experience.Prompt), maxExamplePromptChars),
		Response:  memory.LimitText(strings.TrimSpace(experience.Response), maxExampleResponseChars),
		ToolNames: append([]string(nil), experience.ToolNames...),
		CreatedAt: now,
	}}, meta.Examples...), maxExamples)
	meta.Score = calculateSkillScore(meta)

	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(skillPath, []byte(renderAutoSkill(name, meta)), 0o644); err != nil {
		return err
	}
	return writeSkillMetadata(sidecarPath(skillPath), meta)
}

func findBestAutoSkill(loaded []Skill, experience sessionExperience) Skill {
	expectedSlug := buildSkillSlug(experience.Keywords)
	best := Skill{}
	bestScore := 0
	for _, skill := range loaded {
		if !skill.Metadata.Auto {
			continue
		}
		if expectedSlug != "" && skill.Metadata.Slug == expectedSlug {
			return skill
		}
		score := overlapScore(skill.Metadata.Keywords, experience.Keywords)*2 + overlapScore(skill.Metadata.Tools, experience.ToolNames)*3
		if score > bestScore {
			best = skill
			bestScore = score
		}
	}
	if bestScore < minimumExistingSkillMatch {
		return Skill{}
	}
	return best
}

func buildSkillSlug(tokens []string) string {
	parts := []string{"autoskill"}
	for _, token := range tokens {
		cleaned := strings.ToLower(strings.TrimSpace(token))
		if cleaned == "" {
			continue
		}
		parts = append(parts, cleaned)
		if len(parts) == 5 {
			break
		}
	}
	return strings.Join(parts, "-")
}

func renderAutoSkill(name string, meta SkillMetadata) string {
	title := prettySkillTitle(name)
	lines := []string{
		"# " + title,
		"",
		"Auto-generated from successful MiniClaw runs. This skill is optimized automatically as more matching executions are captured.",
		"",
		fmt.Sprintf("Current score: %d/100", meta.Score),
		"",
		"## When To Use",
		"Use this skill for requests similar to the captured examples and keyword set below.",
		"",
		"## Keywords",
	}
	for _, keyword := range meta.Keywords {
		lines = append(lines, "- "+keyword)
	}
	if len(meta.Keywords) == 0 {
		lines = append(lines, "- (keywords will accumulate as this skill is reused)")
	}
	lines = append(lines, "", "## Recommended Tools")
	for _, toolName := range meta.Tools {
		lines = append(lines, "- "+toolName)
	}
	if len(meta.Tools) == 0 {
		lines = append(lines, "- (no stable tool pattern captured yet)")
	}
	lines = append(lines, "", "## Workflow Pattern")
	if len(meta.Tools) == 0 {
		lines = append(lines, "1. Inspect the workspace before making changes.", "2. Keep the edit scope local.", "3. Validate with the narrowest executable check available.")
	} else {
		for index, toolName := range meta.Tools {
			lines = append(lines, fmt.Sprintf("%d. Consider using %s when the task needs it.", index+1, toolName))
		}
		lines = append(lines, fmt.Sprintf("%d. Finish with a focused validation command before stopping.", len(meta.Tools)+1))
	}
	lines = append(lines, "", "## Recent Examples")
	if len(meta.Examples) == 0 {
		lines = append(lines, "No successful examples captured yet.")
	} else {
		for index, example := range meta.Examples {
			lines = append(lines,
				fmt.Sprintf("### Example %d", index+1),
				"Prompt: "+strings.TrimSpace(example.Prompt),
				"Response: "+strings.TrimSpace(example.Response),
			)
			if len(example.ToolNames) > 0 {
				lines = append(lines, "Tools: "+strings.Join(example.ToolNames, ", "))
			}
			if strings.TrimSpace(example.CreatedAt) != "" {
				lines = append(lines, "Captured: "+strings.TrimSpace(example.CreatedAt))
			}
			lines = append(lines, "")
		}
	}
	lines = append(lines,
		"## Metrics",
		fmt.Sprintf("- captures: %d", meta.CaptureCount),
		fmt.Sprintf("- selected: %d", meta.SelectedCount),
		fmt.Sprintf("- success: %d", meta.SuccessCount),
		fmt.Sprintf("- failure: %d", meta.FailureCount),
	)
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func calculateSkillScore(meta SkillMetadata) int {
	score := 20
	if meta.Auto {
		score += 5
	}
	score += minInt(30, meta.CaptureCount*8)
	score += minInt(20, meta.SuccessCount*5)
	score -= minInt(25, meta.FailureCount*8)
	score += minInt(10, len(meta.Tools)*2)
	score += minInt(10, len(meta.Keywords))
	if score < 1 {
		return 1
	}
	if score > 100 {
		return 100
	}
	return score
}

func sidecarPath(skillPath string) string {
	return filepath.Join(filepath.Dir(skillPath), skillMetadataFileName)
}

func compactExamples(examples []SkillExample, maxExamples int) []SkillExample {
	if maxExamples <= 0 {
		maxExamples = defaultAutoSkillExamples
	}
	trimmed := make([]SkillExample, 0, len(examples))
	seen := map[string]struct{}{}
	for _, example := range examples {
		key := strings.TrimSpace(example.Prompt) + "\n" + strings.TrimSpace(example.Response)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		example.ToolNames = compactStrings(example.ToolNames)
		trimmed = append(trimmed, example)
		if len(trimmed) == maxExamples {
			break
		}
	}
	return trimmed
}

func compactStrings(values []string) []string {
	trimmed := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		trimmed = append(trimmed, value)
	}
	return trimmed
}

func mergeOrdered(base, additions []string) []string {
	merged := append([]string(nil), compactStrings(base)...)
	seen := map[string]struct{}{}
	for _, value := range merged {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged
}

func overlapScore(left, right []string) int {
	set := map[string]struct{}{}
	for _, value := range compactStrings(left) {
		set[value] = struct{}{}
	}
	score := 0
	for _, value := range compactStrings(right) {
		if _, ok := set[value]; ok {
			score++
		}
	}
	return score
}

func prettySkillTitle(name string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(name), "autoskill-")
	trimmed = strings.ReplaceAll(trimmed, "-", " ")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "Auto Skill"
	}
	parts := strings.Fields(trimmed)
	for index, part := range parts {
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func sortedSkillsByScore(loaded []Skill) []Skill {
	ordered := append([]Skill(nil), loaded...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Metadata.Score == ordered[j].Metadata.Score {
			return ordered[i].Name < ordered[j].Name
		}
		return ordered[i].Metadata.Score > ordered[j].Metadata.Score
	})
	return ordered
}