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
			if d == nil || d.IsDir() || strings.ToUpper(d.Name()) != "SKILL.MD" {
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
	return BuildContext(selected)
}

func BuildContext(selected []Skill) string {
	if len(selected) == 0 {
		return ""
	}
	lines := []string{"## Relevant Skills", "Use these skill notes when they are helpful for the current request."}
	for index, skill := range selected {
		headline := fmt.Sprintf("%d. %s", index+1, skill.Name)
		if skill.Metadata.Score > 0 {
			headline += fmt.Sprintf(" [score %d]", skill.Metadata.Score)
		}
		lines = append(lines, fmt.Sprintf("%s: %s\n%s", headline, skill.Description, memory.LimitText(skill.Content, maxSkillContentChars)))
	}
	return strings.Join(lines, "\n\n")
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
	text := strings.ToLower(skill.Name + " " + skill.Description + " " + skill.Content + " " + strings.Join(skill.Metadata.Keywords, " "))
	score := 0
	for _, token := range tokens {
		if strings.Contains(text, token) {
			score++
		}
	}
	score += skill.Metadata.Score / 20
	if skill.Metadata.SuccessCount > skill.Metadata.FailureCount {
		score += minInt(3, skill.Metadata.SuccessCount-skill.Metadata.FailureCount)
	}
	return score
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