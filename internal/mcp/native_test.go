package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miniclaw2/internal/config"
)

func TestNewManagerRegistersBuiltinToolsWhenEnabled(t *testing.T) {
	manager := NewManager(config.Config{EnableMCP: true, RequestTimeout: 5})
	if !manager.HasTool("web_search") || !manager.HasTool("understand_image") {
		t.Fatalf("unexpected builtin tools: %+v", manager.Tools())
	}
}

func TestWebSearchUsesCodingPlanSearchAPI(t *testing.T) {
	var requestBody map[string]any
	var requestPath string
	var requestHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestHeaders = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"organic":[{"title":"MiniMax MCP","link":"https://example.com","snippet":"Structured search result","date":"2026-04-24"}],"related_searches":[{"query":"minimax coding plan"}],"base_resp":{"status_code":0,"status_msg":"ok"}}`))
	}))
	defer server.Close()

	manager := NewManager(config.Config{
		EnableMCP:      true,
		APIKey:         "test-key",
		BaseURL:        server.URL + "/anthropic",
		RequestTimeout: 5,
	})
	manager.client = server.Client()

	result, err := manager.CallTool(context.Background(), "web_search", map[string]any{"query": "minimax mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if requestPath != "/v1/coding_plan/search" {
		t.Fatalf("unexpected request path: %s", requestPath)
	}
	if got := requestHeaders.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("unexpected Authorization header: %s", got)
	}
	if got := requestHeaders.Get("MM-API-Source"); got != "Minimax-MCP" {
		t.Fatalf("unexpected MM-API-Source header: %s", got)
	}
	if got := requestBody["q"]; got != "minimax mcp" {
		t.Fatalf("unexpected request body: %+v", requestBody)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	organic, ok := parsed["organic"].([]any)
	if !ok || len(organic) != 1 {
		t.Fatalf("unexpected web_search result: %+v", parsed)
	}
	first, ok := organic[0].(map[string]any)
	if !ok || first["title"] != "MiniMax MCP" {
		t.Fatalf("unexpected organic result: %+v", organic[0])
	}
	if !strings.Contains(result, "related_searches") {
		t.Fatalf("expected related_searches in result: %s", result)
	}
}

func TestUnderstandImageUsesLocalPathAndCodingPlanVLM(t *testing.T) {
	workspace := t.TempDir()
	imagePath := filepath.Join(workspace, "image.png")
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+iX2kAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	var requestBody map[string]any
	var requestPath string
	var requestHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestHeaders = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"Image looks fine","base_resp":{"status_code":0,"status_msg":"ok"}}`))
	}))
	defer server.Close()

	manager := NewManager(config.Config{
		EnableMCP:      true,
		Workspace:      workspace,
		APIKey:         "test-key",
		BaseURL:        server.URL + "/anthropic",
		Model:          "MiniMax-M2.7",
		MaxTokens:      256,
		Temperature:    0.1,
		RequestTimeout: 5,
	})
	manager.client = server.Client()
	result, err := manager.CallTool(context.Background(), "understand_image", map[string]any{"image_source": "@image.png", "prompt": "describe"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Image looks fine") {
		t.Fatalf("unexpected result: %s", result)
	}
	if requestPath != "/v1/coding_plan/vlm" {
		t.Fatalf("unexpected request path: %s", requestPath)
	}
	if got := requestHeaders.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("unexpected Authorization header: %s", got)
	}
	if got := requestHeaders.Get("MM-API-Source"); got != "Minimax-MCP" {
		t.Fatalf("unexpected MM-API-Source header: %s", got)
	}
	if got := requestBody["prompt"]; got != "describe" {
		t.Fatalf("unexpected request prompt: %+v", requestBody)
	}
	imageURL, ok := requestBody["image_url"].(string)
	if !ok || !strings.HasPrefix(imageURL, "data:image/png;base64,") {
		t.Fatalf("unexpected request body: %+v", requestBody)
	}
	if strings.Contains(imageURL, "@image.png") {
		t.Fatalf("unexpected request body: %+v", requestBody)
	}
}

func TestProcessImageSourceSupportsDataURLsAndRemoteURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4E, 0x47})
	}))
	defer server.Close()

	manager := NewManager(config.Config{EnableMCP: true, RequestTimeout: 5})
	manager.client = server.Client()

	dataURL := "data:image/png;base64,abc123"
	got, err := manager.processImageSource(context.Background(), dataURL)
	if err != nil {
		t.Fatal(err)
	}
	if got != dataURL {
		t.Fatalf("expected data URL passthrough, got %s", got)
	}

	remote, err := manager.processImageSource(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(remote, "data:image/png;base64,") {
		t.Fatalf("unexpected remote conversion result: %s", remote)
	}
}

func TestProcessImageSourceRejectsAbsolutePathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	insidePath := filepath.Join(workspace, "inside.png")
	outsidePath := filepath.Join(root, "outside.png")
	for _, path := range []string{insidePath, outsidePath} {
		if err := os.WriteFile(path, []byte{0x89, 0x50, 0x4E, 0x47}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manager := NewManager(config.Config{EnableMCP: true, Workspace: workspace, RequestTimeout: 5})

	inside, err := manager.processImageSource(context.Background(), insidePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(inside, "data:image/png;base64,") {
		t.Fatalf("unexpected inside image result: %s", inside)
	}
	_, err = manager.processImageSource(context.Background(), outsidePath)
	if err == nil || !strings.Contains(err.Error(), "inside workspace") {
		t.Fatalf("expected outside workspace error, got %v", err)
	}
}
