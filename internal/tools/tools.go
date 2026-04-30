package tools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"miniclaw2/internal/config"
)

const maxExecOutputChars = 12000
const maxExecOutputBytes = 64 * 1024

var blockedExecPatterns = []string{
	"rm -rf",
	"rmdir /s",
	"del /f",
	"mkfs",
	"format ",
	"dd if=",
	"shutdown",
	"reboot",
	"poweroff",
	":(){ :|:& };:",
}

type ToolUse struct {
	ID    string
	Name  string
	Input map[string]string
}

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema map[string]any
}

func Definitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "list_dir",
			Description: "List files and directories inside the workspace. Use this before reading files when you need to explore.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "description": "Relative path inside the workspace. Use . for the workspace root."}}},
		},
		{
			Name:        "read_file",
			Description: "Read a UTF-8 text file from the workspace.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "description": "Relative file path inside the workspace."}, "start_line": map[string]any{"type": "string", "description": "Optional 1-based start line."}, "end_line": map[string]any{"type": "string", "description": "Optional 1-based end line."}}, "required": []string{"path"}},
		},
		{
			Name:        "write_file",
			Description: "Write a UTF-8 text file inside the workspace. This replaces the full file content.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "description": "Relative file path inside the workspace."}, "content": map[string]any{"type": "string", "description": "Full file content to write."}}, "required": []string{"path", "content"}},
		},
		{
			Name:        "exec",
			Description: "Run a shell command with the workspace as the current working directory. Dangerous destructive commands are blocked.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string", "description": "Shell command to run."}}, "required": []string{"command"}},
		},
		{
			Name:        "grep_search",
			Description: "Search for text or regex inside workspace files.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string", "description": "Search pattern."}, "path": map[string]any{"type": "string", "description": "Optional relative directory or file path inside the workspace."}, "is_regexp": map[string]any{"type": "string", "description": "Set to true to treat query as regular expression."}}, "required": []string{"query"}},
		},
	}
}

func Execute(tool ToolUse, cfg config.Config) (string, error) {
	return ExecuteWithContext(context.Background(), tool, cfg)
}

func ExecuteWithContext(ctx context.Context, tool ToolUse, cfg config.Config) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch tool.Name {
	case "list_dir":
		return toolListDir(tool, cfg)
	case "read_file":
		return toolReadFile(tool, cfg)
	case "write_file":
		return toolWriteFile(tool, cfg)
	case "exec":
		return toolExec(ctx, tool, cfg)
	case "grep_search":
		return toolGrepSearch(tool, cfg)
	default:
		return "", fmt.Errorf("unsupported tool: %s", tool.Name)
	}
}

func toolListDir(tool ToolUse, cfg config.Config) (string, error) {
	relPath := firstNonEmpty(tool.Input["path"], tool.Input["dir"], ".")
	resolved, err := ResolveWorkspacePath(cfg.Workspace, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", relPath)
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %w", err)
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

func toolReadFile(tool ToolUse, cfg config.Config) (string, error) {
	relPath := firstNonEmpty(tool.Input["path"], tool.Input["filePath"])
	if relPath == "" {
		return "", errors.New("path is required")
	}
	resolved, err := ResolveWorkspacePath(cfg.Workspace, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("not a file: %s", relPath)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	content := string(data)
	startLine := parseOptionalPositiveInt(firstNonEmpty(tool.Input["start_line"], tool.Input["startLine"]))
	endLine := parseOptionalPositiveInt(firstNonEmpty(tool.Input["end_line"], tool.Input["endLine"]))
	if startLine == 0 && endLine == 0 {
		return content, nil
	}
	lines := strings.Split(content, "\n")
	start := 1
	if startLine > 0 {
		start = startLine
	}
	end := len(lines)
	if endLine > 0 {
		end = endLine
	}
	if start > end || start > len(lines) {
		return "", nil
	}
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start-1:end], "\n"), nil
}

func toolWriteFile(tool ToolUse, cfg config.Config) (string, error) {
	relPath := firstNonEmpty(tool.Input["path"], tool.Input["filePath"])
	content := tool.Input["content"]
	if relPath == "" {
		return "", errors.New("path is required")
	}
	resolved, err := ResolveWorkspacePathForWrite(cfg.Workspace, relPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return fmt.Sprintf("wrote %s (%d chars)", relPath, len(content)), nil
}

func toolExec(ctx context.Context, tool ToolUse, cfg config.Config) (string, error) {
	command := firstNonEmpty(tool.Input["command"], tool.Input["cmd"])
	if command == "" {
		return "", errors.New("command is required")
	}
	if IsBlockedCommand(command) {
		return "", errors.New("command blocked by safety guard")
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-lc", command)
	}
	cmd.Dir = cfg.Workspace
	configureExecCommand(cmd)
	output := newCappedOutputBuffer(maxExecOutputBytes)
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	trimmed := trimExecOutput(output.String(), output.Truncated())
	if trimmed == "" {
		trimmed = "(no output)"
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("exec command canceled: %w", ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("exit code %d: %s", exitErr.ExitCode(), trimmed)
		}
		return "", err
	}
	return trimmed, nil
}

type cappedOutputBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func newCappedOutputBuffer(limit int) *cappedOutputBuffer {
	if limit <= 0 {
		limit = maxExecOutputBytes
	}
	return &cappedOutputBuffer{limit: limit}
}

func (b *cappedOutputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.buffer.Write(p)
	return len(p), nil
}

func (b *cappedOutputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *cappedOutputBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

func trimExecOutput(value string, truncated bool) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed, charTruncated := truncateRunes(trimmed, maxExecOutputChars)
	if truncated || charTruncated {
		trimmed += "\n... (truncated)"
	}
	return trimmed
}

func truncateRunes(value string, limit int) (string, bool) {
	if limit <= 0 {
		return "", value != ""
	}
	count := 0
	for index := range value {
		if count == limit {
			return value[:index], true
		}
		count++
	}
	return value, false
}

func toolGrepSearch(tool ToolUse, cfg config.Config) (string, error) {
	query := firstNonEmpty(tool.Input["query"], tool.Input["pattern"])
	if query == "" {
		return "", errors.New("query is required")
	}
	relPath := firstNonEmpty(tool.Input["path"], ".")
	root, err := ResolveWorkspacePath(cfg.Workspace, relPath)
	if err != nil {
		return "", err
	}
	useRegexp := strings.EqualFold(firstNonEmpty(tool.Input["is_regexp"], tool.Input["isRegexp"], "false"), "true")
	matches, err := grepSearch(root, cfg.Workspace, query, useRegexp)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "(no matches)", nil
	}
	if len(matches) > 50 {
		return strings.Join(matches[:50], "\n") + "\n... (truncated)", nil
	}
	return strings.Join(matches, "\n"), nil
}

func grepSearch(root, workspace, query string, useRegexp bool) ([]string, error) {
	matcher := func(string) bool { return false }
	var re *regexp.Regexp
	if useRegexp {
		compiled, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		re = compiled
		matcher = re.MatchString
	} else {
		matcher = func(line string) bool { return strings.Contains(strings.ToLower(line), strings.ToLower(query)) }
	}

	results := []string{}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	searchFile := func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if looksBinary(data) {
			return nil
		}
		rel, err := RelativeToWorkspace(workspace, path)
		if err != nil {
			rel = path
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if matcher(line) {
				results = append(results, fmt.Sprintf("%s:%d:%s", rel, lineNo, line))
			}
		}
		return nil
	}
	if !info.IsDir() {
		return results, searchFile(root)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		return searchFile(path)
	})
	return results, err
}

func looksBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if bytes.Contains(data, []byte{0}) {
		return true
	}
	prefix := data
	if len(prefix) > 2048 {
		prefix = prefix[:2048]
	}
	return !utf8.Valid(prefix)
}

func ResolveWorkspacePath(workspace, relPath string) (string, error) {
	return resolveWorkspacePath(workspace, relPath, false)
}

func ResolveWorkspacePathForWrite(workspace, relPath string) (string, error) {
	return resolveWorkspacePath(workspace, relPath, true)
}

func resolveWorkspacePath(workspace, relPath string, allowMissing bool) (string, error) {
	base, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	if realBase, err := filepath.EvalSymlinks(base); err == nil {
		base = realBase
	}
	base = filepath.Clean(base)
	trimmed := strings.TrimSpace(relPath)
	var target string
	if trimmed == "" || trimmed == "." {
		target = base
	} else {
		target = filepath.Clean(filepath.Join(base, trimmed))
	}
	if !hasPathPrefix(target, base) {
		return "", fmt.Errorf("path escapes workspace: %s", relPath)
	}
	if allowMissing {
		parent := filepath.Dir(target)
		if realParent, err := filepath.EvalSymlinks(parent); err == nil && !hasPathPrefix(realParent, base) {
			return "", fmt.Errorf("path escapes workspace: %s", relPath)
		}
		return target, nil
	}
	if realTarget, err := filepath.EvalSymlinks(target); err == nil {
		if !hasPathPrefix(realTarget, base) {
			return "", fmt.Errorf("path escapes workspace: %s", relPath)
		}
		return realTarget, nil
	}
	return target, nil
}

func RelativeToWorkspace(workspace, path string) (string, error) {
	base, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func hasPathPrefix(path, base string) bool {
	cleanPath := filepath.Clean(path)
	cleanBase := filepath.Clean(base)
	if runtime.GOOS == "windows" {
		cleanPath = strings.ToLower(cleanPath)
		cleanBase = strings.ToLower(cleanBase)
	}
	if cleanPath == cleanBase {
		return true
	}
	return strings.HasPrefix(cleanPath, cleanBase+string(os.PathSeparator))
}

func IsBlockedCommand(command string) bool {
	normalized := strings.ToLower(command)
	for _, pattern := range blockedExecPatterns {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return false
}

func parseOptionalPositiveInt(value string) int {
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
