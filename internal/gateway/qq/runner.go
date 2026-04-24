package qq

import (
	"context"
	"fmt"

	"miniclaw2/internal/config"
)

type Runner struct{}

func (Runner) Name() string {
	return "qq"
}

func (Runner) Bootstrap(ctx context.Context, cfg config.Config) ([]string, error) {
	if cfg.QQAppID == "" || cfg.QQAppSecret == "" {
		return nil, fmt.Errorf("QQ gateway is not configured yet.\nset qq_app_id and qq_app_secret in ~/.config/miniclaw/config before enabling QQ integration.")
	}
	token, err := FetchAccessToken(ctx, cfg)
	if err != nil {
		return nil, err
	}
	profile, err := FetchBotProfile(ctx, cfg, token.AccessToken)
	if err != nil {
		return nil, err
	}
	statePath, err := WriteGatewayState(cfg, token, profile)
	if err != nil {
		return nil, fmt.Errorf("failed to persist qq gateway state: %v", err)
	}
	return []string{
		"QQ gateway bootstrap ok.",
		"bot id: " + profile.ID,
		"bot name: " + profile.Username,
		"state: " + statePath,
	}, nil
}

func (Runner) Start(ctx context.Context, cfg config.Config) error {
	_ = ctx
	return StartWebhookServer(cfg)
}

func (Runner) StartMessages(cfg config.Config) []string {
	return []string{
		"starting local webhook server on " + cfg.QQWebhookURL(),
		"next step: bind this handler to a public HTTPS address or tunnel for QQ callback verification.",
	}
}