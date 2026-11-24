package registry

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/secrets"
)

type WebhookInput struct {
	Ref             Ref
	Style           string
	Challenge       string
	SignatureHeader string
	SignaturePrefix string
	Encoding        string
	TokenHeader     string
	TokenQuery      string
	TopicHeader     string
	TopicFields     []string
	IDHeader        string
	Secret          string
	Insecure        bool
}

type WebhookAdded struct {
	Ref            Ref
	Webhook        config.Webhook
	VerifyToken    string
	GeneratedToken string
}

func (r *Registry) CheckWebhook(in WebhookInput) (Ref, error) {
	if err := validateRef(in.Ref); err != nil {
		return Ref{}, err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return Ref{}, err
	}
	resolved, err := Resolve(cfg, in.Ref)
	if err != nil {
		return Ref{}, err
	}
	if cfg.Services[resolved.Service].Accounts[resolved.Account].Webhook != nil {
		return Ref{}, webhookExists(resolved)
	}

	style := webhookStyle(in.Style)
	switch style {
	case "hmac", "token", "none":
	default:
		return Ref{}, fmt.Errorf("unknown webhook style %s: use hmac, token or none", style)
	}
	if style == "none" && !in.Insecure {
		return Ref{}, errors.New("--style none accepts deliveries from anyone: add --insecure if that is what you want")
	}
	encoding := webhookEncoding(in.Encoding)
	if style == "hmac" {
		switch encoding {
		case "", "hex", "base64":
		default:
			return Ref{}, fmt.Errorf("unknown webhook encoding %s: use hex or base64", encoding)
		}
	}
	if style == "token" && in.TokenHeader == "" && in.TokenQuery == "" {
		return Ref{}, errors.New("--style token needs --token-header or --token-query")
	}
	challenge := webhookChallenge(in.Challenge)
	if challenge != "" && challenge != "hub" {
		return Ref{}, fmt.Errorf("unknown webhook challenge %s: use hub or leave it empty", challenge)
	}
	for _, name := range []string{in.SignatureHeader, in.TokenHeader, in.TopicHeader, in.IDHeader} {
		if err := validateWebhookHeader(name); err != nil {
			return Ref{}, err
		}
	}
	if err := validateTopicFields(in.TopicFields); err != nil {
		return Ref{}, err
	}
	return resolved, nil
}

func (r *Registry) AddWebhook(in WebhookInput) (WebhookAdded, error) {
	resolved, err := r.CheckWebhook(in)
	if err != nil {
		return WebhookAdded{}, err
	}
	style := webhookStyle(in.Style)
	if style == "hmac" && in.Secret == "" {
		return WebhookAdded{}, errors.New("the signing secret is required for --style hmac")
	}
	challenge := webhookChallenge(in.Challenge)
	encoding := webhookEncoding(in.Encoding)

	id, err := randomHex(16)
	if err != nil {
		return WebhookAdded{}, fmt.Errorf("generating webhook id: %w", err)
	}
	added := WebhookAdded{
		Ref: resolved,
		Webhook: config.Webhook{
			ID:              id,
			Style:           style,
			Challenge:       challenge,
			SignatureHeader: in.SignatureHeader,
			SignaturePrefix: in.SignaturePrefix,
			Encoding:        encoding,
			TokenHeader:     in.TokenHeader,
			TokenQuery:      in.TokenQuery,
			TopicHeader:     in.TopicHeader,
			TopicFields:     append([]string(nil), in.TopicFields...),
			IDHeader:        in.IDHeader,
		},
	}

	signingSecret := ""
	switch style {
	case "hmac":
		signingSecret = in.Secret
	case "token":
		signingSecret = in.Secret
		if signingSecret == "" {
			signingSecret, err = randomHex(32)
			if err != nil {
				return WebhookAdded{}, fmt.Errorf("generating webhook token: %w", err)
			}
			added.GeneratedToken = signingSecret
		}
	}
	if challenge == "hub" {
		added.VerifyToken, err = randomHex(24)
		if err != nil {
			return WebhookAdded{}, fmt.Errorf("generating webhook verify token: %w", err)
		}
	}

	written := make([]string, 0, 2)
	fail := func(err error) (WebhookAdded, error) {
		r.cleanupSecrets(written)
		return WebhookAdded{}, err
	}
	if signingSecret != "" {
		key := secrets.WebhookSigningKey(resolved.Service, resolved.Account)
		if err := r.secrets.Set(key, signingSecret); err != nil {
			return fail(fmt.Errorf("saving webhook signing secret: %w", err))
		}
		written = append(written, key)
	}
	if added.VerifyToken != "" {
		key := secrets.WebhookVerifyTokenKey(resolved.Service, resolved.Account)
		if err := r.secrets.Set(key, added.VerifyToken); err != nil {
			return fail(fmt.Errorf("saving webhook verify token: %w", err))
		}
		written = append(written, key)
	}

	err = r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[resolved.Service]
		if !ok {
			return fmt.Errorf("unknown service %q", resolved.Service)
		}
		account, ok := service.Accounts[resolved.Account]
		if !ok {
			return fmt.Errorf("unknown account %q", resolved.String())
		}
		if account.Webhook != nil {
			return webhookExists(resolved)
		}
		account.Webhook = &added.Webhook
		return nil
	})
	if err != nil {
		r.cleanupSecrets(written)
		return WebhookAdded{}, err
	}
	return added, nil
}

func (r *Registry) RemoveWebhook(ref Ref) (Ref, error) {
	if err := validateRef(ref); err != nil {
		return Ref{}, err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return Ref{}, err
	}
	resolved, err := Resolve(cfg, ref)
	if err != nil {
		return Ref{}, err
	}
	if cfg.Services[resolved.Service].Accounts[resolved.Account].Webhook == nil {
		return resolved, fmt.Errorf("%s has no webhook", resolved.String())
	}

	err = r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[resolved.Service]
		if !ok {
			return fmt.Errorf("unknown service %q", resolved.Service)
		}
		account, ok := service.Accounts[resolved.Account]
		if !ok {
			return fmt.Errorf("unknown account %q", resolved.String())
		}
		if account.Webhook == nil {
			return fmt.Errorf("%s has no webhook", resolved.String())
		}
		account.Webhook = nil
		return nil
	})
	if err != nil {
		return resolved, err
	}
	keys := []string{
		secrets.WebhookSigningKey(resolved.Service, resolved.Account),
		secrets.WebhookVerifyTokenKey(resolved.Service, resolved.Account),
	}
	if err := r.deleteSecrets(keys); err != nil {
		return resolved, fmt.Errorf("deleting webhook secrets of %s: %w", resolved.String(), err)
	}
	return resolved, nil
}

func (r *Registry) AddReader(in config.EventReader) (config.EventReader, error) {
	if err := ValidateName(in.Name); err != nil {
		return config.EventReader{}, err
	}
	if in.Name == "ai" || in.Name == "default" {
		return config.EventReader{}, fmt.Errorf("reader name %s is reserved: choose another name", in.Name)
	}
	if len(in.Command) == 0 {
		return config.EventReader{}, errors.New("the reader needs a command after --")
	}
	cfg, err := r.store.Load()
	if err != nil {
		return config.EventReader{}, err
	}
	if err := validateReaderFilters(cfg, in); err != nil {
		return config.EventReader{}, err
	}
	for _, reader := range cfg.Readers {
		if reader.Name == in.Name {
			return config.EventReader{}, readerExists(in.Name)
		}
	}
	if in.TimeoutSec == 0 {
		in.TimeoutSec = 60
	}
	if in.TimeoutSec < 1 || in.TimeoutSec > 3600 {
		return config.EventReader{}, errors.New("timeout must be between 1 and 3600 seconds")
	}
	in.Command = append([]string(nil), in.Command...)

	err = r.store.Update(func(cfg *config.Config) error {
		for _, reader := range cfg.Readers {
			if reader.Name == in.Name {
				return readerExists(in.Name)
			}
		}
		if err := validateReaderFilters(cfg, in); err != nil {
			return err
		}
		cfg.Readers = append(cfg.Readers, in)
		return nil
	})
	if err != nil {
		return config.EventReader{}, err
	}
	return in, nil
}

func (r *Registry) RemoveReader(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	return r.store.Update(func(cfg *config.Config) error {
		for i := range cfg.Readers {
			if cfg.Readers[i].Name != name {
				continue
			}
			cfg.Readers = append(cfg.Readers[:i], cfg.Readers[i+1:]...)
			return nil
		}
		return fmt.Errorf("unknown reader %s", name)
	})
}

func (r *Registry) Readers() ([]config.EventReader, error) {
	cfg, err := r.store.Load()
	if err != nil {
		return nil, err
	}
	return append([]config.EventReader(nil), cfg.Readers...), nil
}

func validateReaderFilters(cfg *config.Config, reader config.EventReader) error {
	if reader.Service == "" {
		if reader.Account != "" {
			return fmt.Errorf("reader %s: an account needs a service", reader.Name)
		}
		return nil
	}
	service, ok := cfg.Services[reader.Service]
	if !ok {
		return fmt.Errorf("unknown service %q", reader.Service)
	}
	if reader.Account == "" {
		return nil
	}
	if _, ok := service.Accounts[reader.Account]; !ok {
		return fmt.Errorf("unknown account %q", reader.Service+"/"+reader.Account)
	}
	return nil
}

func validateWebhookHeader(name string) error {
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return fmt.Errorf("invalid header name %q: use letters, digits and hyphens", name)
	}
	return nil
}

func validateTopicFields(fields []string) error {
	for _, field := range fields {
		if field == "" {
			return errors.New("invalid topic field: use dot-separated fields without empty segments")
		}
		for _, segment := range strings.Split(field, ".") {
			if segment == "" {
				return fmt.Errorf("invalid topic field %q: use dot-separated fields without empty segments", field)
			}
		}
	}
	return nil
}

func webhookStyle(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func webhookChallenge(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func webhookEncoding(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func webhookExists(ref Ref) error {
	return fmt.Errorf("%s already has a webhook: remove it with pandora webhook remove %s first", ref.String(), ref.String())
}

func readerExists(name string) error {
	return fmt.Errorf("reader %s already exists: remove it with pandora events reader remove %s first", name, name)
}

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
