package core

import (
	"context"
	"fmt"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func (c *Core) Webhook(ctx context.Context, ref string) (WebhookInfo, error) {
	if err := ctx.Err(); err != nil {
		return WebhookInfo{}, err
	}
	resolved, cfg, _, hook, err := c.resolveWebhook(ref)
	if err != nil {
		return WebhookInfo{}, err
	}
	last, err := c.events.LastReceived(ctx)
	if err != nil {
		return WebhookInfo{}, fmt.Errorf("reading the last webhook events: %w", err)
	}
	return webhookInfo(cfg, resolved, hook, last), nil
}

func webhookInfo(cfg *config.Config, ref registry.Ref, hook *config.Webhook, last map[string]time.Time) WebhookInfo {
	info := WebhookInfo{
		Ref:       ref.String(),
		Style:     hook.Style,
		Challenge: hook.Challenge,
	}
	publicURL := ""
	port := 0
	if cfg.Webhooks != nil {
		publicURL = cfg.Webhooks.PublicURL
		port = cfg.Webhooks.Port
	}
	if publicURL != "" {
		info.URL = webhookPublicURL(publicURL, ref.Service, ref.Account, hook.ID)
	}
	if port > 0 {
		info.LocalURL = webhookLocalURL(port, ref.Service, ref.Account, hook.ID)
	}
	if received, ok := last[info.Ref]; ok {
		info.LastEventAt = &received
	}
	return info
}
