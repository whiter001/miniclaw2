package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"miniclaw2/internal/config"
	qqgateway "miniclaw2/internal/gateway/qq"
	"miniclaw2/internal/memory"
	"miniclaw2/internal/provider/minimax"
	"miniclaw2/internal/session"
	"miniclaw2/internal/workspace"
)

const Version = "v0.1.0"

func Run(args []string) int {
	if len(args) == 0 {
		printHelp()
		return 0
	}
	if args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return 0
	}
	if args[0] == "--version" {
		fmt.Println(Version)
		return 0
	}

	cfg := config.Load()
	command := args[0]
	commandArgs := []string{}
	if len(args) > 1 {
		commandArgs = args[1:]
	}
	applyCommandConfigOverrides(&cfg, commandArgs)

	switch command {
	case "onboard":
		return runOnboard(cfg)
	case "status":
		return runStatus(cfg)
	case "gateway":
		return runGateway(cfg, commandArgs)
	case "agent":
		return runAgent(cfg, commandArgs)
	case "memory":
		return runMemory(cfg, commandArgs)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printHelp()
		return 1
	}
}

func applyCommandConfigOverrides(cfg *config.Config, args []string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--workspace":
			if i+1 < len(args) {
				cfg.Workspace = config.ExpandHomePath(args[i+1])
				i++
			}
		case "--mcp":
			cfg.EnableMCP = true
		case "--webhook-port":
			if i+1 < len(args) {
				if port, err := strconv.Atoi(args[i+1]); err == nil {
					cfg.QQWebhookPort = port
				}
				i++
			}
		}
	}
}

func printHelp() {
	fmt.Println("MiniClaw " + Version)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  miniclaw onboard              Initialize config and workspace")
	fmt.Println("  miniclaw status               Show current configuration status")
	fmt.Println("  miniclaw gateway [--once] [--webhook-port PORT]   Start QQ gateway bootstrap or webhook server")
	fmt.Println("  miniclaw agent [-p PROMPT] [--workspace PATH] [--mcp]    Run agent")
	fmt.Println("  miniclaw memory [show|set|append|today|summarize|compact|prune|clear]    Manage memory files")
	fmt.Println("  miniclaw --version            Show version")
	fmt.Println()
	fmt.Println("Environment variables:")
	fmt.Println("  MINICLAW_HOME")
	fmt.Println("  MINICLAW_WORKSPACE")
	fmt.Println("  MINICLAW_MCP_CONFIG_PATH")
	fmt.Println("  MINICLAW_API_KEY")
	fmt.Println("  ANTHROPIC_BASE_URL")
	fmt.Println("  MINICLAW_API_URL (legacy alias)")
	fmt.Println("  MINICLAW_MODEL")
	fmt.Println("  MINICLAW_ENABLE_MCP")
	fmt.Println("  MINICLAW_MCP_BASE_PATH")
	fmt.Println("  MINICLAW_MCP_RESOURCE_MODE")
	fmt.Println("  MINICLAW_QQ_APP_ID")
	fmt.Println("  MINICLAW_QQ_APP_SECRET")
}

func runOnboard(cfg config.Config) int {
	createdConfig := false
	if _, err := os.Stat(cfg.ConfigPath); err != nil {
		if err := config.WriteDefault(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write config: %v\n", err)
			return 1
		}
		createdConfig = true
	}
	if err := workspace.Ensure(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize workspace: %v\n", err)
		return 1
	}
	fmt.Println("MiniClaw onboard complete.")
	if createdConfig {
		fmt.Println("config: " + cfg.ConfigPath)
	} else {
		fmt.Println("config already exists: " + cfg.ConfigPath)
	}
	fmt.Println("workspace: " + cfg.Workspace)
	return 0
}

func runStatus(cfg config.Config) int {
	fmt.Println("MiniClaw status")
	fmt.Println("version: " + Version)
	fmt.Println("config: " + cfg.ConfigPath)
	fmt.Println("mcp config: " + cfg.MCPConfigPath)
	fmt.Println("home: " + cfg.HomeDir)
	fmt.Println("workspace: " + cfg.Workspace)
	fmt.Printf("api configured: %t\n", cfg.APIKey != "")
	fmt.Printf("mcp enabled: %t\n", cfg.EnableMCP)
	fmt.Printf("qq configured: %t\n", cfg.QQAppID != "" && cfg.QQAppSecret != "")
	fmt.Printf("qq token configured: %t\n", cfg.QQToken != "")
	fmt.Println("qq webhook: " + cfg.QQWebhookURL())
	fmt.Println("qq auth callback: " + cfg.QQAuthCallbackURL())
	fmt.Println("qq allow users: " + cfg.QQAllowUsers)
	fmt.Println("qq allow groups: " + cfg.QQAllowGroups)
	_, err := os.Stat(cfg.Workspace)
	fmt.Printf("workspace ready: %t\n", err == nil)
	return 0
}

func runGateway(cfg config.Config, args []string) int {
	if ensureRuntimeReady(cfg) != 0 {
		return 1
	}
	if strings.TrimSpace(cfg.QQAppID) == "" || strings.TrimSpace(cfg.QQAppSecret) == "" {
		fmt.Fprintln(os.Stderr, "QQ gateway is not configured yet.")
		fmt.Fprintln(os.Stderr, "set qq_app_id and qq_app_secret in ~/.config/miniclaw/config before enabling QQ integration.")
		return 1
	}
	token, err := qqgateway.FetchAccessToken(context.Background(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	profile, err := qqgateway.FetchBotProfile(context.Background(), cfg, token.AccessToken)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	statePath, err := qqgateway.WriteGatewayState(cfg, token, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to persist qq gateway state: %v\n", err)
		return 1
	}
	fmt.Println("QQ gateway bootstrap ok.")
	fmt.Println("bot id: " + profile.ID)
	fmt.Println("bot name: " + profile.Username)
	fmt.Println("state: " + statePath)
	if hasFlag(args, "--once") {
		fmt.Println("bootstrap-only mode finished.")
		return 0
	}
	fmt.Println("starting local webhook server on " + cfg.QQWebhookURL())
	fmt.Println("next step: bind this handler to a public HTTPS address or tunnel for QQ callback verification.")
	if err := qqgateway.StartWebhookServer(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func runAgent(cfg config.Config, args []string) int {
	if ensureRuntimeReady(cfg) != 0 {
		return 1
	}
	prompt := parsePromptArg(args)
	if strings.TrimSpace(prompt) != "" {
		recorder, err := session.New(cfg.Workspace)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create session recorder: %v\n", err)
			return 1
		}
		response, err := minimax.RunAgentInSession(context.Background(), cfg, prompt, recorder)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		fmt.Println(response)
		return 0
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		fmt.Fprintln(os.Stderr, "MINICLAW_API_KEY is not configured.")
		fmt.Fprintln(os.Stderr, "set it in ~/.config/miniclaw/config or export MINICLAW_API_KEY.")
		return 1
	}
	fmt.Println("MiniClaw interactive mode. Type exit to quit.")
	recorder, err := session.New(cfg.Workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create session recorder: %v\n", err)
		return 1
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			break
		}
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}
		response, err := minimax.RunAgentInSession(context.Background(), cfg, input, recorder)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			continue
		}
		fmt.Println(response)
	}
	return 0
}

func runMemory(cfg config.Config, args []string) int {
	if ensureRuntimeReady(cfg) != 0 {
		return 1
	}
	store := memory.NewStore(cfg.Workspace)
	settings := memory.SettingsFromConfig(cfg)
	if len(args) == 0 || args[0] == "show" {
		content := store.ContextWithSettings(settings)
		if content == "" {
			fmt.Println("(memory is empty)")
			return 0
		}
		fmt.Println(content)
		return 0
	}

	switch args[0] {
	case "set":
		content := parsePromptArg(args[1:])
		if content == "" {
			fmt.Fprintln(os.Stderr, "usage: miniclaw memory set -p \"content\"")
			return 1
		}
		if err := store.WriteLongTerm(content); err != nil {
			fmt.Fprintf(os.Stderr, "failed to update memory: %v\n", err)
			return 1
		}
		fmt.Println("memory updated: " + cfg.Workspace + string(os.PathSeparator) + filepathJoin("memory", "MEMORY.md"))
		return 0
	case "append":
		content := parsePromptArg(args[1:])
		if content == "" {
			fmt.Fprintln(os.Stderr, "usage: miniclaw memory append -p \"content\"")
			return 1
		}
		if err := store.AppendLongTerm(content); err != nil {
			fmt.Fprintf(os.Stderr, "failed to append memory: %v\n", err)
			return 1
		}
		fmt.Println("memory appended: " + cfg.Workspace + string(os.PathSeparator) + filepathJoin("memory", "MEMORY.md"))
		return 0
	case "today":
		content := parsePromptArg(args[1:])
		if content == "" {
			fmt.Fprintln(os.Stderr, "usage: miniclaw memory today -p \"content\"")
			return 1
		}
		if err := store.AppendToday(content); err != nil {
			fmt.Fprintf(os.Stderr, "failed to append daily note: %v\n", err)
			return 1
		}
		fmt.Println("daily note updated: " + store.TodayFile())
		return 0
	case "clear":
		if err := store.WriteLongTerm("# Memory\n\n"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to clear memory: %v\n", err)
			return 1
		}
		if err := store.WriteSummary("# Summary\n\n"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to clear summary: %v\n", err)
			return 1
		}
		fmt.Println("memory cleared: " + cfg.Workspace + string(os.PathSeparator) + filepathJoin("memory", "MEMORY.md"))
		return 0
	case "summarize":
		days := parseOptionalPositiveIntArgs(args[1:])
		if days == 0 {
			days = settings.RecentDays
		}
		summary, err := store.SummarizeRecentNotesWithSettings(days, settings)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to summarize memory: %v\n", err)
			return 1
		}
		if strings.TrimSpace(summary) == "" {
			fmt.Println("(no recent notes to summarize)")
			return 0
		}
		if err := store.WriteSummary("# Summary\n\n" + summary + "\n"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write summary: %v\n", err)
			return 1
		}
		fmt.Println("summary updated: " + cfg.Workspace + string(os.PathSeparator) + filepathJoin("memory", "SUMMARY.md"))
		return 0
	case "compact":
		if err := store.CompactLongTerm(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to compact memory: %v\n", err)
			return 1
		}
		fmt.Println("memory compacted: " + cfg.Workspace + string(os.PathSeparator) + filepathJoin("memory", "MEMORY.md"))
		return 0
	case "prune":
		keepDays := parseOptionalPositiveIntArgs(args[1:])
		if keepDays == 0 {
			keepDays = settings.PruneKeepDays
		}
		removed, err := store.PruneDailyNotes(keepDays)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to prune daily notes: %v\n", err)
			return 1
		}
		fmt.Printf("pruned %d daily note(s), kept last %d day(s)\n", removed, keepDays)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown memory command: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "usage: miniclaw memory [show|set|append|today|summarize|compact|prune|clear]")
		return 1
	}
}

func ensureRuntimeReady(cfg config.Config) int {
	if _, err := os.Stat(cfg.ConfigPath); err != nil {
		fmt.Fprintf(os.Stderr, "config not found: %s\n", cfg.ConfigPath)
		fmt.Fprintln(os.Stderr, "run `miniclaw onboard` first.")
		return 1
	}
	if err := workspace.Ensure(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "failed to prepare workspace: %v\n", err)
		return 1
	}
	return 0
}

func parsePromptArg(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "-p" || args[i] == "--prompt" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}

func parseOptionalPositiveIntArgs(args []string) int {
	for _, arg := range args {
		parsed, err := strconv.Atoi(arg)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func filepathJoin(parts ...string) string {
	return strings.Join(parts, string(os.PathSeparator))
}
