package qq

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"crypto/ed25519"

	"miniclaw2/internal/config"
	"miniclaw2/internal/provider/minimax"
)

const accessTokenURL = "https://bots.qq.com/app/getAppAccessToken"

var stateFileMu sync.Mutex

type AccessToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	FetchedAt   int64  `json:"fetched_at"`
}

type accessTokenAPIResponse struct {
	AccessToken string          `json:"access_token"`
	ExpiresIn   json.RawMessage `json:"expires_in"`
}

type BotProfile struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	ShareURL string `json:"share_url"`
}

type ReplyTarget struct {
	Scene       string
	OpenID      string
	GroupOpenID string
	MsgID       string
}

func FetchAccessToken(ctx context.Context, cfg config.Config) (AccessToken, error) {
	if strings.TrimSpace(cfg.QQAppID) == "" || strings.TrimSpace(cfg.QQAppSecret) == "" {
		return AccessToken{}, fmt.Errorf("qq_app_id or qq_app_secret is not configured")
	}
	payload := map[string]string{"appId": cfg.QQAppID, "clientSecret": cfg.QQAppSecret}
	body, err := json.Marshal(payload)
	if err != nil {
		return AccessToken{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, accessTokenURL, bytes.NewReader(body))
	if err != nil {
		return AccessToken{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Duration(cfg.RequestTimeout) * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return AccessToken{}, fmt.Errorf("qq access token request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return AccessToken{}, err
	}
	if response.StatusCode != http.StatusOK {
		return AccessToken{}, fmt.Errorf("qq access token api error %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var apiResponse accessTokenAPIResponse
	if err := json.Unmarshal(responseBody, &apiResponse); err != nil {
		return AccessToken{}, err
	}
	expiresIn, err := parseFlexibleInt(apiResponse.ExpiresIn)
	if err != nil {
		return AccessToken{}, fmt.Errorf("invalid qq expires_in value: %w", err)
	}
	token := AccessToken{
		AccessToken: apiResponse.AccessToken,
		ExpiresIn:   expiresIn,
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return AccessToken{}, fmt.Errorf("qq access token missing in response")
	}
	token.FetchedAt = time.Now().Unix()
	return token, nil
}

func FetchBotProfile(ctx context.Context, cfg config.Config, accessToken string) (BotProfile, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.QQAPIBase, "/")+"/users/@me", nil)
	if err != nil {
		return BotProfile{}, err
	}
	request.Header.Set("Authorization", "QQBot "+accessToken)
	client := &http.Client{Timeout: time.Duration(cfg.RequestTimeout) * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return BotProfile{}, fmt.Errorf("qq profile request failed: %w", err)
	}
	defer response.Body.Close()
	var profile BotProfile
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		return BotProfile{}, err
	}
	if response.StatusCode != http.StatusOK {
		return BotProfile{}, fmt.Errorf("qq profile api error %d", response.StatusCode)
	}
	return profile, nil
}

func WriteGatewayState(cfg config.Config, token AccessToken, profile BotProfile) (string, error) {
	statePath := filepath.Join(cfg.Workspace, "state", "qq_gateway_state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return "", err
	}
	payload := map[string]any{
		"fetched_at":   time.Unix(token.FetchedAt, 0).Format(time.RFC3339),
		"access_token": token.AccessToken,
		"expires_in":   token.ExpiresIn,
		"profile":      profile,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	if err := os.WriteFile(statePath, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return statePath, nil
}

func SendReply(ctx context.Context, cfg config.Config, accessToken string, target ReplyTarget, content string, msgSeq int) (string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", fmt.Errorf("qq reply content is empty")
	}
	sequence := msgSeq
	if sequence <= 0 {
		sequence = 1
	}
	var endpoint string
	payload := map[string]any{"content": trimmed, "msg_type": 0, "msg_id": target.MsgID, "msg_seq": sequence}
	switch target.Scene {
	case "c2c":
		endpoint = fmt.Sprintf("%s/v2/users/%s/messages", strings.TrimRight(cfg.QQAPIBase, "/"), target.OpenID)
	case "group":
		endpoint = fmt.Sprintf("%s/v2/groups/%s/messages", strings.TrimRight(cfg.QQAPIBase, "/"), target.GroupOpenID)
	default:
		return "", fmt.Errorf("unsupported qq reply scene: %s", target.Scene)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "QQBot "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: time.Duration(cfg.RequestTimeout) * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("qq send message request failed: %w", err)
	}
	defer response.Body.Close()
	responseBytes, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("qq send message api error %d: %s", response.StatusCode, strings.TrimSpace(string(responseBytes)))
	}
	return strings.TrimSpace(string(responseBytes)), nil
}

func IsTargetAllowed(cfg config.Config, target ReplyTarget) bool {
	switch target.Scene {
	case "c2c":
		if strings.TrimSpace(cfg.QQAllowUsers) == "" {
			return true
		}
		return CSVContainsValue(cfg.QQAllowUsers, target.OpenID)
	case "group":
		if strings.TrimSpace(cfg.QQAllowGroups) == "" {
			return true
		}
		return CSVContainsValue(cfg.QQAllowGroups, target.GroupOpenID)
	default:
		return false
	}
}

func CSVContainsValue(csv, value string) bool {
	needle := strings.TrimSpace(value)
	if needle == "" {
		return false
	}
	for _, item := range strings.Split(csv, ",") {
		if strings.TrimSpace(item) == needle {
			return true
		}
	}
	return false
}

func BuildFailureMessage(errorMessage string) string {
	if strings.Contains(errorMessage, minimax.ToolIterationErrorPrefix) {
		return "这个问题触发了过多工具调用，我没能在限定步数内完成。请把问题拆小一点，或直接说明要查看的文件、目录或命令。"
	}
	return "处理失败，请稍后重试。"
}

func SignValidation(secret, eventTS, plainToken string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("qq_app_secret is empty")
	}
	seed := secret
	for len(seed) < ed25519.SeedSize {
		seed += secret
	}
	seed = seed[:ed25519.SeedSize]
	privateKey := ed25519.NewKeyFromSeed([]byte(seed))
	signature := ed25519.Sign(privateKey, []byte(eventTS+plainToken))
	return hex.EncodeToString(signature), nil
}

func parseFlexibleInt(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return asInt, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		trimmed := strings.TrimSpace(asString)
		if trimmed == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	}
	return 0, fmt.Errorf("unsupported integer payload: %s", string(raw))
}
