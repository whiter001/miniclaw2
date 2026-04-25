package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	cronpkg "miniclaw2/internal/cron"
	"miniclaw2/internal/config"
	"miniclaw2/internal/gateway"
	"miniclaw2/internal/gateway/weixin"
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
	case "cron":
		return runCron(cfg, commandArgs)
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
		case "--channel":
			if i+1 < len(args) {
				cfg.GatewayChannel = gateway.NormalizeChannelName(args[i+1])
				i++
			}
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
	fmt.Println("  miniclaw cron [list|run|serve|trigger]   Manage serial cron tasks in workspace/cron")
	fmt.Println("  miniclaw gateway [--channel qq|weixin] [--once] [--webhook-port PORT]   Start gateway bootstrap or channel runner")
	fmt.Println("  miniclaw gateway login [--channel weixin] [--verbose] [--no-open]   Login and save a Weixin account")
	fmt.Println("  miniclaw gateway accounts [--channel weixin] [--use ACCOUNT]   List or activate Weixin accounts")
	fmt.Println("  miniclaw gateway logout [--channel weixin] [--account ACCOUNT]   Remove a Weixin account")
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
	fmt.Println("  MINICLAW_GATEWAY_CHANNEL")
	fmt.Println("  MINICLAW_QQ_APP_ID")
	fmt.Println("  MINICLAW_QQ_APP_SECRET")
	fmt.Println("  MINICLAW_WEIXIN_API_BASE")
	fmt.Println("  MINICLAW_WEIXIN_CDN_BASE")
	fmt.Println("  MINICLAW_WEIXIN_TOKEN")
	fmt.Println("  MINICLAW_WEIXIN_ACCOUNT_ID")
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
	accountIDs, _ := weixin.ListAccountIDs(cfg)
	activeWeixinAccount := weixin.LoadActiveAccountID(cfg)
	weixinConfigured := strings.TrimSpace(cfg.WeixinToken) != "" || len(accountIDs) > 0
	fmt.Println("MiniClaw status")
	fmt.Println("version: " + Version)
	fmt.Println("config: " + cfg.ConfigPath)
	fmt.Println("mcp config: " + cfg.MCPConfigPath)
	fmt.Println("home: " + cfg.HomeDir)
	fmt.Println("workspace: " + cfg.Workspace)
	fmt.Println("gateway channel: " + gateway.NormalizeChannelName(cfg.GatewayChannel))
	fmt.Printf("api configured: %t\n", cfg.APIKey != "")
	fmt.Printf("mcp enabled: %t\n", cfg.EnableMCP)
	fmt.Printf("qq configured: %t\n", cfg.QQAppID != "" && cfg.QQAppSecret != "")
	fmt.Printf("weixin configured: %t\n", weixinConfigured)
	fmt.Printf("qq token configured: %t\n", cfg.QQToken != "")
	fmt.Println("qq webhook: " + cfg.QQWebhookURL())
	fmt.Println("qq auth callback: " + cfg.QQAuthCallbackURL())
	fmt.Println("qq allow users: " + cfg.QQAllowUsers)
	fmt.Println("qq allow groups: " + cfg.QQAllowGroups)
	fmt.Println("weixin api base: " + cfg.WeixinAPIBase)
	fmt.Println("weixin cdn base: " + cfg.WeixinCDNBase)
	fmt.Println("weixin active account: " + activeWeixinAccount)
	fmt.Printf("weixin stored accounts: %d\n", len(accountIDs))
	fmt.Println("weixin allow users: " + cfg.WeixinAllowUsers)
	_, err := os.Stat(cfg.Workspace)
	fmt.Printf("workspace ready: %t\n", err == nil)
	return 0
}

func runGateway(cfg config.Config, args []string) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "login":
			return runGatewayLogin(cfg, args[1:])
		case "accounts":
			return runGatewayAccounts(cfg, args[1:])
		case "logout":
			return runGatewayLogout(cfg, args[1:])
		default:
			fmt.Fprintf(os.Stderr, "unknown gateway subcommand: %s\n", args[0])
			fmt.Fprintln(os.Stderr, "usage: miniclaw gateway [login|accounts|logout] [flags]")
			return 1
		}
	}
	if ensureRuntimeReady(cfg) != 0 {
		return 1
	}
	runner, err := gateway.Resolve(cfg.GatewayChannel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	lines, err := runner.Bootstrap(context.Background(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	if hasFlag(args, "--once") {
		fmt.Println("bootstrap-only mode finished.")
		return 0
	}
	for _, line := range runner.StartMessages(cfg) {
		fmt.Println(line)
	}
	if err := runner.Start(context.Background(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func runGatewayLogin(cfg config.Config, args []string) int {
	if ensureRuntimeReady(cfg) != 0 {
		return 1
	}
	ensureWeixinGatewayChannel(&cfg)
	if gateway.NormalizeChannelName(cfg.GatewayChannel) != "weixin" {
		fmt.Fprintln(os.Stderr, "gateway login currently only supports --channel weixin")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	openQRCodePage := !hasFlag(args, "--no-open")
	result, err := weixin.LoginWithQR(ctx, cfg, weixin.QRLoginOptions{
		Verbose: hasFlag(args, "--verbose"),
		Logf:    func(line string) { fmt.Println(line) },
		Timeout: parseOptionalDuration(args, "--timeout"),
		OnQRReady: func(start weixin.QRLoginStartResult) {
			if strings.TrimSpace(start.QRCodeURL) == "" {
				return
			}
			fmt.Println("二维码页面链接: " + start.QRCodeURL)
			if !openQRCodePage {
				fmt.Println("手工打开命令: " + browserOpenHint(start.QRCodeURL))
				return
			}
			if err := openBrowserURL(start.QRCodeURL); err != nil {
				fmt.Println("无法自动打开浏览器，请手工执行: " + browserOpenHint(start.QRCodeURL))
				return
			}
			fmt.Println("已尝试在默认浏览器打开二维码页面。")
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if result.QRCodeURL != "" {
		fmt.Println("qrcode page: " + result.QRCodeURL)
	}
	fmt.Println("weixin account saved: " + result.AccountID)
	if result.UserID != "" {
		fmt.Println("user id: " + result.UserID)
	}
	if result.BaseURL != "" {
		fmt.Println("api base: " + result.BaseURL)
	}
	return 0
}

func openBrowserURL(rawURL string) error {
	name, args, err := browserOpenCommand(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func browserOpenHint(rawURL string) string {
	name, args, err := browserOpenCommand(runtime.GOOS, rawURL)
	if err != nil {
		return rawURL
	}
	parts := append([]string{name}, args...)
	return strings.Join(parts, " ")
}

func browserOpenCommand(goos, rawURL string) (string, []string, error) {
	url := strconv.Quote(strings.TrimSpace(rawURL))
	if url == `""` {
		return "", nil, fmt.Errorf("empty url")
	}
	switch goos {
	case "darwin":
		return "open", []string{url[1 : len(url)-1]}, nil
	case "linux":
		return "xdg-open", []string{url[1 : len(url)-1]}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url[1 : len(url)-1]}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform: %s", goos)
	}
}

func runGatewayAccounts(cfg config.Config, args []string) int {
	if ensureRuntimeReady(cfg) != 0 {
		return 1
	}
	ensureWeixinGatewayChannel(&cfg)
	if gateway.NormalizeChannelName(cfg.GatewayChannel) != "weixin" {
		fmt.Fprintln(os.Stderr, "gateway accounts currently only supports --channel weixin")
		return 1
	}
	ids, err := weixin.ListAccountIDs(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list weixin accounts: %v\n", err)
		return 1
	}
	requested := weixin.NormalizeAccountID(flagValue(args, "--use"))
	if requested != "" {
		if !containsString(ids, requested) {
			fmt.Fprintf(os.Stderr, "unknown weixin account: %s\n", requested)
			return 1
		}
		if err := weixin.SetActiveAccountID(cfg, requested); err != nil {
			fmt.Fprintf(os.Stderr, "failed to activate weixin account: %v\n", err)
			return 1
		}
		fmt.Println("active weixin account: " + requested)
	}
	if len(ids) == 0 {
		fmt.Println("no weixin accounts configured.")
		fmt.Println("run `miniclaw gateway login --channel weixin` first.")
		return 0
	}
	active := weixin.LoadActiveAccountID(cfg)
	for _, id := range ids {
		marker := "-"
		if id == active {
			marker = "*"
		}
		data, err := weixin.LoadAccount(cfg, id)
		if err != nil {
			fmt.Printf("%s %s\n", marker, id)
			continue
		}
		details := []string{}
		if strings.TrimSpace(data.UserID) != "" {
			details = append(details, "user_id="+strings.TrimSpace(data.UserID))
		}
		if strings.TrimSpace(data.BaseURL) != "" {
			details = append(details, "base="+strings.TrimSpace(data.BaseURL))
		}
		if strings.TrimSpace(data.SavedAt) != "" {
			details = append(details, "saved_at="+strings.TrimSpace(data.SavedAt))
		}
		if len(details) == 0 {
			fmt.Printf("%s %s\n", marker, id)
			continue
		}
		fmt.Printf("%s %s (%s)\n", marker, id, strings.Join(details, ", "))
	}
	return 0
}

func runCron(cfg config.Config, args []string) int {
	if ensureRuntimeReady(cfg) != 0 {
		return 1
	}
	command := "list"
	commandArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		commandArgs = args[1:]
	}
	switch command {
	case "list":
		statuses, err := cronpkg.InspectTasks(cfg.Workspace, time.Now())
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		if len(statuses) == 0 {
			fmt.Println("no cron tasks configured.")
			fmt.Println("create JSON task files under " + cfg.Workspace + string(os.PathSeparator) + "cron")
			return 0
		}
		for _, status := range statuses {
			fmt.Printf("- %s enabled=%t due=%t running=%t next=%s last=%s\n",
				status.Task.ID,
				status.Task.Enabled,
				status.Due,
				status.State.Running,
				formatCronTime(status.State.NextRunAt),
				firstCronValue(status.State.LastStatus, "-"),
			)
		}
		return 0
	case "run":
		summary, err := cronpkg.RunDueTasksOnce(context.Background(), cfg, time.Now())
		for _, result := range summary.Results {
			if result.Status == "idle" {
				continue
			}
			fmt.Printf("- %s %s next=%s\n", result.TaskID, result.Status, formatCronTime(result.NextRunAt))
			if strings.TrimSpace(result.Message) != "" {
				fmt.Println("  " + result.Message)
			}
		}
		fmt.Printf("cron run summary: evaluated=%d ran=%d skipped=%d failed=%d\n", summary.Evaluated, summary.Ran, summary.Skipped, summary.Failed)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		return 0
	case "trigger":
		taskID := flagValue(commandArgs, "--id")
		if strings.TrimSpace(taskID) == "" && len(commandArgs) > 0 && !strings.HasPrefix(commandArgs[0], "-") {
			taskID = commandArgs[0]
		}
		if strings.TrimSpace(taskID) == "" {
			fmt.Fprintln(os.Stderr, "usage: miniclaw cron trigger --id TASK_ID")
			return 1
		}
		result, err := cronpkg.RunTaskByID(context.Background(), cfg, taskID, time.Now())
		fmt.Printf("- %s %s next=%s\n", result.TaskID, result.Status, formatCronTime(result.NextRunAt))
		if strings.TrimSpace(result.Message) != "" {
			fmt.Println(result.Message)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		return 0
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		pollInterval := parseOptionalDuration(commandArgs, "--poll")
		err := cronpkg.Serve(ctx, cfg, pollInterval, func(line string) { fmt.Println(line) })
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown cron subcommand: %s\n", command)
		fmt.Fprintln(os.Stderr, "usage: miniclaw cron [list|run|serve|trigger]")
		return 1
	}
}

func runGatewayLogout(cfg config.Config, args []string) int {
	if ensureRuntimeReady(cfg) != 0 {
		return 1
	}
	ensureWeixinGatewayChannel(&cfg)
	if gateway.NormalizeChannelName(cfg.GatewayChannel) != "weixin" {
		fmt.Fprintln(os.Stderr, "gateway logout currently only supports --channel weixin")
		return 1
	}
	target, err := resolveWeixinAccountID(cfg, flagValue(args, "--account"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if err := weixin.ClearAccount(cfg, target); err != nil {
		fmt.Fprintf(os.Stderr, "failed to remove weixin account: %v\n", err)
		return 1
	}
	remaining, err := weixin.ListAccountIDs(cfg)
	if err == nil && len(remaining) > 0 {
		active := weixin.LoadActiveAccountID(cfg)
		if active == "" || active == target {
			_ = weixin.SetActiveAccountID(cfg, remaining[0])
		}
	}
	fmt.Println("removed weixin account: " + target)
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

func parseOptionalDuration(args []string, flag string) time.Duration {
	value := flagValue(args, flag)
	if strings.TrimSpace(value) == "" {
		return 0
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func flagValue(args []string, flag string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func ensureWeixinGatewayChannel(cfg *config.Config) {
	channel := gateway.NormalizeChannelName(cfg.GatewayChannel)
	if channel == "" || channel == "qq" {
		cfg.GatewayChannel = "weixin"
	}
}

func resolveWeixinAccountID(cfg config.Config, requested string) (string, error) {
	selected := weixin.NormalizeAccountID(strings.TrimSpace(requested))
	if selected == "" {
		selected = weixin.NormalizeAccountID(strings.TrimSpace(cfg.WeixinAccountID))
	}
	if selected == "" {
		selected = weixin.LoadActiveAccountID(cfg)
	}
	ids, err := weixin.ListAccountIDs(cfg)
	if err != nil {
		return "", err
	}
	if selected == "" {
		switch len(ids) {
		case 0:
			return "", fmt.Errorf("no weixin accounts configured")
		case 1:
			return ids[0], nil
		default:
			return "", fmt.Errorf("multiple weixin accounts are registered; use --account to choose one")
		}
	}
	if !containsString(ids, selected) {
		return "", fmt.Errorf("unknown weixin account: %s", selected)
	}
	return selected, nil
}

func formatCronTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339)
}

func firstCronValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func filepathJoin(parts ...string) string {
	return strings.Join(parts, string(os.PathSeparator))
}
