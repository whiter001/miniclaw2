package runtime

import (
	"context"
	"fmt"
	"strings"

	"miniclaw2/internal/config"
	"miniclaw2/internal/skills"
)

type skillCommand struct {
	Action string
	Name   string
	Body   string
}

func handleSkillCommand(ctx context.Context, cfg config.Config, prompt string, sendReply SendReplyFunc) (bool, error) {
	command, ok := parseSkillCommand(prompt)
	if !ok {
		return false, nil
	}
	response, err := executeSkillCommand(cfg, command)
	if err != nil {
		response = "skill command failed: " + err.Error() + "\n\n" + skillCommandHelp()
	}
	if err := sendReply(ctx, response, 1); err != nil {
		return true, err
	}
	return true, nil
}

func parseSkillCommand(prompt string) (skillCommand, bool) {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return skillCommand{}, false
	}
	lines := strings.Split(trimmed, "\n")
	firstLine := strings.TrimSpace(lines[0])
	fields := strings.Fields(firstLine)
	if len(fields) == 0 {
		return skillCommand{}, false
	}
	if fields[0] != "/skill" && fields[0] != "/skills" {
		return skillCommand{}, false
	}
	command := skillCommand{Action: "help"}
	if len(fields) > 1 {
		command.Action = strings.ToLower(strings.TrimSpace(fields[1]))
	}
	if len(fields) > 2 {
		command.Name = strings.Join(fields[2:], " ")
	}
	if len(lines) > 1 {
		command.Body = strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	return command, true
}

func executeSkillCommand(cfg config.Config, command skillCommand) (string, error) {
	switch command.Action {
	case "help", "":
		return skillCommandHelp(), nil
	case "list", "ls":
		return formatSkillList(skills.ListSkills(cfg)), nil
	case "show", "get":
		skill, err := skills.FindSkill(cfg, command.Name)
		if err != nil {
			return "", err
		}
		return formatSkillDetails(skill), nil
	case "add":
		skill, err := skills.CreateSkill(cfg, command.Name, command.Body, false)
		if err != nil {
			return "", err
		}
		return "skill added\n\n" + formatSkillDetails(skill), nil
	case "update", "set":
		skill, err := skills.CreateSkill(cfg, command.Name, command.Body, true)
		if err != nil {
			return "", err
		}
		return "skill updated\n\n" + formatSkillDetails(skill), nil
	case "optimize", "refresh":
		skill, err := skills.OptimizeSkill(cfg, command.Name)
		if err != nil {
			return "", err
		}
		return "skill optimized\n\n" + formatSkillDetails(skill), nil
	case "delete", "remove", "rm":
		name := skills.NormalizeSkillName(command.Name)
		if name == "" {
			return "", fmt.Errorf("skill name is required")
		}
		if err := skills.DeleteSkill(cfg, name); err != nil {
			return "", err
		}
		return "skill deleted: " + name, nil
		default:
		return skillCommandHelp(), nil
	}
}

func skillCommandHelp() string {
	return strings.Join([]string{
		"skill commands:",
		"/skill list",
		"/skill show <name>",
		"/skill add <name> then put skill content on following lines",
		"/skill update <name> then put replacement content on following lines",
		"/skill optimize <name>",
		"/skill delete <name>",
	}, "\n")
}

func formatSkillList(list []skills.Skill) string {
	if len(list) == 0 {
		return "no skills found"
	}
	lines := []string{"skills:"}
	for _, skill := range list {
		line := "- " + skill.Name
		if strings.TrimSpace(skill.Description) != "" {
			line += ": " + skill.Description
		}
		if skill.Metadata.Score > 0 {
			line += fmt.Sprintf(" [score %d]", skill.Metadata.Score)
		}
		if skill.Metadata.Auto {
			line += " [auto]"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatSkillDetails(skill skills.Skill) string {
	lines := []string{
		"name: " + skill.Name,
	}
	if strings.TrimSpace(skill.Description) != "" {
		lines = append(lines, "description: "+skill.Description)
	}
	if skill.Metadata.Score > 0 {
		lines = append(lines, fmt.Sprintf("score: %d", skill.Metadata.Score))
	}
	if skill.Metadata.Auto {
		lines = append(lines, "mode: auto")
	} else {
		lines = append(lines, "mode: manual")
	}
	if len(skill.Metadata.Tools) > 0 {
		lines = append(lines, "tools: "+strings.Join(skill.Metadata.Tools, ", "))
	}
	if len(skill.Metadata.Keywords) > 0 {
		lines = append(lines, "keywords: "+strings.Join(skill.Metadata.Keywords, ", "))
	}
	lines = append(lines, "", strings.TrimSpace(skill.Content))
	return strings.Join(lines, "\n")
}