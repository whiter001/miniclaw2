package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	HomeDir                     string  `json:"home_dir"`
	Workspace                   string  `json:"workspace"`
	ConfigPath                  string  `json:"config_path"`
	MCPConfigPath               string  `json:"mcp_config_path"`
	APIKey                      string  `json:"api_key"`
	BaseURL                     string  `json:"base_url"`
	Model                       string  `json:"model"`
	Temperature                 float64 `json:"temperature"`
	MaxTokens                   int     `json:"max_tokens"`
	RequestTimeout              int     `json:"request_timeout"`
	EnableMCP                   bool    `json:"enable_mcp"`
	MCPBasePath                 string  `json:"mcp_base_path"`
	MCPResourceMode             string  `json:"mcp_resource_mode"`
	GatewayChannel              string  `json:"gateway_channel"`
	QQAppID                     string  `json:"qq_app_id"`
	QQToken                     string  `json:"qq_token"`
	QQAppSecret                 string  `json:"qq_app_secret"`
	QQAPIBase                   string  `json:"qq_api_base"`
	QQWebhookHost               string  `json:"qq_webhook_host"`
	QQWebhookPort               int     `json:"qq_webhook_port"`
	QQWebhookPath               string  `json:"qq_webhook_path"`
	QQAuthCallbackPath          string  `json:"qq_auth_callback_path"`
	QQAllowUsers                string  `json:"qq_allow_users"`
	QQAllowGroups               string  `json:"qq_allow_groups"`
	QQProcessingText            string  `json:"qq_processing_text"`
	WeixinAPIBase               string  `json:"weixin_api_base"`
	WeixinCDNBase               string  `json:"weixin_cdn_base"`
	WeixinToken                 string  `json:"weixin_token"`
	WeixinAccountID             string  `json:"weixin_account_id"`
	WeixinAllowUsers            string  `json:"weixin_allow_users"`
	WeixinProcessingText        string  `json:"weixin_processing_text"`
	MaxToolIterations           int     `json:"max_tool_iterations"`
	MemoryRecentDays            int     `json:"memory_recent_days"`
	MemoryRecentChars           int     `json:"memory_recent_chars"`
	MemorySummaryMaxLines       int     `json:"memory_summary_max_lines"`
	MemorySummaryMaxChars       int     `json:"memory_summary_max_chars"`
	MemoryDailyEntryMaxChars    int     `json:"memory_daily_entry_max_chars"`
	MemorySignificanceThreshold int     `json:"memory_significance_threshold"`
	MemoryPruneKeepDays         int     `json:"memory_prune_keep_days"`
	EnableAutoSkills            bool    `json:"enable_auto_skills"`
	EnableSkillScoring          bool    `json:"enable_skill_scoring"`
	AutoSkillMinToolCalls       int     `json:"auto_skill_min_tool_calls"`
	AutoSkillMaxExamples        int     `json:"auto_skill_max_examples"`
	SkillSelectionLimit         int     `json:"skill_selection_limit"`
}

func Default() Config {
	homeDir := filepath.Join(mustUserHomeDir(), ".miniclaw")
	return Config{
		HomeDir:                     homeDir,
		Workspace:                   filepath.Join(homeDir, "workspace"),
		ConfigPath:                  filepath.Join(mustUserHomeDir(), ".config", "miniclaw", "config"),
		MCPConfigPath:               filepath.Join(mustUserHomeDir(), ".config", "miniclaw", "mcp.json"),
		APIKey:                      "",
		BaseURL:                     "https://api.minimaxi.com/anthropic",
		Model:                       "MiniMax-M2.7",
		Temperature:                 0.7,
		MaxTokens:                   8192,
		RequestTimeout:              120,
		EnableMCP:                   false,
		MCPBasePath:                 "",
		MCPResourceMode:             "url",
		GatewayChannel:              "qq",
		QQAppID:                     "",
		QQToken:                     "",
		QQAppSecret:                 "",
		QQAPIBase:                   "https://api.sgroup.qq.com",
		QQWebhookHost:               "127.0.0.1",
		QQWebhookPort:               8080,
		QQWebhookPath:               "/webhook/qq",
		QQAuthCallbackPath:          "/qq-callback",
		QQAllowUsers:                "",
		QQAllowGroups:               "",
		QQProcessingText:            "收到，处理中，请稍候。",
		WeixinAPIBase:               "https://ilinkai.weixin.qq.com",
		WeixinCDNBase:               "https://novac2c.cdn.weixin.qq.com/c2c",
		WeixinToken:                 "",
		WeixinAccountID:             "",
		WeixinAllowUsers:            "",
		WeixinProcessingText:        "收到，处理中，请稍候。",
		MaxToolIterations:           100,
		MemoryRecentDays:            2,
		MemoryRecentChars:           1600,
		MemorySummaryMaxLines:       20,
		MemorySummaryMaxChars:       2000,
		MemoryDailyEntryMaxChars:    500,
		MemorySignificanceThreshold: 3,
		MemoryPruneKeepDays:         14,
		EnableAutoSkills:            true,
		EnableSkillScoring:          true,
		AutoSkillMinToolCalls:       2,
		AutoSkillMaxExamples:        5,
		SkillSelectionLimit:         2,
	}
}

func Load() Config {
	cfg := Default()
	cfg.HomeDir = ExpandHomePath(cfg.HomeDir)
	cfg.Workspace = ExpandHomePath(cfg.Workspace)
	cfg.ConfigPath = ExpandHomePath(cfg.ConfigPath)
	cfg.MCPConfigPath = ExpandHomePath(cfg.MCPConfigPath)
	cfg.MCPBasePath = ExpandHomePath(cfg.MCPBasePath)

	if data, err := os.ReadFile(cfg.ConfigPath); err == nil {
		cfg = ParseContent(string(data), cfg)
	}
	ApplyEnvOverrides(&cfg)
	return cfg
}

func ParseContent(content string, base Config) Config {
	cfg := base
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		value := strings.TrimSpace(trimmed[eq+1:])
		applyConfigValue(&cfg, key, value)
	}
	cfg.HomeDir = ExpandHomePath(cfg.HomeDir)
	cfg.Workspace = ExpandHomePath(cfg.Workspace)
	cfg.ConfigPath = ExpandHomePath(cfg.ConfigPath)
	cfg.MCPConfigPath = ExpandHomePath(cfg.MCPConfigPath)
	cfg.MCPBasePath = ExpandHomePath(cfg.MCPBasePath)
	return cfg
}

func ApplyEnvOverrides(cfg *Config) {
	applyEnvString("MINICLAW_HOME", &cfg.HomeDir, true)
	applyEnvString("MINICLAW_WORKSPACE", &cfg.Workspace, true)
	applyEnvString("MINICLAW_MCP_CONFIG_PATH", &cfg.MCPConfigPath, true)
	applyEnvString("MINICLAW_API_KEY", &cfg.APIKey, false)
	applyEnvString("ANTHROPIC_BASE_URL", &cfg.BaseURL, false)
	applyEnvString("MINICLAW_API_URL", &cfg.BaseURL, false)
	applyEnvString("MINICLAW_MODEL", &cfg.Model, false)
	applyEnvString("MINICLAW_QQ_APP_ID", &cfg.QQAppID, false)
	applyEnvString("MINICLAW_QQ_TOKEN", &cfg.QQToken, false)
	applyEnvString("MINICLAW_QQ_APP_SECRET", &cfg.QQAppSecret, false)
	applyEnvString("MINICLAW_QQ_API_BASE", &cfg.QQAPIBase, false)
	applyEnvString("MINICLAW_QQ_WEBHOOK_HOST", &cfg.QQWebhookHost, false)
	applyEnvInt("MINICLAW_QQ_WEBHOOK_PORT", &cfg.QQWebhookPort, nil)
	applyEnvString("MINICLAW_QQ_WEBHOOK_PATH", &cfg.QQWebhookPath, false)
	applyEnvString("MINICLAW_QQ_AUTH_CALLBACK_PATH", &cfg.QQAuthCallbackPath, false)
	applyEnvString("MINICLAW_QQ_ALLOW_USERS", &cfg.QQAllowUsers, false)
	applyEnvString("MINICLAW_QQ_ALLOW_GROUPS", &cfg.QQAllowGroups, false)
	applyEnvString("MINICLAW_QQ_PROCESSING_TEXT", &cfg.QQProcessingText, false)
	applyEnvFloat("MINICLAW_TEMPERATURE", &cfg.Temperature)
	applyEnvInt("MINICLAW_MEMORY_RECENT_DAYS", &cfg.MemoryRecentDays, func(v int) bool { return v > 0 })
	applyEnvInt("MINICLAW_MEMORY_RECENT_CHARS", &cfg.MemoryRecentChars, func(v int) bool { return v > 0 })
	applyEnvInt("MINICLAW_MEMORY_SUMMARY_MAX_LINES", &cfg.MemorySummaryMaxLines, func(v int) bool { return v > 0 })
	applyEnvInt("MINICLAW_MEMORY_SUMMARY_MAX_CHARS", &cfg.MemorySummaryMaxChars, func(v int) bool { return v > 0 })
	applyEnvInt("MINICLAW_MEMORY_DAILY_ENTRY_MAX_CHARS", &cfg.MemoryDailyEntryMaxChars, func(v int) bool { return v > 0 })
	applyEnvInt("MINICLAW_MEMORY_SIGNIFICANCE_THRESHOLD", &cfg.MemorySignificanceThreshold, func(v int) bool { return v > 0 })
	applyEnvInt("MINICLAW_MEMORY_PRUNE_KEEP_DAYS", &cfg.MemoryPruneKeepDays, func(v int) bool { return v >= 0 })
	applyEnvBool("MINICLAW_ENABLE_AUTO_SKILLS", &cfg.EnableAutoSkills)
	applyEnvBool("MINICLAW_ENABLE_SKILL_SCORING", &cfg.EnableSkillScoring)
	applyEnvInt("MINICLAW_AUTO_SKILL_MIN_TOOL_CALLS", &cfg.AutoSkillMinToolCalls, func(v int) bool { return v > 0 })
	applyEnvInt("MINICLAW_AUTO_SKILL_MAX_EXAMPLES", &cfg.AutoSkillMaxExamples, func(v int) bool { return v > 0 })
	applyEnvInt("MINICLAW_SKILL_SELECTION_LIMIT", &cfg.SkillSelectionLimit, func(v int) bool { return v > 0 && v <= 20 })
	applyEnvInt("MINICLAW_MAX_TOKENS", &cfg.MaxTokens, nil)
	applyEnvInt("MINICLAW_REQUEST_TIMEOUT", &cfg.RequestTimeout, nil)
	applyEnvInt("MINICLAW_MAX_TOOL_ITERATIONS", &cfg.MaxToolIterations, func(v int) bool { return v >= 10 && v <= 1000 })
	applyEnvBool("MINICLAW_ENABLE_MCP", &cfg.EnableMCP)
	applyEnvString("MINICLAW_MCP_BASE_PATH", &cfg.MCPBasePath, true)
	applyEnvString("MINICLAW_MCP_RESOURCE_MODE", &cfg.MCPResourceMode, false)
	applyEnvString("MINICLAW_GATEWAY_CHANNEL", &cfg.GatewayChannel, false)
	applyEnvString("MINICLAW_WEIXIN_API_BASE", &cfg.WeixinAPIBase, false)
	applyEnvString("MINICLAW_WEIXIN_CDN_BASE", &cfg.WeixinCDNBase, false)
	applyEnvString("MINICLAW_WEIXIN_TOKEN", &cfg.WeixinToken, false)
	applyEnvString("MINICLAW_WEIXIN_ACCOUNT_ID", &cfg.WeixinAccountID, false)
	applyEnvString("MINICLAW_WEIXIN_ALLOW_USERS", &cfg.WeixinAllowUsers, false)
	applyEnvString("MINICLAW_WEIXIN_PROCESSING_TEXT", &cfg.WeixinProcessingText, false)

	cfg.HomeDir = ExpandHomePath(cfg.HomeDir)
	cfg.Workspace = ExpandHomePath(cfg.Workspace)
	cfg.ConfigPath = ExpandHomePath(cfg.ConfigPath)
	cfg.MCPConfigPath = ExpandHomePath(cfg.MCPConfigPath)
	cfg.MCPBasePath = ExpandHomePath(cfg.MCPBasePath)
}

func EnsureConfigParentDir(configPath string) error {
	parent := filepath.Dir(configPath)
	if parent == "" || parent == "." {
		return nil
	}
	return os.MkdirAll(parent, 0o755)
}

func WriteDefault(cfg Config) error {
	if err := EnsureConfigParentDir(cfg.ConfigPath); err != nil {
		return err
	}
	content := strings.Join([]string{
		"# MiniClaw config",
		"home_dir=" + cfg.HomeDir,
		"workspace=" + cfg.Workspace,
		"mcp_config_path=" + cfg.MCPConfigPath,
		"api_key=",
		"base_url=" + cfg.BaseURL,
		"model=" + cfg.Model,
		"temperature=" + strconv.FormatFloat(cfg.Temperature, 'f', -1, 64),
		"max_tokens=" + strconv.Itoa(cfg.MaxTokens),
		"request_timeout=" + strconv.Itoa(cfg.RequestTimeout),
		"max_tool_iterations=" + strconv.Itoa(cfg.MaxToolIterations),
		"enable_mcp=" + strconv.FormatBool(cfg.EnableMCP),
		"mcp_base_path=" + cfg.MCPBasePath,
		"mcp_resource_mode=" + cfg.MCPResourceMode,
		"gateway_channel=" + cfg.GatewayChannel,
		"qq_app_id=",
		"qq_token=",
		"qq_app_secret=",
		"qq_api_base=" + cfg.QQAPIBase,
		"qq_webhook_host=" + cfg.QQWebhookHost,
		"qq_webhook_port=" + strconv.Itoa(cfg.QQWebhookPort),
		"qq_webhook_path=" + cfg.QQWebhookPath,
		"qq_auth_callback_path=" + cfg.QQAuthCallbackPath,
		"qq_allow_users=" + cfg.QQAllowUsers,
		"qq_allow_groups=" + cfg.QQAllowGroups,
		"qq_processing_text=" + cfg.QQProcessingText,
		"weixin_api_base=" + cfg.WeixinAPIBase,
		"weixin_cdn_base=" + cfg.WeixinCDNBase,
		"weixin_token=",
		"weixin_account_id=" + cfg.WeixinAccountID,
		"weixin_allow_users=" + cfg.WeixinAllowUsers,
		"weixin_processing_text=" + cfg.WeixinProcessingText,
		"memory_recent_days=" + strconv.Itoa(cfg.MemoryRecentDays),
		"memory_recent_chars=" + strconv.Itoa(cfg.MemoryRecentChars),
		"memory_summary_max_lines=" + strconv.Itoa(cfg.MemorySummaryMaxLines),
		"memory_summary_max_chars=" + strconv.Itoa(cfg.MemorySummaryMaxChars),
		"memory_daily_entry_max_chars=" + strconv.Itoa(cfg.MemoryDailyEntryMaxChars),
		"memory_significance_threshold=" + strconv.Itoa(cfg.MemorySignificanceThreshold),
		"memory_prune_keep_days=" + strconv.Itoa(cfg.MemoryPruneKeepDays),
		"enable_auto_skills=" + strconv.FormatBool(cfg.EnableAutoSkills),
		"enable_skill_scoring=" + strconv.FormatBool(cfg.EnableSkillScoring),
		"auto_skill_min_tool_calls=" + strconv.Itoa(cfg.AutoSkillMinToolCalls),
		"auto_skill_max_examples=" + strconv.Itoa(cfg.AutoSkillMaxExamples),
		"skill_selection_limit=" + strconv.Itoa(cfg.SkillSelectionLimit),
	}, "\n") + "\n"
	return os.WriteFile(cfg.ConfigPath, []byte(content), 0o644)
}

func ExpandHomePath(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		return mustUserHomeDir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		return filepath.Join(mustUserHomeDir(), path[2:])
	}
	return path
}

func mustUserHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}

func applyConfigValue(cfg *Config, key, value string) {
	switch key {
	case "home_dir":
		cfg.HomeDir = value
	case "workspace":
		cfg.Workspace = value
	case "mcp_config_path":
		cfg.MCPConfigPath = value
	case "api_key":
		cfg.APIKey = value
	case "base_url", "api_url":
		cfg.BaseURL = value
	case "model":
		cfg.Model = value
	case "temperature":
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Temperature = parsed
		}
	case "max_tokens":
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.MaxTokens = parsed
		}
	case "request_timeout":
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.RequestTimeout = parsed
		}
	case "max_tool_iterations":
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 10 && parsed <= 1000 {
			cfg.MaxToolIterations = parsed
		}
	case "memory_recent_days":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.MemoryRecentDays = parsed
		}
	case "memory_recent_chars":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.MemoryRecentChars = parsed
		}
	case "memory_summary_max_lines":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.MemorySummaryMaxLines = parsed
		}
	case "memory_summary_max_chars":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.MemorySummaryMaxChars = parsed
		}
	case "memory_daily_entry_max_chars":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.MemoryDailyEntryMaxChars = parsed
		}
	case "memory_significance_threshold":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.MemorySignificanceThreshold = parsed
		}
	case "memory_prune_keep_days":
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
			cfg.MemoryPruneKeepDays = parsed
		}
	case "enable_auto_skills":
		cfg.EnableAutoSkills = value == "true" || value == "1"
	case "enable_skill_scoring":
		cfg.EnableSkillScoring = value == "true" || value == "1"
	case "auto_skill_min_tool_calls":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.AutoSkillMinToolCalls = parsed
		}
	case "auto_skill_max_examples":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			cfg.AutoSkillMaxExamples = parsed
		}
	case "skill_selection_limit":
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 20 {
			cfg.SkillSelectionLimit = parsed
		}
	case "enable_mcp":
		cfg.EnableMCP = value == "true" || value == "1"
	case "mcp_base_path":
		cfg.MCPBasePath = value
	case "mcp_resource_mode":
		cfg.MCPResourceMode = value
	case "gateway_channel":
		cfg.GatewayChannel = value
	case "qq_app_id":
		cfg.QQAppID = value
	case "qq_token":
		cfg.QQToken = value
	case "qq_app_secret":
		cfg.QQAppSecret = value
	case "qq_api_base":
		cfg.QQAPIBase = value
	case "qq_webhook_host":
		cfg.QQWebhookHost = value
	case "qq_webhook_port":
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.QQWebhookPort = parsed
		}
	case "qq_webhook_path":
		cfg.QQWebhookPath = value
	case "qq_auth_callback_path":
		cfg.QQAuthCallbackPath = value
	case "qq_allow_users":
		cfg.QQAllowUsers = value
	case "qq_allow_groups":
		cfg.QQAllowGroups = value
	case "qq_processing_text":
		cfg.QQProcessingText = value
	case "weixin_api_base":
		cfg.WeixinAPIBase = value
	case "weixin_cdn_base":
		cfg.WeixinCDNBase = value
	case "weixin_token":
		cfg.WeixinToken = value
	case "weixin_account_id":
		cfg.WeixinAccountID = value
	case "weixin_allow_users":
		cfg.WeixinAllowUsers = value
	case "weixin_processing_text":
		cfg.WeixinProcessingText = value
	}
}

func applyEnvString(name string, target *string, expandHome bool) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return
	}
	if expandHome {
		*target = ExpandHomePath(value)
		return
	}
	*target = value
}

func applyEnvInt(name string, target *int, validator func(int) bool) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return
	}
	if validator != nil && !validator(parsed) {
		return
	}
	*target = parsed
}

func applyEnvFloat(name string, target *float64) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err == nil {
		*target = parsed
	}
}

func applyEnvBool(name string, target *bool) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	*target = value == "true" || value == "1"
}

func (cfg Config) QQWebhookURL() string {
	return fmt.Sprintf("http://%s:%d%s", cfg.QQWebhookHost, cfg.QQWebhookPort, cfg.QQWebhookPath)
}

func (cfg Config) QQAuthCallbackURL() string {
	return fmt.Sprintf("http://%s:%d%s", cfg.QQWebhookHost, cfg.QQWebhookPort, cfg.QQAuthCallbackPath)
}
