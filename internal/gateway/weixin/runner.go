package weixin

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"miniclaw2/internal/config"
	gatewayruntime "miniclaw2/internal/gateway/runtime"
)

type Runner struct{}

func (Runner) Name() string {
	return "weixin"
}

func (Runner) Bootstrap(ctx context.Context, cfg config.Config) ([]string, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	account := client.Account()
	return []string{
		"Weixin gateway bootstrap ok.",
		"account id: " + firstNonEmpty(account.RawAccountID, account.ID),
		"api base: " + account.BaseURL,
		"mode: long-polling text MVP",
	}, nil
}

func (Runner) Start(ctx context.Context, cfg config.Config) error {
	client, err := NewClient(cfg)
	if err != nil {
		return err
	}
	account := client.Account()
	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	cursor := loadCursor(cfg, account.ID)
	backoff := time.Second
	for {
		select {
		case <-shutdownCtx.Done():
			return nil
		default:
		}
		response, err := client.GetUpdates(shutdownCtx, cursor)
		if err != nil {
			if shutdownCtx.Err() != nil {
				return nil
			}
			fmt.Fprintf(os.Stderr, "weixin poll error: %v\n", err)
			if !sleepWithContext(shutdownCtx, backoff) {
				return nil
			}
			if backoff < 5*time.Second {
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
			}
			continue
		}
		backoff = time.Second
		if strings.TrimSpace(response.GetUpdatesBuf) != "" && response.GetUpdatesBuf != cursor {
			cursor = response.GetUpdatesBuf
			_ = saveCursor(cfg, account.ID, cursor)
		}
		for _, message := range ExtractTextMessages(response.Msgs) {
			payload := map[string]any{
				"msg_id":        message.MessageID,
				"from_user_id":  message.SenderUserID,
				"context_token": message.ContextToken,
				"session_id":    message.SessionID,
			}
			if !IsUserAllowed(cfg, message.SenderUserID) {
				_ = gatewayruntime.AppendEventLog(cfg, "weixin", "event_blocked", payload)
				continue
			}
			err := gatewayruntime.ProcessTextMessage(shutdownCtx, gatewayruntime.ProcessOptions{
				Config: cfg,
				Message: gatewayruntime.TextMessage{
					Channel:   "weixin",
					Scene:     "dm",
					MessageID: message.MessageID,
					Prompt:    message.Prompt,
					Payload:   payload,
				},
				ModelPrompt:    buildWeixinModelPrompt(message.Prompt),
				ProcessingText: cfg.WeixinProcessingText,
				SendReply: func(ctx context.Context, content string, sequence int) error {
					return client.SendRenderedReply(ctx, content, message.SenderUserID, message.ContextToken, sequence)
				},
			})
			if err != nil {
				_ = gatewayruntime.AppendEventLog(cfg, "weixin", "event_error", map[string]any{
					"msg_id":       message.MessageID,
					"from_user_id": message.SenderUserID,
					"error":        err.Error(),
				})
			}
		}
	}
}

func (Runner) StartMessages(cfg config.Config) []string {
	account, err := ResolveAccount(cfg)
	if err != nil {
		return []string{"starting weixin long-poll gateway"}
	}
	return []string{
		"starting weixin long-poll gateway against " + account.BaseURL,
		"media replies are enabled via MEDIA: directives in the final agent response.",
	}
}

func loadCursor(cfg config.Config, accountID string) string {
	path := resolveCursorPath(cfg, accountID)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func saveCursor(cfg config.Config, accountID, cursor string) error {
	path := resolveCursorPath(cfg, accountID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(cursor)+"\n"), 0o644)
}

func resolveCursorPath(cfg config.Config, accountID string) string {
	name := NormalizeAccountID(accountID)
	if name == "" {
		name = "default"
	}
	return filepath.Join(resolveWeixinStateDir(cfg), name+".sync.txt")
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func buildWeixinModelPrompt(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return trimmed
	}
	instructions := "\n\nChannel note: you are replying in a Weixin direct-message channel. If you need to send an image or file, put each media source on its own final line as MEDIA:<absolute local path or https URL>. Keep any caption text on separate lines outside MEDIA directives."
	return trimmed + instructions
}
