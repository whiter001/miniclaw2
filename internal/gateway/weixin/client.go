package weixin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"miniclaw2/internal/config"
	qqgateway "miniclaw2/internal/gateway/qq"
)

const userMessageType = 1

type Client struct {
	account    Account
	workspace  string
	uin        string
	httpClient *http.Client
}

type UpdatesResponse struct {
	Ret                int             `json:"ret"`
	ErrCode            int             `json:"errcode"`
	ErrMsg             string          `json:"errmsg"`
	Msgs               []WeixinMessage `json:"msgs"`
	GetUpdatesBuf      string          `json:"get_updates_buf"`
	LongPollingTimeout int             `json:"longpolling_timeout_ms"`
}

type WeixinMessage struct {
	Seq          int64         `json:"seq"`
	MessageID    json.Number   `json:"message_id"`
	FromUserID   string        `json:"from_user_id"`
	ToUserID     string        `json:"to_user_id"`
	SessionID    string        `json:"session_id"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ContextToken string        `json:"context_token"`
	ItemList     []MessageItem `json:"item_list"`
}

type MessageItem struct {
	Type     int `json:"type"`
	TextItem struct {
		Text string `json:"text"`
	} `json:"text_item"`
}

type IncomingMessage struct {
	MessageID    string
	SenderUserID string
	ContextToken string
	Prompt       string
	SessionID    string
}

func NewClient(cfg config.Config) (*Client, error) {
	account, err := ResolveAccount(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{
		account:    account,
		workspace:  cfg.Workspace,
		uin:        randomEncodedUIN(),
		httpClient: &http.Client{Timeout: time.Duration(cfg.RequestTimeout) * time.Second},
	}, nil
}

func (c *Client) Account() Account {
	return c.account
}

func (c *Client) GetUpdates(ctx context.Context, cursor string) (UpdatesResponse, error) {
	response := UpdatesResponse{}
	if err := c.postJSON(ctx, "ilink/bot/getupdates", map[string]any{"get_updates_buf": cursor, "base_info": c.baseInfo()}, &response); err != nil {
		return UpdatesResponse{}, err
	}
	if response.Ret != 0 {
		return UpdatesResponse{}, fmt.Errorf("weixin getupdates api error %d: %s", response.ErrCode, strings.TrimSpace(response.ErrMsg))
	}
	return response, nil
}

func (c *Client) SendTextMessage(ctx context.Context, targetUserID, contextToken, content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("weixin reply content is empty")
	}
	return c.sendMessageItems(ctx, targetUserID, contextToken, []map[string]any{{
		"type":      messageItemTypeText,
		"text_item": map[string]string{"text": trimmed},
	}})
}

func ExtractTextMessages(messages []WeixinMessage) []IncomingMessage {
	result := make([]IncomingMessage, 0, len(messages))
	for _, message := range messages {
		if message.MessageType != userMessageType {
			continue
		}
		prompt := strings.TrimSpace(extractTextPrompt(message.ItemList))
		if prompt == "" {
			continue
		}
		messageID := strings.TrimSpace(message.MessageID.String())
		if messageID == "" || messageID == "0" {
			messageID = fmt.Sprintf("seq-%d", message.Seq)
		}
		result = append(result, IncomingMessage{
			MessageID:    messageID,
			SenderUserID: strings.TrimSpace(message.FromUserID),
			ContextToken: strings.TrimSpace(message.ContextToken),
			Prompt:       prompt,
			SessionID:    strings.TrimSpace(message.SessionID),
		})
	}
	return result
}

func IsUserAllowed(cfg config.Config, userID string) bool {
	if strings.TrimSpace(cfg.WeixinAllowUsers) == "" {
		return true
	}
	return qqgateway.CSVContainsValue(cfg.WeixinAllowUsers, userID)
}

func extractTextPrompt(items []MessageItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type != messageItemTypeText {
			continue
		}
		text := strings.TrimSpace(item.TextItem.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload any, target any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.account.BaseURL, "/")+"/"+strings.TrimLeft(endpoint, "/"), bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("AuthorizationType", "ilink_bot_token")
	if strings.TrimSpace(c.account.Token) != "" {
		request.Header.Set("Authorization", "Bearer "+c.account.Token)
	}
	request.Header.Set("X-WECHAT-UIN", c.uin)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("weixin %s api error %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	if target == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return err
	}
	return nil
}

func (c *Client) baseInfo() map[string]string {
	return map[string]string{"channel_version": "miniclaw2-go"}
}

func (c *Client) mediaTempDir() string {
	return filepath.Join(c.workspace, "state", "weixin", "media")
}

func randomEncodedUIN() string {
	value := rand.New(rand.NewSource(time.Now().UnixNano())).Uint32()
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(value), 10)))
}
