package config

import "testing"

func TestParseContentAppliesMemorySettings(t *testing.T) {
	base := Default()
	content := `
memory_recent_days=4
memory_recent_chars=2500
memory_summary_max_lines=11
memory_summary_max_chars=3333
memory_daily_entry_max_chars=777
memory_significance_threshold=6
memory_prune_keep_days=9
`
	cfg := ParseContent(content, base)
	if cfg.MemoryRecentDays != 4 || cfg.MemoryRecentChars != 2500 || cfg.MemorySummaryMaxLines != 11 || cfg.MemorySummaryMaxChars != 3333 || cfg.MemoryDailyEntryMaxChars != 777 || cfg.MemorySignificanceThreshold != 6 || cfg.MemoryPruneKeepDays != 9 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseContentAppliesAnthropicBaseURLAliases(t *testing.T) {
	base := Default()
	if base.BaseURL != "https://api.minimaxi.com/anthropic" {
		t.Fatalf("unexpected default base url: %s", base.BaseURL)
	}
	fromBase := ParseContent("base_url=https://example.com/anthropic", base)
	if fromBase.BaseURL != "https://example.com/anthropic" {
		t.Fatalf("unexpected base_url value: %s", fromBase.BaseURL)
	}
	fromLegacy := ParseContent("api_url=https://legacy.example.com/anthropic", base)
	if fromLegacy.BaseURL != "https://legacy.example.com/anthropic" {
		t.Fatalf("unexpected api_url value: %s", fromLegacy.BaseURL)
	}
}

func TestParseContentAppliesGatewayChannelSettings(t *testing.T) {
	base := Default()
	content := `
gateway_channel=weixin
weixin_api_base=https://weixin.example.test
weixin_cdn_base=https://cdn.example.test
weixin_token=token-1
weixin_account_id=bot-1
weixin_allow_users=u-1,u-2
weixin_processing_text=处理中
`
	cfg := ParseContent(content, base)
	if cfg.GatewayChannel != "weixin" || cfg.WeixinAPIBase != "https://weixin.example.test" || cfg.WeixinCDNBase != "https://cdn.example.test" || cfg.WeixinToken != "token-1" || cfg.WeixinAccountID != "bot-1" || cfg.WeixinAllowUsers != "u-1,u-2" || cfg.WeixinProcessingText != "处理中" {
		t.Fatalf("unexpected gateway config: %+v", cfg)
	}
}
