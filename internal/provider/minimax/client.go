package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"miniclaw2/internal/config"
)

type Client struct {
	httpClient *http.Client
}

type requestBody struct {
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature float64          `json:"temperature"`
	TopP        float64          `json:"top_p"`
	System      string           `json:"system,omitempty"`
	Tools       []toolSchema     `json:"tools,omitempty"`
	Messages    []requestMessage `json:"messages"`
}

type requestMessage struct {
	Role    string                `json:"role"`
	Content []requestContentBlock `json:"content"`
}

type requestContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

func (b requestContentBlock) MarshalJSON() ([]byte, error) {
	payload := map[string]any{
		"type": b.Type,
	}
	if b.Text != "" {
		payload["text"] = b.Text
	}
	if b.ID != "" {
		payload["id"] = b.ID
	}
	if b.Name != "" {
		payload["name"] = b.Name
	}
	if b.Type == "tool_use" {
		if b.Input == nil {
			payload["input"] = map[string]any{}
		} else {
			payload["input"] = b.Input
		}
	} else if b.Input != nil && len(b.Input) > 0 {
		payload["input"] = b.Input
	}
	if b.ToolUseID != "" {
		payload["tool_use_id"] = b.ToolUseID
	}
	if b.Content != "" {
		payload["content"] = b.Content
	}
	if b.IsError {
		payload["is_error"] = true
	}
	return json.Marshal(payload)
}

type toolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type responseBody struct {
	Content []responseContentBlock `json:"content"`
}

type responseContentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{httpClient: &http.Client{Timeout: time.Duration(cfg.RequestTimeout) * time.Second}}
}

func (c *Client) CallText(ctx context.Context, cfg config.Config, prompt string) (string, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", fmt.Errorf("MINICLAW_API_KEY is not configured")
	}
	body := requestBody{
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
		TopP:        1.0,
		System:      LoadSystemPrompt(cfg),
		Messages: []requestMessage{{
			Role: "user",
			Content: []requestContentBlock{{
				Type: "text",
				Text: prompt,
			}},
		}},
	}
	response, err := c.send(ctx, cfg, body)
	if err != nil {
		return "", err
	}
	var parsed responseBody
	if err := json.Unmarshal(response, &parsed); err != nil {
		return "", err
	}
	text := ExtractTextBlocks(parsed.Content)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("MiniMax returned an empty text response")
	}
	return strings.TrimSpace(text), nil
}

func (c *Client) Send(ctx context.Context, cfg config.Config, body requestBody) (responseBody, error) {
	var response responseBody
	payload, err := c.send(ctx, cfg, body)
	if err != nil {
		return response, err
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return response, err
	}
	return response, nil
}

func (c *Client) send(ctx context.Context, cfg config.Config, body requestBody) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, ResolveAnthropicMessagesURL(cfg.BaseURL), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MiniMax API error %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return responseBody, nil
}

func ResolveAnthropicMessagesURL(apiURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if strings.HasSuffix(trimmed, "/messages") {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/anthropic") || strings.HasSuffix(trimmed, "/anthropic/v1") {
		return trimmed + "/messages"
	}
	return trimmed
}

func ExtractTextBlocks(blocks []responseContentBlock) string {
	parts := []string{}
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "")
}
