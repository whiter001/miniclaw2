package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/provider/minimax"
	"miniclaw2/internal/session"
)

var eventLogMu sync.Mutex
var seenMessageMu sync.Mutex

type TextMessage struct {
	Channel   string
	Scene     string
	MessageID string
	Prompt    string
	Payload   map[string]any
}

type SendReplyFunc func(ctx context.Context, content string, sequence int) error

type ProcessOptions struct {
	Config         config.Config
	Message        TextMessage
	ModelPrompt    string
	ProcessingText string
	SendReply      SendReplyFunc
}

func ProcessTextMessage(ctx context.Context, options ProcessOptions) error {
	channel := sanitizeChannelName(options.Message.Channel)
	if channel == "" {
		return fmt.Errorf("channel is empty")
	}
	if options.SendReply == nil {
		return fmt.Errorf("send reply is nil")
	}
	seen, err := MarkMessageSeen(options.Config, channel, options.Message.MessageID)
	if err != nil {
		return err
	}
	if !seen {
		return AppendEventLog(options.Config, channel, "event_duplicate", mergePayload(options.Message.Payload, "scene", options.Message.Scene, "msg_id", options.Message.MessageID))
	}
	prompt := strings.TrimSpace(options.Message.Prompt)
	if prompt == "" {
		return nil
	}
	modelPrompt := strings.TrimSpace(options.ModelPrompt)
	if modelPrompt == "" {
		modelPrompt = prompt
	}
	if strings.TrimSpace(options.ProcessingText) != "" {
		if err := options.SendReply(ctx, options.ProcessingText, 1); err != nil {
			_ = AppendEventLog(options.Config, channel, "reply_placeholder_error", mergePayload(options.Message.Payload, "scene", options.Message.Scene, "msg_id", options.Message.MessageID, "error", err.Error()))
		}
	}
	recorder, err := session.New(options.Config.Workspace)
	if err != nil {
		return err
	}
	if err := recorder.AppendMessage("message", "user", prompt); err != nil {
		return err
	}
	response, err := minimax.RunAgentWithRecorder(ctx, options.Config, modelPrompt, recorder)
	if err != nil {
		failureMessage := BuildFailureMessage(err.Error())
		_ = AppendAgentErrorLog(options.Config, channel, options.Message, recorder.SessionID, prompt, err.Error())
		_ = options.SendReply(ctx, failureMessage, 2)
		return err
	}
	_ = minimax.AppendDailyMemoryEntry(options.Config, prompt, response)
	if err := options.SendReply(ctx, response, 2); err != nil {
		return err
	}
	return AppendEventLog(options.Config, channel, "reply_sent", mergePayload(options.Message.Payload, "scene", options.Message.Scene, "msg_id", options.Message.MessageID, "content", response))
}

func AppendAgentErrorLog(cfg config.Config, channel string, message TextMessage, sessionID, prompt, errorMessage string) error {
	kind := "event_error"
	if strings.Contains(errorMessage, minimax.ToolIterationErrorPrefix) {
		kind = "event_tool_iteration_limit"
	}
	payload := mergePayload(message.Payload,
		"scene", message.Scene,
		"msg_id", message.MessageID,
		"session_id", sessionID,
		"prompt", limitErrorPreview(prompt),
		"error", errorMessage,
	)
	return AppendEventLog(cfg, channel, kind, payload)
}

func MarkMessageSeen(cfg config.Config, channel, msgID string) (bool, error) {
	trimmedID := strings.TrimSpace(msgID)
	if trimmedID == "" {
		return true, nil
	}
	path := filepath.Join(cfg.Workspace, "state", sanitizeChannelName(channel)+"_seen_message_ids.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	seenMessageMu.Lock()
	defer seenMessageMu.Unlock()
	data, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == trimmedID {
			return false, nil
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer file.Close()
	if _, err := file.WriteString(trimmedID + "\n"); err != nil {
		return false, err
	}
	return true, nil
}

func AppendEventLog(cfg config.Config, channel, kind string, payload any) error {
	path := filepath.Join(cfg.Workspace, "state", sanitizeChannelName(channel)+"_webhook_events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := map[string]any{"ts": time.Now().Format(time.RFC3339Nano), "kind": kind}
	switch typed := payload.(type) {
	case nil:
	case []byte:
		if json.Valid(typed) {
			entry["payload"] = json.RawMessage(typed)
		} else {
			entry["payload"] = string(typed)
		}
	default:
		entry["payload"] = typed
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	eventLogMu.Lock()
	defer eventLogMu.Unlock()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func BuildFailureMessage(errorMessage string) string {
	if strings.Contains(errorMessage, minimax.ToolIterationErrorPrefix) {
		return "这个问题触发了过多工具调用，我没能在限定步数内完成。请把问题拆小一点，或直接说明要查看的文件、目录或命令。"
	}
	return "处理失败，请稍后重试。"
}

func mergePayload(base map[string]any, keyValues ...any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for index := 0; index+1 < len(keyValues); index += 2 {
		key, ok := keyValues[index].(string)
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		merged[key] = keyValues[index+1]
	}
	return merged
}

func sanitizeChannelName(channel string) string {
	trimmed := strings.ToLower(strings.TrimSpace(channel))
	if trimmed == "" {
		return "gateway"
	}
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	return builder.String()
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
