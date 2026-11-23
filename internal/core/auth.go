package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/oauth"
	"github.com/PierreOliveira1/pandora/internal/registry"
)

func (c *Core) Login(ctx context.Context, ref string) error {
	cfg, err := c.store.Load()
	if err != nil {
		return err
	}
	parsed, err := registry.ParseRef(ref)
	if err != nil {
		return err
	}
	resolved, err := registry.Resolve(cfg, parsed)
	if err != nil {
		return err
	}
	service := cfg.Services[resolved.Service]
	if service.Kind != config.KindMCPHTTP {
		return fmt.Errorf("%s does not use OAuth", resolved.Service)
	}

	switch service.Auth {
	case config.AuthHeader:
		return fmt.Errorf("%s uses a static header: remove the account and add it again with a new --header", resolved.Service)
	case config.AuthNone, "":
		if err := registry.New(c.store, c.secrets).SetAuth(resolved.Service, config.AuthOAuth); err != nil {
			return err
		}
		cfg, err = c.store.Load()
		if err != nil {
			return err
		}
		service = cfg.Services[resolved.Service]
	case config.AuthOAuth:
	default:
		return fmt.Errorf("service %q uses unsupported authentication mode %q", resolved.Service, service.Auth)
	}

	if c.oauth == nil {
		return errors.New("OAuth is not configured")
	}
	account, ok := service.Accounts[resolved.Account]
	if !ok {
		return fmt.Errorf("unknown account %q", resolved.String())
	}
	url := account.URL
	if url == "" {
		url = service.URL
	}
	var clientID string
	var scopes []string
	var redirectPort int
	if service.OAuth != nil {
		clientID = service.OAuth.ClientID
		scopes = service.OAuth.Scopes
		redirectPort = service.OAuth.RedirectPort
	}

	if err := c.oauth.Login(ctx, oauth.LoginParams{
		Service:      resolved.Service,
		Account:      resolved.Account,
		MCPURL:       url,
		ClientID:     clientID,
		Scopes:       scopes,
		RedirectPort: redirectPort,
	}); err != nil {
		return err
	}

	c.drop(resolved.String())
	_, _ = c.Tools(ctx, resolved.Service, true)
	return nil
}

func (c *Core) Logout(ctx context.Context, ref string) error {
	cfg, err := c.store.Load()
	if err != nil {
		return err
	}
	parsed, err := registry.ParseRef(ref)
	if err != nil {
		return err
	}
	resolved, err := registry.Resolve(cfg, parsed)
	if err != nil {
		return err
	}
	if c.oauth == nil {
		return errors.New("OAuth is not configured")
	}
	err = c.oauth.Logout(resolved.Service, resolved.Account)
	c.drop(resolved.String())
	return err
}

func (c *Core) DetectAuth(ctx context.Context, ref string) (StatusInfo, error) {
	info := c.Status(ctx, ref)
	if !info.NeedsLogin {
		return info, nil
	}

	cfg, err := c.store.Load()
	if err != nil {
		return info, err
	}
	parsed, err := registry.ParseRef(ref)
	if err != nil {
		return info, err
	}
	resolved, err := registry.Resolve(cfg, parsed)
	if err != nil {
		return info, err
	}
	service := cfg.Services[resolved.Service]
	if service.Kind == config.KindMCPHTTP && (service.Auth == config.AuthNone || service.Auth == "") {
		if err := registry.New(c.store, c.secrets).SetAuth(resolved.Service, config.AuthOAuth); err != nil {
			return info, err
		}
	}
	return info, nil
}
