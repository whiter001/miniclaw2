package runtime

import (
	"context"
	"strings"
	"testing"

	"miniclaw2/internal/config"
	"miniclaw2/internal/skills"
)

func TestProcessTextMessageHandlesExplicitSkillCommands(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	replies := []string{}
	sendReply := func(ctx context.Context, content string, sequence int) error {
		replies = append(replies, content)
		return nil
	}

	commands := []struct {
		messageID string
		prompt    string
		contains  string
	}{
		{messageID: "1", prompt: "/skill add testing\nUse read_file before editing and validate with go test.", contains: "skill added"},
		{messageID: "2", prompt: "/skill list", contains: "testing"},
		{messageID: "3", prompt: "/skill show testing", contains: "name: testing"},
		{messageID: "4", prompt: "/skill optimize testing", contains: "skill optimized"},
		{messageID: "5", prompt: "/skill delete testing", contains: "skill deleted: testing"},
		{messageID: "6", prompt: "/skill list", contains: "no skills found"},
	}

	for _, item := range commands {
		replies = replies[:0]
		err := ProcessTextMessage(context.Background(), ProcessOptions{
			Config: cfg,
			Message: TextMessage{Channel: "qq", Scene: "dm", MessageID: item.messageID, Prompt: item.prompt},
			ProcessingText: "处理中",
			SendReply:      sendReply,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(replies) != 1 {
			t.Fatalf("expected one direct reply for %q, got %d", item.prompt, len(replies))
		}
		if !strings.Contains(replies[0], item.contains) {
			t.Fatalf("expected %q in reply %q", item.contains, replies[0])
		}
	}

	if _, err := skills.FindSkill(cfg, "testing"); err == nil {
		t.Fatal("expected testing skill to be deleted")
	}
}

func TestProcessTextMessageSkillCommandHelpOnError(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	replies := []string{}
	err := ProcessTextMessage(context.Background(), ProcessOptions{
		Config: cfg,
		Message: TextMessage{Channel: "weixin", Scene: "dm", MessageID: "1", Prompt: "/skill delete missing-skill"},
		SendReply: func(ctx context.Context, content string, sequence int) error {
			replies = append(replies, content)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(replies) != 1 {
		t.Fatalf("expected one reply, got %d", len(replies))
	}
	if !strings.Contains(replies[0], "skill command failed") || !strings.Contains(replies[0], "/skill list") {
		t.Fatalf("unexpected reply: %s", replies[0])
	}
}