package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Recorder struct {
	SessionID string
	FilePath  string

	mu sync.Mutex
}

type messageLine struct {
	TS      string `json:"ts"`
	Kind    string `json:"kind"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	ToolName string `json:"tool_name,omitempty"`
	ToolID   string `json:"tool_id,omitempty"`
	IsError  bool   `json:"is_error,omitempty"`
}

func New(workspace string) (*Recorder, error) {
	sessionID := strings.ReplaceAll(time.Now().Format("20060102_150405.000"), ":", "-")
	filePath := filepath.Join(workspace, "sessions", fmt.Sprintf("session-%s.jsonl", sessionID))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return &Recorder{SessionID: sessionID, FilePath: filePath}, nil
}

func (r *Recorder) AppendMessage(kind, role, content string) error {
	return r.append(messageLine{
		TS:      time.Now().Format(time.RFC3339Nano),
		Kind:    kind,
		Role:    role,
		Content: content,
	})
}

func (r *Recorder) AppendTool(toolName, toolID, result string, isError bool) error {
	return r.append(messageLine{
		TS:       time.Now().Format(time.RFC3339Nano),
		Kind:     "tool",
		ToolName: toolName,
		ToolID:   toolID,
		IsError:  isError,
		Content:  result,
	})
}

func (r *Recorder) append(value messageLine) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(r.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}
