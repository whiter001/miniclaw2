package qq

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"miniclaw2/internal/config"
	"miniclaw2/internal/provider/minimax"
	"miniclaw2/internal/session"
)

var eventLogMu sync.Mutex
var seenMessageMu sync.Mutex

type WebhookHandler struct {
	Config config.Config
	sem    chan struct{}
}

type eventEnvelope struct {
	Op int             `json:"op"`
	T  string          `json:"t"`
	D  json.RawMessage `json:"d"`
}

type messageEvent struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	GroupOpenID string `json:"group_openid"`
	Author      struct {
		UserOpenID string `json:"user_openid"`
	} `json:"author"`
}

type validationData struct {
	PlainToken string `json:"plain_token"`
	EventTS    string `json:"event_ts"`
}

func NewWebhookHandler(cfg config.Config) *WebhookHandler {
	return &WebhookHandler{Config: cfg, sem: make(chan struct{}, 8)}
}

func StartWebhookServer(cfg config.Config) error {
	handler := NewWebhookHandler(cfg)
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.QQWebhookHost, cfg.QQWebhookPort),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownCtx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestPath := r.URL.Path
	if requestPath == h.Config.QQAuthCallbackPath {
		if r.Method != http.MethodGet {
			writePlain(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleAuthCallback(w, r)
		return
	}
	if requestPath != h.Config.QQWebhookPath {
		writePlain(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		writePlain(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var envelope eventEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if envelope.Op == 13 {
		response, err := BuildValidationResponse(h.Config, body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		_ = AppendEventLog(h.Config, "validation", body)
		writeJSONBytes(w, http.StatusOK, response)
		return
	}
	_ = AppendEventLog(h.Config, "event", body)
	go h.processEventAsync(body)
	writeJSON(w, http.StatusOK, map[string]int{"op": 12})
}

func (h *WebhookHandler) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	payload := map[string]string{"url": r.URL.String(), "query": r.URL.RawQuery}
	data, _ := json.Marshal(payload)
	_ = AppendEventLog(h.Config, "auth_callback", data)
	body := "<!doctype html><html><head><meta charset=\"utf-8\"><title>MiniClaw QQ Callback</title></head><body><h1>MiniClaw QQ Callback</h1><p>网页授权回调已到达。</p><pre>" + html.EscapeString(r.URL.String()) + "</pre></body></html>"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (h *WebhookHandler) processEventAsync(payload []byte) {
	h.sem <- struct{}{}
	defer func() { <-h.sem }()
	if err := HandleMessageEvent(context.Background(), h.Config, payload); err != nil {
		_ = AppendEventLog(h.Config, "event_error", []byte(fmt.Sprintf(`{"error":%q}`, err.Error())))
	}
}

func HandleMessageEvent(ctx context.Context, cfg config.Config, payload []byte) error {
	target, err := ExtractReplyTarget(payload)
	if err != nil || target.Scene == "" {
		return err
	}
	if !IsTargetAllowed(cfg, target) {
		return AppendEventLog(cfg, "event_blocked", []byte(fmt.Sprintf(`{"scene":%q,"msg_id":%q}`, target.Scene, target.MsgID)))
	}
	seen, err := MarkMessageSeen(cfg, target.MsgID)
	if err != nil {
		return err
	}
	if !seen {
		return AppendEventLog(cfg, "event_duplicate", []byte(fmt.Sprintf(`{"scene":%q,"msg_id":%q}`, target.Scene, target.MsgID)))
	}
	prompt, err := ExtractEventPrompt(payload)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" {
		return nil
	}
	token, err := FetchAccessToken(ctx, cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.QQProcessingText) != "" {
		if _, err := SendReply(ctx, cfg, token.AccessToken, target, cfg.QQProcessingText, 1); err != nil {
			_ = AppendEventLog(cfg, "reply_placeholder_error", []byte(fmt.Sprintf(`{"scene":%q,"msg_id":%q,"error":%q}`, target.Scene, target.MsgID, err.Error())))
		}
	}
	recorder, err := session.New(cfg.Workspace)
	if err != nil {
		return err
	}
	response, err := minimax.RunAgentInSession(ctx, cfg, prompt, recorder)
	if err != nil {
		failureMessage := BuildFailureMessage(err.Error())
		_ = AppendAgentErrorLog(cfg, target, recorder.SessionID, prompt, err.Error())
		_, _ = SendReply(ctx, cfg, token.AccessToken, target, failureMessage, 2)
		return err
	}
	if _, err := SendReply(ctx, cfg, token.AccessToken, target, response, 2); err != nil {
		return err
	}
	return AppendEventLog(cfg, "reply_sent", []byte(fmt.Sprintf(`{"scene":%q,"msg_id":%q,"content":%q}`, target.Scene, target.MsgID, response)))
}

func BuildValidationResponse(cfg config.Config, payload []byte) ([]byte, error) {
	var envelope eventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}
	var data validationData
	if err := json.Unmarshal(envelope.D, &data); err != nil {
		return nil, err
	}
	if strings.TrimSpace(data.PlainToken) == "" || strings.TrimSpace(data.EventTS) == "" {
		return nil, fmt.Errorf("invalid validation payload")
	}
	signature, err := SignValidation(cfg.QQAppSecret, data.EventTS, data.PlainToken)
	if err != nil {
		return nil, err
	}
	response := map[string]string{"plain_token": data.PlainToken, "signature": signature}
	return json.Marshal(response)
}

func ExtractReplyTarget(payload []byte) (ReplyTarget, error) {
	var envelope eventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return ReplyTarget{}, err
	}
	var event messageEvent
	if err := json.Unmarshal(envelope.D, &event); err != nil {
		return ReplyTarget{}, err
	}
	switch envelope.T {
	case "C2C_MESSAGE_CREATE":
		return ReplyTarget{Scene: "c2c", OpenID: event.Author.UserOpenID, MsgID: event.ID}, nil
	case "GROUP_AT_MESSAGE_CREATE":
		return ReplyTarget{Scene: "group", GroupOpenID: event.GroupOpenID, MsgID: event.ID}, nil
	default:
		return ReplyTarget{}, nil
	}
}

func ExtractEventPrompt(payload []byte) (string, error) {
	var envelope eventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", err
	}
	var event messageEvent
	if err := json.Unmarshal(envelope.D, &event); err != nil {
		return "", err
	}
	return strings.TrimSpace(event.Content), nil
}

func MarkMessageSeen(cfg config.Config, msgID string) (bool, error) {
	trimmedID := strings.TrimSpace(msgID)
	if trimmedID == "" {
		return true, nil
	}
	path := filepath.Join(cfg.Workspace, "state", "qq_seen_message_ids.txt")
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

func AppendAgentErrorLog(cfg config.Config, target ReplyTarget, sessionID, prompt, errorMessage string) error {
	kind := "event_error"
	if strings.Contains(errorMessage, minimax.ToolIterationErrorPrefix) {
		kind = "event_tool_iteration_limit"
	}
	payload := []byte(fmt.Sprintf(`{"scene":%q,"msg_id":%q,"session_id":%q,"prompt":%q,"error":%q}`, target.Scene, target.MsgID, sessionID, limitErrorPreview(prompt), errorMessage))
	return AppendEventLog(cfg, kind, payload)
}

func AppendEventLog(cfg config.Config, kind string, payload []byte) error {
	path := filepath.Join(cfg.Workspace, "state", "qq_webhook_events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := map[string]any{"ts": time.Now().Format(time.RFC3339Nano), "kind": kind}
	if json.Valid(payload) {
		entry["payload"] = json.RawMessage(payload)
	} else {
		entry["payload"] = string(payload)
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, _ := json.Marshal(value)
	writeJSONBytes(w, status, data)
}

func writeJSONBytes(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
