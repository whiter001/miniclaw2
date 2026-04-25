package minimax

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"miniclaw2/internal/config"
	"miniclaw2/internal/mcp"
	"miniclaw2/internal/session"
	"miniclaw2/internal/skills"
	"miniclaw2/internal/tools"
)

const ToolIterationErrorPrefix = "tool iteration limit reached"

type ToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

func RunAgent(ctx context.Context, cfg config.Config, prompt string) (string, error) {
	recorder, err := session.New(cfg.Workspace)
	if err != nil {
		return "", err
	}
	if err := recorder.AppendMessage("message", "user", prompt); err != nil {
		return "", err
	}
	return RunAgentWithRecorder(ctx, cfg, prompt, recorder)
}

func RunAgentInSession(ctx context.Context, cfg config.Config, prompt string, recorder *session.Recorder) (string, error) {
	if err := recorder.AppendMessage("message", "user", prompt); err != nil {
		return "", err
	}
	response, err := RunAgentWithRecorder(ctx, cfg, prompt, recorder)
	if err != nil {
		return "", err
	}
	if err := AppendDailyMemoryEntry(cfg, prompt, response); err != nil {
		return response, nil
	}
	return response, nil
}

func RunAgentWithRecorder(ctx context.Context, cfg config.Config, prompt string, recorder *session.Recorder) (string, error) {
	client := NewClient(cfg)
	manager := mcp.NewManager(cfg)
	defer manager.StopAll()
	promptContext := BuildPromptContextForQuery(cfg, prompt)

	messages := []requestMessage{{
		Role: "user",
		Content: []requestContentBlock{{
			Type: "text",
			Text: prompt,
		}},
	}}
	lastAssistantText := ""
	lastToolUses := []ToolUse{}

	for iteration := 0; iteration < cfg.MaxToolIterations; iteration++ {
		response, err := client.Send(ctx, cfg, requestBody{
			Model:       cfg.Model,
			MaxTokens:   cfg.MaxTokens,
			Temperature: cfg.Temperature,
			TopP:        1.0,
			System:      promptContext.Prompt,
			Tools:       buildEffectiveToolSchema(manager),
			Messages:    messages,
		})
		if err != nil {
			_ = skills.UpdateSelectedSkillScores(cfg, promptContext.Skills, false)
			return "", err
		}
		assistantText := strings.TrimSpace(ExtractTextBlocks(response.Content))
		toolUses := extractToolUses(response.Content)
		lastAssistantText = assistantText
		lastToolUses = append([]ToolUse(nil), toolUses...)
		messages = append(messages, requestMessage{Role: "assistant", Content: responseToRequestBlocks(response.Content)})
		if len(toolUses) == 0 {
			if err := recorder.AppendMessage("message", "assistant", assistantText); err != nil {
				_ = skills.UpdateSelectedSkillScores(cfg, promptContext.Skills, false)
				return "", err
			}
			_ = skills.UpdateSelectedSkillScores(cfg, promptContext.Skills, true)
			_ = skills.AutoCaptureSession(cfg, recorder.FilePath, prompt, assistantText)
			return assistantText, nil
		}
		for _, toolUse := range toolUses {
			_ = recorder.AppendTool(toolUse.Name, toolUse.ID, "invoked", false)
			toolResult, err := executeEffectiveTool(ctx, toolUse, cfg, manager)
			if err != nil {
				message := "Error: " + err.Error()
				_ = recorder.AppendTool(toolUse.Name, toolUse.ID, message, true)
				messages = append(messages, buildToolResultMessage(toolUse, message, true))
				continue
			}
			_ = recorder.AppendTool(toolUse.Name, toolUse.ID, toolResult, false)
			messages = append(messages, buildToolResultMessage(toolUse, toolResult, false))
		}
	}
	_ = skills.UpdateSelectedSkillScores(cfg, promptContext.Skills, false)
	return "", fmt.Errorf("%s", buildToolIterationLimitError(cfg.MaxToolIterations, lastAssistantText, lastToolUses))
}

func buildEffectiveToolSchema(manager *mcp.Manager) []toolSchema {
	definitions := tools.Definitions()
	schemas := make([]toolSchema, 0, len(definitions)+len(manager.Tools()))
	for _, definition := range definitions {
		schemas = append(schemas, toolSchema{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema})
	}
	for _, definition := range manager.Tools() {
		schemas = append(schemas, toolSchema{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema})
	}
	return schemas
}

func extractToolUses(blocks []responseContentBlock) []ToolUse {
	toolUses := []ToolUse{}
	for _, block := range blocks {
		if block.Type != "tool_use" || block.ID == "" || block.Name == "" {
			continue
		}
		toolUses = append(toolUses, ToolUse{ID: block.ID, Name: block.Name, Input: block.Input})
	}
	return toolUses
}

func responseToRequestBlocks(blocks []responseContentBlock) []requestContentBlock {
	converted := make([]requestContentBlock, 0, len(blocks))
	for _, block := range blocks {
		converted = append(converted, requestContentBlock{Type: block.Type, Text: block.Text, ID: block.ID, Name: block.Name, Input: block.Input})
	}
	return converted
}

func executeEffectiveTool(ctx context.Context, toolUse ToolUse, cfg config.Config, manager *mcp.Manager) (string, error) {
	localInput := stringifyToolInput(toolUse.Input)
	result, err := tools.Execute(tools.ToolUse{ID: toolUse.ID, Name: toolUse.Name, Input: localInput}, cfg)
	if err == nil {
		return result, nil
	}
	if cfg.EnableMCP && manager.HasTool(toolUse.Name) {
		return manager.CallTool(ctx, toolUse.Name, toolUse.Input)
	}
	return "", err
}

func stringifyToolInput(input map[string]any) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		switch typed := value.(type) {
		case string:
			result[key] = typed
		default:
			data, err := json.Marshal(typed)
			if err != nil {
				result[key] = fmt.Sprint(typed)
			} else {
				result[key] = string(data)
			}
		}
	}
	return result
}

func buildToolResultMessage(toolUse ToolUse, result string, isError bool) requestMessage {
	return requestMessage{Role: "user", Content: []requestContentBlock{{Type: "tool_result", ToolUseID: toolUse.ID, Content: result, IsError: isError}}}
}

func buildToolIterationLimitError(iteration int, assistantText string, toolUses []ToolUse) string {
	details := []string{fmt.Sprintf("after %d rounds", iteration)}
	if names := summarizeToolUseNames(toolUses); names != "" {
		details = append(details, "last tools: "+names)
	}
	if preview := limitErrorPreview(assistantText); preview != "" {
		details = append(details, "last assistant text: "+preview)
	}
	return ToolIterationErrorPrefix + " (" + strings.Join(details, "; ") + ")"
}

func summarizeToolUseNames(toolUses []ToolUse) string {
	if len(toolUses) == 0 {
		return ""
	}
	names := []string{}
	for _, toolUse := range toolUses {
		if strings.TrimSpace(toolUse.Name) == "" {
			continue
		}
		names = append(names, toolUse.Name)
		if len(names) == 4 {
			break
		}
	}
	if len(names) == 0 {
		return ""
	}
	summary := strings.Join(names, ", ")
	if len(toolUses) > len(names) {
		summary += fmt.Sprintf(" +%d more", len(toolUses)-len(names))
	}
	return summary
}

func limitErrorPreview(value string) string {
	preview := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "\r", " "))
	if preview == "" {
		return ""
	}
	if len(preview) > 120 {
		return preview[:120] + "..."
	}
	return preview
}
