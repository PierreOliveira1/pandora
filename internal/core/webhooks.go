package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/webhook"
)

type WebhookInfo struct {
	Ref         string     `json:"ref"`
	Style       string     `json:"style"`
	Challenge   string     `json:"challenge,omitempty"`
	URL         string     `json:"url,omitempty"`
	LocalURL    string     `json:"local_url,omitempty"`
	LastEventAt *time.Time `json:"last_event_at,omitempty"`
}

type WebhookTestResult struct {
	URL       string
	Handshake bool
}

func (c *Core) EnsureWebhookPort(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	port := 0
	err := c.store.Update(func(cfg *config.Config) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if cfg.Webhooks == nil {
			cfg.Webhooks = &config.WebhookSettings{}
		}
		if cfg.Webhooks.Port != 0 {
			port = cfg.Webhooks.Port
			return nil
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("finding a free webhook port: %w", err)
		}
		address, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			_ = listener.Close()
			return errors.New("finding a free webhook port: unexpected address type")
		}
		port = address.Port
		if err := listener.Close(); err != nil {
			return fmt.Errorf("closing the webhook port listener: %w", err)
		}
		cfg.Webhooks.Port = port
		return nil
	})
	if err != nil {
		return 0, err
	}
	return port, nil
}

func (c *Core) WebhooksActive() (bool, int, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return false, 0, fmt.Errorf("loading config: %w", err)
	}
	port := 0
	if cfg.Webhooks != nil {
		port = cfg.Webhooks.Port
	}
	for _, service := range cfg.Services {
		if service == nil {
			continue
		}
		for _, account := range service.Accounts {
			if account != nil && account.Webhook != nil {
				return true, port, nil
			}
		}
	}
	return false, port, nil
}

func (c *Core) WebhookLookup(ctx context.Context, service, account, id string) (webhook.Hook, webhook.Secrets, bool, error) {
	if err := ctx.Err(); err != nil {
		return webhook.Hook{}, webhook.Secrets{}, false, err
	}
	cfg, err := c.store.Load()
	if err != nil {
		return webhook.Hook{}, webhook.Secrets{}, false, fmt.Errorf("loading config: %w", err)
	}
	svc, ok := cfg.Services[service]
	if !ok || svc == nil {
		return webhook.Hook{}, webhook.Secrets{}, false, nil
	}
	acc, ok := svc.Accounts[account]
	if !ok || acc == nil || acc.Webhook == nil {
		return webhook.Hook{}, webhook.Secrets{}, false, nil
	}
	hookConfig := acc.Webhook
	if subtle.ConstantTimeCompare([]byte(hookConfig.ID), []byte(id)) != 1 {
		return webhook.Hook{}, webhook.Secrets{}, false, nil
	}
	secretsValue, err := c.webhookSecrets(service, account)
	if err != nil {
		return webhook.Hook{}, webhook.Secrets{}, false, err
	}
	hook := webhook.Hook{
		Service:     service,
		Account:     account,
		ID:          hookConfig.ID,
		Verify:      webhookVerification(hookConfig),
		TopicHeader: hookConfig.TopicHeader,
		TopicFields: append([]string(nil), hookConfig.TopicFields...),
		IDHeader:    hookConfig.IDHeader,
	}
	return hook, secretsValue, true, nil
}

func (c *Core) Webhooks(ctx context.Context) ([]WebhookInfo, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	last, err := c.events.LastReceived(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the last webhook events: %w", err)
	}
	infos := make([]WebhookInfo, 0)
	for serviceName, service := range cfg.Services {
		if service == nil {
			continue
		}
		for accountName, account := range service.Accounts {
			if account == nil || account.Webhook == nil {
				continue
			}
			ref := registry.Ref{Service: serviceName, Account: accountName}
			infos = append(infos, webhookInfo(cfg, ref, account.Webhook, last))
		}
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Ref < infos[j].Ref
	})
	return infos, nil
}

func (c *Core) WebhookVerifyToken(ctx context.Context, ref string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resolved, _, _, hook, err := c.resolveWebhook(ref)
	if err != nil {
		return "", err
	}
	if hook.Challenge != "hub" {
		return "", nil
	}
	token, err := c.secretValue(secrets.WebhookVerifyTokenKey(resolved.Service, resolved.Account))
	if err != nil {
		return "", fmt.Errorf("reading webhook verify token for %s: %w", resolved.String(), err)
	}
	return token, nil
}

func (c *Core) TestWebhook(ctx context.Context, ref string) (WebhookTestResult, error) {
	resolved, cfg, _, hook, err := c.resolveWebhook(ref)
	if err != nil {
		return WebhookTestResult{}, err
	}
	target := webhookTarget(cfg, resolved, hook.ID)
	if target == "" {
		return WebhookTestResult{}, errors.New("the webhook receiver has no address yet: run pandora tunnel use tailscale")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := WebhookTestResult{URL: target}

	if hook.Challenge == "hub" {
		token, err := c.WebhookVerifyToken(ctx, resolved.String())
		if err != nil {
			return result, err
		}
		challenge, err := newWebhookChallenge()
		if err != nil {
			return result, err
		}
		verificationURL, err := url.Parse(target)
		if err != nil {
			return result, fmt.Errorf("building the webhook verification URL: %w", err)
		}
		query := verificationURL.Query()
		query.Set("hub.mode", "subscribe")
		query.Set("hub.challenge", challenge)
		query.Set("hub.verify_token", token)
		verificationURL.RawQuery = query.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, verificationURL.String(), nil)
		if err != nil {
			return result, errors.New("could not build the webhook verification request: check the public URL and the tunnel")
		}
		response, err := c.webhookRequest(request)
		if err != nil {
			return result, err
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return result, fmt.Errorf("reading the webhook verification response: %w", readErr)
		}
		if response.StatusCode != http.StatusOK || string(body) != challenge {
			return result, fmt.Errorf("the receiver did not answer the verification at %s: got %s", target, response.Status)
		}
		result.Handshake = true
	}

	signing, err := c.secretValue(secrets.WebhookSigningKey(resolved.Service, resolved.Account))
	if err != nil {
		return result, fmt.Errorf("reading webhook signing secret for %s: %w", resolved.String(), err)
	}
	body, err := json.Marshal(struct {
		PandoraTest bool   `json:"pandora_test"`
		SentAt      string `json:"sent_at"`
	}{
		PandoraTest: true,
		SentAt:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return result, fmt.Errorf("encoding the webhook test: %w", err)
	}

	postURL := target
	header, signature := webhook.Sign(webhookVerification(hook), signing, body)
	if header == "" && strings.EqualFold(hook.Style, "token") && hook.TokenQuery != "" && signature != "" {
		parsed, err := url.Parse(postURL)
		if err != nil {
			return result, fmt.Errorf("building the webhook test URL: %w", err)
		}
		query := parsed.Query()
		query.Set(hook.TokenQuery, signature)
		parsed.RawQuery = query.Encode()
		postURL = parsed.String()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		return result, errors.New("could not build the webhook test request: check the public URL and the tunnel")
	}
	request.Header.Set("Content-Type", "application/json")
	if header != "" {
		request.Header.Set(header, signature)
	}
	response, err := c.webhookRequest(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("the receiver answered %s at %s", response.Status, target)
	}
	return result, nil
}

func (c *Core) resolveWebhook(ref string) (registry.Ref, *config.Config, *config.Account, *config.Webhook, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return registry.Ref{}, nil, nil, nil, fmt.Errorf("loading config: %w", err)
	}
	parsed, err := registry.ParseRef(ref)
	if err != nil {
		return registry.Ref{}, nil, nil, nil, err
	}
	resolved, err := registry.Resolve(cfg, parsed)
	if err != nil {
		return registry.Ref{}, nil, nil, nil, err
	}
	account := cfg.Services[resolved.Service].Accounts[resolved.Account]
	if account == nil || account.Webhook == nil {
		message := fmt.Sprintf("%s has no webhook: add one with pandora webhook add %s", resolved.String(), resolved.String())
		return resolved, cfg, nil, nil, errors.New(message)
	}
	return resolved, cfg, account, account.Webhook, nil
}

func webhookTarget(cfg *config.Config, ref registry.Ref, id string) string {
	if cfg.Webhooks == nil {
		return ""
	}
	if cfg.Webhooks.PublicURL != "" {
		return webhookPublicURL(cfg.Webhooks.PublicURL, ref.Service, ref.Account, id)
	}
	if cfg.Webhooks.Port > 0 {
		return webhookLocalURL(cfg.Webhooks.Port, ref.Service, ref.Account, id)
	}
	return ""
}

func webhookPublicURL(base, service, account, id string) string {
	return strings.TrimRight(base, "/") + webhookPath(service, account, id)
}

func webhookLocalURL(port int, service, account, id string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, webhookPath(service, account, id))
}

func webhookPath(service, account, id string) string {
	return "/hooks/" + service + "/" + account + "/" + id
}

func webhookVerification(hook *config.Webhook) webhook.Verification {
	return webhook.Verification{
		Style:           hook.Style,
		SignatureHeader: hook.SignatureHeader,
		SignaturePrefix: hook.SignaturePrefix,
		Encoding:        hook.Encoding,
		TokenHeader:     hook.TokenHeader,
		TokenQuery:      hook.TokenQuery,
		Challenge:       hook.Challenge,
	}
}

func (c *Core) webhookSecrets(service, account string) (webhook.Secrets, error) {
	signing, err := c.secretValue(secrets.WebhookSigningKey(service, account))
	if err != nil {
		return webhook.Secrets{}, fmt.Errorf("reading webhook signing secret for %s/%s: %w", service, account, err)
	}
	verifyToken, err := c.secretValue(secrets.WebhookVerifyTokenKey(service, account))
	if err != nil {
		return webhook.Secrets{}, fmt.Errorf("reading webhook verify token for %s/%s: %w", service, account, err)
	}
	return webhook.Secrets{Signing: signing, VerifyToken: verifyToken}, nil
}

func (c *Core) secretValue(key string) (string, error) {
	value, err := c.secrets.Get(key)
	if errors.Is(err, secrets.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func newWebhookChallenge() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generating a webhook challenge: %w", err)
	}
	return hex.EncodeToString(random), nil
}

func (c *Core) webhookRequest(request *http.Request) (*http.Response, error) {
	response, err := c.httpClient.Do(request)
	if err == nil {
		return response, nil
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return nil, errors.New("the webhook receiver is not running: run pandora daemon start")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("the webhook receiver did not answer before the timeout: %w", context.DeadlineExceeded)
	}
	if errors.Is(err, context.Canceled) {
		return nil, fmt.Errorf("the webhook test was canceled: %w", context.Canceled)
	}
	return nil, errors.New("could not reach the webhook receiver: check that it is running and the URL is correct")
}
