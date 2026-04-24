package weixin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"miniclaw2/internal/config"
)

func TestExtractTextMessages(t *testing.T) {
	messages := []WeixinMessage{
		{
			Seq:          11,
			MessageID:    json.Number("101"),
			FromUserID:   "user-1",
			ContextToken: "ctx-1",
			SessionID:    "session-1",
			MessageType:  userMessageType,
			ItemList: []MessageItem{{
				Type: messageItemTypeText,
				TextItem: struct {
					Text string `json:"text"`
				}{Text: " hello "},
			}},
		},
		{
			Seq:         12,
			MessageType: 2,
			ItemList: []MessageItem{{
				Type: messageItemTypeText,
				TextItem: struct {
					Text string `json:"text"`
				}{Text: "ignore me"},
			}},
		},
	}
	result := ExtractTextMessages(messages)
	if len(result) != 1 {
		t.Fatalf("unexpected messages: %+v", result)
	}
	if result[0].MessageID != "101" || result[0].SenderUserID != "user-1" || result[0].ContextToken != "ctx-1" || result[0].Prompt != "hello" {
		t.Fatalf("unexpected extracted message: %+v", result[0])
	}
}

func TestClientGetUpdatesAndSendTextMessage(t *testing.T) {
	var sendBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("AuthorizationType") != "ilink_bot_token" {
			t.Fatalf("unexpected authorization type: %s", r.Header.Get("AuthorizationType"))
		}
		if r.Header.Get("Authorization") != "Bearer token-1" {
			t.Fatalf("unexpected authorization: %s", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/ilink/bot/getupdates":
			_, _ = w.Write([]byte(`{"ret":0,"msgs":[{"message_id":101,"from_user_id":"user-1","context_token":"ctx-1","message_type":1,"item_list":[{"type":1,"text_item":{"text":"hello"}}]}],"get_updates_buf":"cursor-2","longpolling_timeout_ms":35000}`))
		case "/ilink/bot/sendmessage":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			sendBody = string(body)
			_, _ = w.Write([]byte(`{"ret":0}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(config.Config{WeixinAPIBase: server.URL, WeixinToken: "token-1", RequestTimeout: 3})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.GetUpdates(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if response.GetUpdatesBuf != "cursor-2" || len(ExtractTextMessages(response.Msgs)) != 1 {
		t.Fatalf("unexpected updates response: %+v", response)
	}
	if err := client.SendTextMessage(context.Background(), "user-1", "ctx-1", "reply"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sendBody, `"to_user_id":"user-1"`) || !strings.Contains(sendBody, `"context_token":"ctx-1"`) || !strings.Contains(sendBody, `"text":"reply"`) {
		t.Fatalf("unexpected send body: %s", sendBody)
	}
}

func TestParseRenderedReply(t *testing.T) {
	reply := ParseRenderedReply("标题说明\nMEDIA:https://example.com/a.png\nMEDIA:/tmp/demo.pdf\n结尾")
	if reply.Text != "标题说明\n结尾" {
		t.Fatalf("unexpected text: %q", reply.Text)
	}
	if len(reply.MediaSources) != 2 || reply.MediaSources[0] != "https://example.com/a.png" || reply.MediaSources[1] != "/tmp/demo.pdf" {
		t.Fatalf("unexpected media sources: %+v", reply.MediaSources)
	}
}

func TestSaveAccountAndResolveAccount(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	path, err := SaveAccount(cfg, "Bot 1", AccountData{
		RawAccountID: "Bot 1",
		Token:        "token-1",
		BaseURL:      "https://api.example.test",
		UserID:       "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "bot-1.json") {
		t.Fatalf("unexpected account path: %s", path)
	}
	ids, err := ListAccountIDs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "bot-1" {
		t.Fatalf("unexpected account ids: %+v", ids)
	}
	if active := LoadActiveAccountID(cfg); active != "bot-1" {
		t.Fatalf("unexpected active account: %s", active)
	}
	account, err := ResolveAccount(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if account.ID != "bot-1" || account.RawAccountID != "Bot 1" || account.Token != "token-1" || account.UserID != "user-1" {
		t.Fatalf("unexpected resolved account: %+v", account)
	}
}
