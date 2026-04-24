package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"miniclaw2/internal/config"
)

type commandTool struct {
	tool    Tool
	command string
	args    []string
	env     map[string]string
	timeout time.Duration
}

var templateVarPattern = regexp.MustCompile(`\{\{([A-Za-z0-9_]+)\}\}`)

func loadCommandTools(cfg config.Config) []*commandTool {
	raw := loadRawMCPConfig(cfg)
	return loadCommandToolsFromRaw(raw, cfg)
}

func loadCommandToolsFromRaw(raw rawMCPConfig, cfg config.Config) []*commandTool {
	toolsList := make([]*commandTool, 0, len(raw.Tools))
	for name, rawTool := range raw.Tools {
		if rawTool.Type != "command" || strings.TrimSpace(rawTool.Command) == "" {
			continue
		}
		inputSchema := rawTool.normalizedInputSchema()
		if len(inputSchema) == 0 {
			inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		timeout := time.Duration(rawTool.normalizedTimeoutSeconds()) * time.Second
		if timeout <= 0 {
			timeout = time.Duration(cfg.RequestTimeout) * time.Second
		}
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		toolsList = append(toolsList, &commandTool{
			tool: Tool{
				Name:        name,
				Description: firstNonEmpty(rawTool.Description, "External command tool: "+name),
				InputSchema: inputSchema,
			},
			command: rawTool.Command,
			args:    append([]string(nil), rawTool.Args...),
			env:     cloneStringMap(rawTool.Env),
			timeout: timeout,
		})
	}
	sort.Slice(toolsList, func(i, j int) bool {
		return toolsList[i].tool.Name < toolsList[j].tool.Name
	})
	return toolsList
}

func (c *commandTool) call(ctx context.Context, cfg config.Config, input map[string]any) (string, error) {
	variables := buildCommandTemplateVariables(cfg, input)
	command, err := renderCommandTemplate(c.command, variables)
	if err != nil {
		return "", err
	}
	args, err := renderCommandArgs(c.args, variables)
	if err != nil {
		return "", err
	}
	args = injectDefaultCommandArgs(command, args, cfg)
	env, err := renderCommandEnv(c.env, variables)
	if err != nil {
		return "", err
	}
	injectDefaultCommandEnv(command, args, env, cfg)

	runCtx := ctx
	cancel := func() {}
	if c.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, c.timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, command, args...)
	cmd.Dir = cfg.Workspace
	cmd.Env = mergeCommandEnv(os.Environ(), env)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("external tool %q timed out", c.tool.Name)
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("external tool %q failed: %s", c.tool.Name, message)
	}
	result := strings.TrimSpace(stdout.String())
	if result != "" {
		return result, nil
	}
	result = strings.TrimSpace(stderr.String())
	if result != "" {
		return result, nil
	}
	return "(no output)", nil
}

func buildCommandTemplateVariables(cfg config.Config, input map[string]any) map[string]string {
	variables := map[string]string{
		"MINICLAW_API_KEY":   cfg.APIKey,
		"MINICLAW_BASE_URL":  cfg.BaseURL,
		"MINICLAW_MODEL":     cfg.Model,
		"MINICLAW_WORKSPACE": cfg.Workspace,
		"MINICLAW_REGION":    deriveMiniClawRegion(cfg.BaseURL),
	}
	for key, value := range input {
		variables[key] = strings.TrimSpace(toString(value))
	}
	return variables
}

func renderCommandArgs(args []string, variables map[string]string) ([]string, error) {
	rendered := make([]string, 0, len(args))
	for _, arg := range args {
		value, err := renderCommandTemplate(arg, variables)
		if err != nil {
			return nil, err
		}
		rendered = append(rendered, value)
	}
	return rendered, nil
}

func renderCommandEnv(env map[string]string, variables map[string]string) (map[string]string, error) {
	rendered := make(map[string]string, len(env))
	for key, value := range env {
		resolved, err := renderCommandTemplate(value, variables)
		if err != nil {
			return nil, err
		}
		rendered[key] = resolved
	}
	return rendered, nil
}

func renderCommandTemplate(template string, variables map[string]string) (string, error) {
	missing := ""
	result := templateVarPattern.ReplaceAllStringFunc(template, func(match string) string {
		groups := templateVarPattern.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		value, ok := variables[groups[1]]
		if !ok {
			missing = groups[1]
			return ""
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("missing template variable %q", missing)
	}
	return result, nil
}

func injectDefaultCommandEnv(command string, args []string, env map[string]string, cfg config.Config) {
	commandBase := strings.ToLower(filepathBase(command))
	if commandBase != "mmx" && commandBase != "mmx.cmd" {
		return
	}
	if hasCommandArgValue(args, "--api-key") {
		return
	}
	if strings.TrimSpace(env["MINIMAX_API_KEY"]) == "" && strings.TrimSpace(cfg.APIKey) != "" {
		env["MINIMAX_API_KEY"] = cfg.APIKey
	}
	if strings.TrimSpace(env["MINIMAX_REGION"]) == "" {
		env["MINIMAX_REGION"] = deriveMiniClawRegion(cfg.BaseURL)
	}
}

func injectDefaultCommandArgs(command string, args []string, cfg config.Config) []string {
	commandBase := strings.ToLower(filepathBase(command))
	if commandBase != "mmx" && commandBase != "mmx.cmd" {
		return args
	}
	injected := append([]string(nil), args...)
	if !hasCommandArg(injected, "--non-interactive") {
		injected = append(injected, "--non-interactive")
	}
	if !hasCommandArg(injected, "--quiet") {
		injected = append(injected, "--quiet")
	}
	if !hasCommandArgValue(injected, "--region") {
		injected = append(injected, "--region", deriveMiniClawRegion(cfg.BaseURL))
	}
	if strings.TrimSpace(cfg.APIKey) != "" && !hasCommandArgValue(injected, "--api-key") {
		injected = append(injected, "--api-key", cfg.APIKey)
	}
	return injected
}

func deriveMiniClawRegion(baseURL string) string {
	lower := strings.ToLower(strings.TrimSpace(baseURL))
	if strings.Contains(lower, "minimaxi.com") {
		return "cn"
	}
	return "global"
}

func mergeCommandEnv(base []string, overrides map[string]string) []string {
	merged := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		merged[parts[0]] = parts[1]
	}
	for key, value := range overrides {
		merged[key] = value
	}
	result := make([]string, 0, len(merged))
	for key, value := range merged {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

func hasCommandArg(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func hasCommandArgValue(args []string, flag string) bool {
	for index, arg := range args {
		if arg != flag {
			continue
		}
		if index+1 < len(args) && strings.TrimSpace(args[index+1]) != "" {
			return true
		}
	}
	return false
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
