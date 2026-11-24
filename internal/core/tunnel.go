package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/tunnel"
)

const TunnelPath = "/pandora"

type TunnelInfo struct {
	Provider  string `json:"provider,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
	Port      int    `json:"port,omitempty"`
	Exposed   bool   `json:"exposed"`
	Problem   string `json:"problem,omitempty"`
}

func (c *Core) UseTunnel(ctx context.Context, provider, base string) (TunnelInfo, error) {
	switch provider {
	case "tailscale", "url":
	default:
		return TunnelInfo{}, fmt.Errorf("unknown tunnel %s: use tailscale or url", provider)
	}
	if provider == "url" && base == "" {
		return TunnelInfo{}, errors.New("the url tunnel needs a public URL: run pandora tunnel use url <base>")
	}
	port, err := c.EnsureWebhookPort(ctx)
	if err != nil {
		return TunnelInfo{}, err
	}
	settings := config.WebhookSettings{Tunnel: provider, PublicURL: base}
	p := c.tunnelFor(settings)
	if p == nil {
		return TunnelInfo{}, fmt.Errorf("unknown tunnel %s: use tailscale or url", provider)
	}
	publicBase, err := p.Expose(ctx, port, TunnelPath)
	if err != nil {
		return TunnelInfo{}, err
	}
	err = c.store.Update(func(cfg *config.Config) error {
		if cfg.Webhooks == nil {
			cfg.Webhooks = &config.WebhookSettings{}
		}
		cfg.Webhooks.Tunnel = provider
		cfg.Webhooks.PublicURL = publicBase
		return nil
	})
	if err != nil {
		return TunnelInfo{}, fmt.Errorf("saving the tunnel: %w", err)
	}
	return TunnelInfo{
		Provider:  provider,
		PublicURL: publicBase,
		Port:      port,
		Exposed:   true,
	}, nil
}

func (c *Core) TunnelStatus(ctx context.Context) (TunnelInfo, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return TunnelInfo{}, fmt.Errorf("loading config: %w", err)
	}
	settings := config.WebhookSettings{}
	if cfg.Webhooks != nil {
		settings = *cfg.Webhooks
	}
	if settings.Tunnel == "" {
		return TunnelInfo{
			Port:    settings.Port,
			Problem: "no tunnel: run pandora tunnel use tailscale",
		}, nil
	}
	p := c.tunnelFor(settings)
	if p == nil {
		return TunnelInfo{}, fmt.Errorf("unknown tunnel %s: use tailscale or url", settings.Tunnel)
	}
	info := TunnelInfo{
		Provider:  settings.Tunnel,
		PublicURL: settings.PublicURL,
		Port:      settings.Port,
	}
	status, err := p.Check(ctx, settings.Port, TunnelPath)
	if err != nil {
		return info, err
	}
	info.Exposed = status.Exposed
	info.Problem = status.Problem
	if status.PublicBase != "" {
		info.PublicURL = status.PublicBase
	}
	return info, nil
}

func (c *Core) StopTunnel(ctx context.Context) error {
	cfg, err := c.store.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.Webhooks == nil || cfg.Webhooks.Tunnel == "" {
		return nil
	}
	settings := *cfg.Webhooks
	p := c.tunnelFor(settings)
	if p == nil {
		return fmt.Errorf("unknown tunnel %s: use tailscale or url", settings.Tunnel)
	}
	if err := p.Unexpose(ctx, TunnelPath); err != nil {
		return err
	}
	err = c.store.Update(func(cfg *config.Config) error {
		if cfg.Webhooks == nil {
			return nil
		}
		cfg.Webhooks.Tunnel = ""
		cfg.Webhooks.PublicURL = ""
		return nil
	})
	if err != nil {
		return fmt.Errorf("saving the tunnel: %w", err)
	}
	return nil
}

func (c *Core) tunnelFor(settings config.WebhookSettings) tunnel.Provider {
	if c.tunnel != nil {
		return c.tunnel(settings)
	}
	switch settings.Tunnel {
	case "tailscale":
		return tunnel.NewTailscale(tunnel.ExecRunner{})
	case "url":
		return tunnel.Static{Base: settings.PublicURL}
	default:
		return nil
	}
}
