package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"miniclaw2/internal/config"
	localtools "miniclaw2/internal/tools"
)

func SkillRoot(cfg config.Config) string {
	return filepath.Join(cfg.Workspace, "skills")
}

func ListSkills(cfg config.Config) []Skill {
	return sortedSkillsByScore(Discover(SkillRoot(cfg)))
}

func FindSkill(cfg config.Config, rawName string) (Skill, error) {
	requested := NormalizeSkillName(rawName)
	if requested == "" {
		return Skill{}, fmt.Errorf("skill name is required")
	}
	for _, skill := range Discover(SkillRoot(cfg)) {
		if strings.EqualFold(skill.Name, requested) || strings.EqualFold(skill.Metadata.Slug, requested) {
			return skill, nil
		}
	}
	return Skill{}, fmt.Errorf("skill not found: %s", requested)
}

func CreateSkill(cfg config.Config, rawName, content string, overwrite bool) (Skill, error) {
	name := NormalizeSkillName(rawName)
	if name == "" {
		return Skill{}, fmt.Errorf("skill name is required")
	}
	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" {
		return Skill{}, fmt.Errorf("skill content is required")
	}
	path := filepath.Join(SkillRoot(cfg), name, "SKILL.md")
	if _, err := os.Stat(path); err == nil && !overwrite {
		return Skill{}, fmt.Errorf("skill already exists: %s", name)
	}
	document := ensureSkillDocument(name, trimmedContent)
	now := time.Now().Format(time.RFC3339)
	meta := loadOptionalMetadata(sidecarPath(path))
	if meta.CreatedAt == "" {
		meta.CreatedAt = now
	}
	meta.Slug = name
	meta.Auto = false
	meta.UpdatedAt = now
	meta.Keywords = mergeOrdered(meta.Keywords, tokenize(name+" "+firstParagraph(document)+" "+document))
	meta.Tools = mergeOrdered(meta.Tools, detectMentionedTools(document))
	meta.Score = calculateSkillScore(meta)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Skill{}, err
	}
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		return Skill{}, err
	}
	if err := writeSkillMetadata(sidecarPath(path), meta); err != nil {
		return Skill{}, err
	}
	return readSkill(path)
}

func OptimizeSkill(cfg config.Config, rawName string) (Skill, error) {
	skill, err := FindSkill(cfg, rawName)
	if err != nil {
		return Skill{}, err
	}
	now := time.Now().Format(time.RFC3339)
	meta := skill.Metadata
	meta.Slug = NormalizeSkillName(skill.Name)
	if meta.CreatedAt == "" {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now
	meta.Keywords = mergeOrdered(meta.Keywords, tokenize(skill.Name+" "+skill.Description+" "+skill.Content))
	meta.Tools = mergeOrdered(meta.Tools, detectMentionedTools(skill.Content))
	meta.Score = calculateSkillScore(meta)
	if meta.Auto {
		if err := os.WriteFile(skill.Path, []byte(renderAutoSkill(skill.Name, meta)), 0o644); err != nil {
			return Skill{}, err
		}
	}
	if err := writeSkillMetadata(sidecarPath(skill.Path), meta); err != nil {
		return Skill{}, err
	}
	return readSkill(skill.Path)
}

func DeleteSkill(cfg config.Config, rawName string) error {
	skill, err := FindSkill(cfg, rawName)
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Dir(skill.Path))
}

func NormalizeSkillName(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r) || r == '/' || r == '.':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func ensureSkillDocument(name, content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "#") {
		trimmed = "# " + prettySkillTitle(name) + "\n\n" + trimmed
	}
	return strings.TrimRight(trimmed, "\n") + "\n"
}

func detectMentionedTools(content string) []string {
	text := strings.ToLower(content)
	found := []string{}
	for _, definition := range localtools.Definitions() {
		name := strings.TrimSpace(definition.Name)
		if name == "" {
			continue
		}
		if strings.Contains(text, strings.ToLower(name)) {
			found = append(found, name)
		}
	}
	return compactStrings(found)
}
