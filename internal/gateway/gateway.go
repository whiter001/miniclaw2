package gateway

import (
	"context"
	"fmt"
	"strings"

	"miniclaw2/internal/config"
	qqgateway "miniclaw2/internal/gateway/qq"
	"miniclaw2/internal/gateway/weixin"
)

type Runner interface {
	Name() string
	Bootstrap(ctx context.Context, cfg config.Config) ([]string, error)
	Start(ctx context.Context, cfg config.Config) error
	StartMessages(cfg config.Config) []string
}

func Resolve(name string) (Runner, error) {
	switch NormalizeChannelName(name) {
	case "qq":
		return qqgateway.Runner{}, nil
	case "weixin":
		return weixin.Runner{}, nil
	default:
		return nil, fmt.Errorf("unsupported gateway channel: %s", strings.TrimSpace(name))
	}
}

func NormalizeChannelName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "qq":
		return "qq"
	case "wechat", "weixin":
		return "weixin"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}