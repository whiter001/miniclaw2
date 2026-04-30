package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"miniclaw2/internal/memory"
)

const (
	defaultSelectedSkills = 2
	maxSkillContentChars  = 2000
)

var stopTokens = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "for": {}, "from": {},
	"in": {}, "into": {}, "of": {}, "on": {}, "or": {}, "please": {},
	"the": {}, "this": {}, "to": {}, "with": {},
}

type Skill struct {
	Name        string
	Description string
	Path        string
	Content     string
	Metadata    SkillMetadata
}

func Discover(directories ...string) []Skill {
	return discoverSkills(false, directories...)
}

func discoverSkills(includeInternal bool, directories ...string) []Skill {
	skills := []Skill{}
	seen := map[string]struct{}{}
	for _, directory := range directories {
		resolved := strings.TrimSpace(directory)
		if resolved == "" {
			continue
		}
		_ = filepath.WalkDir(resolved, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d == nil {
				return nil
			}
			if d.IsDir() {
				if !includeInternal && isInternalSkillDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.ToUpper(d.Name()) != "SKILL.MD" {
				return nil
			}
			if _, ok := seen[path]; ok {
				return nil
			}
			seen[path] = struct{}{}
			skill, readErr := readSkill(path)
			if readErr != nil {
				return nil
			}
			skills = append(skills, skill)
			return nil
		})
	}
	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	return skills
}

func isInternalSkillDir(name string) bool {
	return name == candidateSkillDirName || name == archivedSkillDirName
}

func Select(query string, maxSkills int, loaded []Skill) []Skill {
	if maxSkills <= 0 {
		maxSkills = defaultSelectedSkills
	}
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 || len(loaded) == 0 {
		return nil
	}
	type scoredSkill struct {
		skill Skill
		score int
	}
	results := make([]scoredSkill, 0, len(loaded))
	for _, skill := range loaded {
		score := scoreSkill(skill, queryTokens)
		if score <= 0 {
			continue
		}
		results = append(results, scoredSkill{skill: skill, score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].skill.Name < results[j].skill.Name
		}
		return results[i].score > results[j].score
	})
	if len(results) > maxSkills {
		results = results[:maxSkills]
	}
	selected := make([]Skill, 0, len(results))
	for _, item := range results {
		selected = append(selected, item.skill)
	}
	return selected
}

func BuildTurnContext(query string, maxSkills int, directories ...string) string {
	selected := Select(query, maxSkills, Discover(directories...))
	return BuildContextForQuery(query, selected)
}

func BuildContext(selected []Skill) string {
	return buildContext("", selected)
}

func BuildContextForQuery(query string, selected []Skill) string {
	return buildContext(query, selected)
}

func buildContext(query string, selected []Skill) string {
	if len(selected) == 0 {
		return ""
	}
	queryTokens := tokenize(query)
	lines := []string{"## Relevant Skills", "Use these skill notes when they are helpful for the current request."}
	for index, skill := range selected {
		headline := fmt.Sprintf("%d. %s", index+1, skill.Name)
		if skill.Metadata.Score > 0 {
			headline += fmt.Sprintf(" [score %d]", skill.Metadata.Score)
		}
		lines = append(lines, fmt.Sprintf("%s: %s\n%s", headline, skill.Description, memory.LimitText(promptSafeSkillContentForQuery(skill, queryTokens), maxSkillContentChars)))
	}
	return strings.Join(lines, "\n\n")
}

func promptSafeSkillContent(skill Skill) string {
	return promptSafeSkillContentForQuery(skill, nil)
}

func promptSafeSkillContentForQuery(skill Skill, queryTokens []string) string {
	if !skill.Metadata.Auto {
		return skill.Content
	}
	content := stripMarkdownSection(skill.Content, "## Recent Examples")
	if len(queryTokens) == 0 {
		return content
	}
	if selected := selectAutoSkillPromptSections(content, queryTokens); selected != "" {
		return selected
	}
	return content
}

func stripMarkdownSection(content, heading string) string {
	lines := strings.Split(content, "\n")
	filtered := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if skipping {
			if strings.HasPrefix(trimmed, "## ") {
				skipping = false
			} else {
				continue
			}
		}
		if strings.EqualFold(trimmed, heading) {
			skipping = true
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimRight(strings.Join(filtered, "\n"), "\n") + "\n"
}

type markdownSection struct {
	Heading string
	Text    string
}

func selectAutoSkillPromptSections(content string, queryTokens []string) string {
	content = stripMarkdownSection(content, "## Recent Captures")
	content = stripMarkdownSection(content, "## Metrics")
	sections := splitMarkdownSections(content)
	if len(sections) == 0 {
		return ""
	}
	preferred := map[string]struct{}{
		"## When To Use":       {},
		"## Decision Hints":    {},
		"## Procedure":         {},
		"## Watchouts":         {},
		"## Final Outcome":     {},
		"## Recommended Tools": {},
	}
	orderedPreferred := []string{"## When To Use", "## Decision Hints", "## Procedure", "## Watchouts", "## Final Outcome", "## Recommended Tools"}
	byHeading := map[string]markdownSection{}
	intro := ""
	for _, section := range sections {
		if section.Heading == "" {
			intro = strings.TrimSpace(section.Text)
			continue
		}
		byHeading[section.Heading] = section
	}
	parts := []string{}
	if intro != "" {
		parts = append(parts, intro)
	}
	for _, heading := range orderedPreferred {
		if section, ok := byHeading[heading]; ok {
			parts = append(parts, strings.TrimSpace(section.Text))
		}
	}
	for _, section := range sections {
		if section.Heading == "" {
			continue
		}
		if _, ok := preferred[section.Heading]; ok {
			continue
		}
		if sectionMatchesQuery(section.Text, queryTokens) {
			parts = append(parts, strings.TrimSpace(section.Text))
		}
	}
	if len(parts) <= 1 {
		return ""
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n")) + "\n"
}

func splitMarkdownSections(content string) []markdownSection {
	lines := strings.Split(content, "\n")
	sections := []markdownSection{}
	currentHeading := ""
	currentLines := []string{}
	flush := func() {
		text := strings.TrimRight(strings.Join(currentLines, "\n"), "\n")
		if strings.TrimSpace(text) == "" {
			currentLines = nil
			return
		}
		sections = append(sections, markdownSection{Heading: currentHeading, Text: text})
		currentLines = nil
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			currentHeading = trimmed
			currentLines = append(currentLines, line)
			continue
		}
		currentLines = append(currentLines, line)
	}
	flush()
	return sections
}

func sectionMatchesQuery(text string, queryTokens []string) bool {
	if len(queryTokens) == 0 {
		return false
	}
	lower := strings.ToLower(text)
	for _, token := range queryTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func readSkill(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	content := string(data)
	directory := filepath.Base(filepath.Dir(path))
	description := firstParagraph(content)
	if description == "" {
		description = directory
	}
	return Skill{
		Name:        directory,
		Description: description,
		Path:        path,
		Content:     content,
		Metadata:    loadOptionalMetadata(sidecarPath(path)),
	}, nil
}

func loadOptionalMetadata(path string) SkillMetadata {
	meta, err := readSkillMetadata(path)
	if err != nil {
		return SkillMetadata{}
	}
	return meta
}

func firstParagraph(content string) string {
	lines := strings.Split(content, "\n")
	collecting := false
	parts := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if collecting && len(parts) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") && !collecting {
			continue
		}
		collecting = true
		parts = append(parts, trimmed)
		if len(parts) >= 2 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func scoreSkill(skill Skill, tokens []string) int {
	score := 0
	score += scoreTextField(skill.Name, tokens, 6)
	score += scoreTextField(skill.Description, tokens, 4)
	score += scoreTextField(strings.Join(skill.Metadata.Keywords, " "), tokens, 5)
	score += scoreTextField(strings.Join(skill.Metadata.Tools, " "), tokens, 4)
	score += scoreTextField(metadataGuidanceText(skill.Metadata), tokens, 3)
	score += scoreTextField(scoringSafeSkillContent(skill), tokens, 1)
	if score == 0 {
		return 0
	}
	score += skill.Metadata.Score / 20
	if skill.Metadata.SuccessCount > skill.Metadata.FailureCount {
		score += minInt(3, skill.Metadata.SuccessCount-skill.Metadata.FailureCount)
	}
	return score
}

func scoreTextField(text string, tokens []string, weight int) int {
	if weight <= 0 || len(tokens) == 0 || strings.TrimSpace(text) == "" {
		return 0
	}
	lower := strings.ToLower(text)
	fieldTokens := tokenSet(tokenize(text))
	score := 0
	for _, token := range tokens {
		if _, ok := fieldTokens[token]; ok {
			score += weight
			continue
		}
		if allowsSubstringMatch(token) && strings.Contains(lower, token) {
			score += weight
		}
	}
	return score
}

func tokenSet(tokens []string) map[string]struct{} {
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[token] = struct{}{}
	}
	return set
}

func allowsSubstringMatch(token string) bool {
	for _, char := range token {
		if char > 127 {
			return true
		}
	}
	return false
}

func metadataGuidanceText(meta SkillMetadata) string {
	parts := []string{}
	parts = append(parts, meta.DecisionHints...)
	parts = append(parts, meta.Procedure...)
	parts = append(parts, meta.Watchouts...)
	parts = append(parts, meta.FinalOutcome)
	return strings.Join(parts, " ")
}

func scoringSafeSkillContent(skill Skill) string {
	content := promptSafeSkillContent(skill)
	if !skill.Metadata.Auto {
		return content
	}
	content = stripMarkdownSection(content, "## Recent Captures")
	content = stripMarkdownSection(content, "## Metrics")
	return content
}

func tokenize(query string) []string {
	query = strings.ToLower(query)
	fields := strings.FieldsFunc(query, func(char rune) bool {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			return false
		}
		return char != '_' && char != '-'
	})
	tokens := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		if field == "" {
			continue
		}
		if _, blocked := stopTokens[field]; blocked {
			continue
		}
		if len([]rune(field)) < 2 && field != "go" && field != "c" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		tokens = append(tokens, field)
	}
	return tokens
}
