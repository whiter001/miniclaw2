package qq

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"miniclaw2/internal/config"
)

func TestCSVContainsValue(t *testing.T) {
	if !CSVContainsValue("alpha,beta,gamma", "beta") {
		t.Fatal("expected beta to be found")
	}
	if !CSVContainsValue("alpha, beta ,gamma", "beta") {
		t.Fatal("expected trimmed beta to be found")
	}
	if CSVContainsValue("alpha,beta,gamma", "delta") {
		t.Fatal("did not expect delta to be found")
	}
}

func TestIsTargetAllowed(t *testing.T) {
	cfg := config.Config{QQAllowUsers: "u-1,u-2", QQAllowGroups: "g-1,g-2"}
	if !IsTargetAllowed(cfg, ReplyTarget{Scene: "c2c", OpenID: "u-2"}) {
		t.Fatal("expected user to be allowed")
	}
	if IsTargetAllowed(cfg, ReplyTarget{Scene: "c2c", OpenID: "u-9"}) {
		t.Fatal("did not expect unknown user to be allowed")
	}
	if !IsTargetAllowed(cfg, ReplyTarget{Scene: "group", GroupOpenID: "g-1"}) {
		t.Fatal("expected group to be allowed")
	}
}

func TestMarkMessageSeen(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Config{Workspace: workspace}
	first, err := MarkMessageSeen(cfg, "msg-1")
	if err != nil || !first {
		t.Fatalf("expected first mark to succeed, got %v %v", first, err)
	}
	second, err := MarkMessageSeen(cfg, "msg-1")
	if err != nil || second {
		t.Fatalf("expected second mark to be duplicate, got %v %v", second, err)
	}
}

func TestExtractReplyTargetAndPromptForC2C(t *testing.T) {
	payload := []byte(`{"t":"C2C_MESSAGE_CREATE","d":{"id":"msg-1","content":" hello ","author":{"user_openid":"user-1"}}}`)
	target, err := ExtractReplyTarget(payload)
	if err != nil {
		t.Fatal(err)
	}
	if target.Scene != "c2c" || target.OpenID != "user-1" || target.MsgID != "msg-1" {
		t.Fatalf("unexpected target: %+v", target)
	}
	prompt, err := ExtractEventPrompt(payload)
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "hello" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestExtractReplyTargetAndPromptForGroup(t *testing.T) {
	payload := []byte(`{"t":"GROUP_AT_MESSAGE_CREATE","d":{"id":"msg-2","content":" ping ","group_openid":"group-1"}}`)
	target, err := ExtractReplyTarget(payload)
	if err != nil {
		t.Fatal(err)
	}
	if target.Scene != "group" || target.GroupOpenID != "group-1" || target.MsgID != "msg-2" {
		t.Fatalf("unexpected target: %+v", target)
	}
	prompt, err := ExtractEventPrompt(payload)
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "ping" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestWebhookGetRejectedForMessagePath(t *testing.T) {
	handler := NewWebhookHandler(config.Config{QQWebhookPath: "/webhook/qq"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/webhook/qq", nil)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if strings.TrimSpace(recorder.Body.String()) != "method not allowed" {
		t.Fatalf("unexpected body: %q", recorder.Body.String())
	}
}

func TestWebhookValidationReturnsSignature(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	handler := NewWebhookHandler(config.Config{Workspace: workspace, QQAppSecret: "secret-value", QQWebhookPath: "/webhook/qq"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webhook/qq", strings.NewReader(`{"op":13,"d":{"plain_token":"plain-token","event_ts":"1700000000"}}`))
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["plain_token"] != "plain-token" || payload["signature"] == "" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestWebhookEventRejectsMissingSignature(t *testing.T) {
	workspace := t.TempDir()
	handler := NewWebhookHandler(config.Config{Workspace: workspace, QQAppSecret: "secret-value", QQWebhookPath: "/webhook/qq"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/webhook/qq", strings.NewReader(`{"t":"READY","d":{}}`))
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "signature timestamp") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestWebhookEventRejectsExpiredSignature(t *testing.T) {
	body := []byte(`{"t":"READY","d":{}}`)
	timestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	signature, err := signQQWebhookPayload("secret-value", timestamp, body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/webhook/qq", strings.NewReader(string(body)))
	request.Header.Set("X-Signature-Timestamp", timestamp)
	request.Header.Set("X-Signature-Ed25519", signature)

	workspace := t.TempDir()
	handler := NewWebhookHandler(config.Config{Workspace: workspace, QQAppSecret: "secret-value", QQWebhookPath: "/webhook/qq"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "allowed window") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestWebhookEventRejectsInvalidSignature(t *testing.T) {
	signedBody := []byte(`{"t":"READY","d":{}}`)
	actualBody := []byte(`{"t":"READY","d":{"tampered":true}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature, err := signQQWebhookPayload("secret-value", timestamp, signedBody)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/webhook/qq", strings.NewReader(string(actualBody)))
	request.Header.Set("X-Signature-Timestamp", timestamp)
	request.Header.Set("X-Signature-Ed25519", signature)

	workspace := t.TempDir()
	handler := NewWebhookHandler(config.Config{Workspace: workspace, QQAppSecret: "secret-value", QQWebhookPath: "/webhook/qq"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "invalid qq webhook signature") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestWebhookEventAcceptsValidSignature(t *testing.T) {
	body := []byte(`{"t":"READY","d":{}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature, err := signQQWebhookPayload("secret-value", timestamp, body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/webhook/qq", strings.NewReader(string(body)))
	request.Header.Set("X-Signature-Timestamp", timestamp)
	request.Header.Set("X-Signature-Ed25519", signature)

	workspace := t.TempDir()
	handler := NewWebhookHandler(config.Config{Workspace: workspace, QQAppSecret: "secret-value", QQWebhookPath: "/webhook/qq"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != `{"op":12}` {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestAuthCallbackRendersHTML(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	handler := NewWebhookHandler(config.Config{Workspace: workspace, QQAuthCallbackPath: "/qq-callback"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/qq-callback?code=abc", nil)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "MiniClaw QQ Callback") || !strings.Contains(body, "/qq-callback?code=abc") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBuildFailureMessageForIterationLimit(t *testing.T) {
	message := BuildFailureMessage("tool iteration limit reached (after 8 rounds; last tools: exec)")
	if !strings.Contains(message, "过多工具调用") {
		t.Fatalf("unexpected message: %s", message)
	}
}

func TestParseFlexibleInt(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "number", raw: `120`, want: 120},
		{name: "string", raw: `"7200"`, want: 7200},
		{name: "empty string", raw: `""`, want: 0},
		{name: "invalid", raw: `"oops"`, wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseFlexibleInt([]byte(tc.raw))
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
