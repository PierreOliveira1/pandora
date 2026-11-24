package registry

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/httptool"
	"github.com/PierreOliveira1/pandora/internal/secrets"
)

type Registry struct {
	store   *config.Store
	secrets secrets.Store
}

func New(store *config.Store, sec secrets.Store) *Registry {
	return &Registry{store: store, secrets: sec}
}

type AddInput struct {
	Service            string
	Account            string
	Kind               config.Kind
	URL                string
	URLs               map[string]string
	Command            []string
	Spec               string
	Auth               config.AuthMode
	OAuthClientID      string
	OAuthClientSecret  string
	OAuthScopes        []string
	Description        string
	ServiceDescription string
	Env                map[string]string
	SecretEnv          map[string]string
	SecretHeaders      map[string]string
}

type AccountInfo struct {
	Service     string          `json:"service"`
	Account     string          `json:"account"`
	Kind        config.Kind     `json:"kind"`
	Auth        config.AuthMode `json:"auth"`
	Default     bool            `json:"default"`
	Description string          `json:"description,omitempty"`
}

type oauthClient struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func (r *Registry) Add(in AddInput) error {
	if err := ValidateName(in.Service); err != nil {
		return err
	}
	if err := ValidateName(in.Account); err != nil {
		return err
	}

	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	if service, ok := cfg.Services[in.Service]; ok {
		if definesService(in) && !sameDefinition(service, in) {
			return differentDefinition(in.Service)
		}
		if _, ok := service.Accounts[in.Account]; ok {
			return fmt.Errorf("account %q already exists", in.Service+"/"+in.Account)
		}
		if len(in.SecretHeaders) > 0 && !supportsHeaders(service.Kind) {
			return fmt.Errorf("secret headers are not supported for %s services", service.Kind)
		}
	} else if err := validateNewService(in); err != nil {
		return err
	}

	headers := make(map[string]string, len(in.SecretHeaders))
	for name, value := range in.SecretHeaders {
		headers[http.CanonicalHeaderKey(name)] = value
	}
	secretEnvNames := sortedNames(in.SecretEnv)
	secretHeaderNames := sortedNames(headers)

	written, err := r.writeAddSecrets(in, headers)
	if err != nil {
		return err
	}

	err = r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[in.Service]
		if ok {
			if definesService(in) && !sameDefinition(service, in) {
				return differentDefinition(in.Service)
			}
		} else {
			if err := validateNewService(in); err != nil {
				return err
			}
			service = newService(in)
			cfg.Services[in.Service] = service
		}
		if service.Accounts == nil {
			service.Accounts = map[string]*config.Account{}
		}
		if _, ok := service.Accounts[in.Account]; ok {
			return fmt.Errorf("account %q already exists", in.Service+"/"+in.Account)
		}
		service.Accounts[in.Account] = &config.Account{
			Description:   in.Description,
			Env:           in.Env,
			SecretEnv:     secretEnvNames,
			SecretHeaders: secretHeaderNames,
			CredentialsID: newCredentialsID(),
		}
		if service.DefaultAccount == "" {
			service.DefaultAccount = in.Account
		}
		return nil
	})
	if err != nil {
		r.cleanupSecrets(written)
		return err
	}
	return nil
}

func definesService(in AddInput) bool {
	return in.Kind != "" || in.URL != "" || len(in.URLs) > 0 || len(in.Command) > 0 || in.Spec != "" || in.Auth != "" ||
		in.OAuthClientID != "" || in.OAuthClientSecret != "" || len(in.OAuthScopes) > 0 || in.ServiceDescription != ""
}

func sameDefinition(service *config.Service, in AddInput) bool {
	if in.Kind != "" && in.Kind != service.Kind {
		return false
	}
	if in.URL != "" && in.URL != service.URL {
		return false
	}
	if len(in.URLs) > 0 && !sameURLs(service.URLs, in.URLs) {
		return false
	}
	if len(in.Command) > 0 && !slices.Equal(in.Command, service.Command) {
		return false
	}
	if in.Spec != "" && in.Spec != service.Spec {
		return false
	}
	if in.Auth != "" && in.Auth != service.Auth {
		return false
	}
	if in.ServiceDescription != "" && in.ServiceDescription != service.Description {
		return false
	}
	return in.OAuthClientID == "" && in.OAuthClientSecret == "" && len(in.OAuthScopes) == 0
}

func differentDefinition(service string) error {
	return fmt.Errorf("service %q already exists with a different definition; to add another account run pandora add %s --as <name> with only the account's credentials", service, service)
}

func supportsHeaders(kind config.Kind) bool {
	return kind == config.KindMCPHTTP || kind == config.KindOpenAPI || kind == config.KindHTTP
}

func supportsOAuth(kind config.Kind) bool {
	return kind == config.KindMCPHTTP || kind == config.KindOpenAPI
}

func validateNewService(in AddInput) error {
	switch in.Kind {
	case config.KindMCPHTTP:
		parsed, err := url.Parse(in.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("service %q needs a valid http or https URL", in.Service)
		}
	case config.KindMCPStdio:
		if len(in.Command) == 0 {
			return fmt.Errorf("service %q needs a command for mcp-stdio", in.Service)
		}
	case config.KindOpenAPI:
		if in.Spec == "" {
			return fmt.Errorf("service %q needs an openapi spec", in.Service)
		}
	case config.KindCommand:
	case config.KindHTTP:
		if len(in.URLs) == 0 {
			return fmt.Errorf("service %s needs at least one URL: use --http name=URL", in.Service)
		}
		for _, name := range sortedKeys(in.URLs) {
			if err := ValidateName(name); err != nil {
				return err
			}
			if err := httptool.ValidateURL(in.URLs[name]); err != nil {
				return err
			}
		}
	case "":
		return fmt.Errorf("service %q needs a kind: use mcp-stdio, mcp-http, openapi, command or http", in.Service)
	default:
		return fmt.Errorf("unknown service kind %q", in.Kind)
	}

	if len(in.SecretHeaders) > 0 && !supportsHeaders(in.Kind) {
		return fmt.Errorf("secret headers are not supported for %s services", in.Kind)
	}
	if (in.OAuthClientID != "" || in.OAuthClientSecret != "" || len(in.OAuthScopes) > 0 || in.Auth == config.AuthOAuth) &&
		!supportsOAuth(in.Kind) {
		return fmt.Errorf("oauth is not supported for %s services", in.Kind)
	}
	return nil
}

func newService(in AddInput) *config.Service {
	service := &config.Service{
		Kind:        in.Kind,
		Description: in.ServiceDescription,
		Command:     in.Command,
		URL:         in.URL,
		URLs:        normalizedURLs(in.URLs),
		Spec:        in.Spec,
		Accounts:    map[string]*config.Account{},
	}
	switch {
	case len(in.SecretHeaders) > 0:
		service.Auth = config.AuthHeader
	case in.OAuthClientID != "" || in.Auth == config.AuthOAuth:
		service.Auth = config.AuthOAuth
		service.OAuth = &config.OAuthConfig{ClientID: in.OAuthClientID, Scopes: in.OAuthScopes}
	case in.Auth != "":
		service.Auth = in.Auth
	default:
		service.Auth = config.AuthNone
	}
	return service
}

func (r *Registry) writeAddSecrets(in AddInput, headers map[string]string) ([]string, error) {
	written := make([]string, 0, len(in.SecretEnv)+len(headers)+1)
	fail := func(err error) ([]string, error) {
		r.cleanupSecrets(written)
		return nil, err
	}

	for _, name := range sortedNames(in.SecretEnv) {
		key := secrets.EnvKey(in.Service, in.Account, name)
		if err := r.secrets.Set(key, in.SecretEnv[name]); err != nil {
			return fail(fmt.Errorf("saving secret for %s: %w", name, err))
		}
		written = append(written, key)
	}
	for _, name := range sortedNames(headers) {
		key := secrets.HeaderKey(in.Service, in.Account, name)
		if err := r.secrets.Set(key, headers[name]); err != nil {
			return fail(fmt.Errorf("saving secret for header %s: %w", name, err))
		}
		written = append(written, key)
	}
	if in.OAuthClientSecret != "" {
		payload, err := json.Marshal(oauthClient{ClientID: in.OAuthClientID, ClientSecret: in.OAuthClientSecret})
		if err != nil {
			return fail(fmt.Errorf("encoding oauth client: %w", err))
		}
		key := secrets.OAuthClientKey(in.Service)
		if err := r.secrets.Set(key, string(payload)); err != nil {
			return fail(fmt.Errorf("saving oauth client secret: %w", err))
		}
		written = append(written, key)
	}
	return written, nil
}

func (r *Registry) List() ([]AccountInfo, []string, error) {
	cfg, err := r.store.Load()
	if err != nil {
		return nil, nil, err
	}

	accounts := make([]AccountInfo, 0)
	emptyServices := make([]string, 0)
	for _, serviceName := range sortedKeys(cfg.Services) {
		service := cfg.Services[serviceName]
		if len(service.Accounts) == 0 {
			emptyServices = append(emptyServices, serviceName)
			continue
		}
		for _, accountName := range sortedKeys(service.Accounts) {
			account := service.Accounts[accountName]
			accounts = append(accounts, AccountInfo{
				Service:     serviceName,
				Account:     accountName,
				Kind:        service.Kind,
				Auth:        service.Auth,
				Default:     service.DefaultAccount == accountName,
				Description: account.Description,
			})
		}
	}
	return accounts, emptyServices, nil
}

func (r *Registry) RemoveAccount(ref Ref) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	resolved, err := Resolve(cfg, ref)
	if err != nil {
		return err
	}
	account := cfg.Services[resolved.Service].Accounts[resolved.Account]
	keys := accountSecretKeys(resolved.Service, resolved.Account, account)

	err = r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[resolved.Service]
		if !ok {
			return fmt.Errorf("unknown service %q", resolved.Service)
		}
		if _, ok := service.Accounts[resolved.Account]; !ok {
			return fmt.Errorf("unknown account %q", resolved.String())
		}
		delete(service.Accounts, resolved.Account)
		if service.DefaultAccount == resolved.Account {
			service.DefaultAccount = firstAccount(service.Accounts)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := r.deleteSecrets(keys); err != nil {
		return fmt.Errorf("deleting secrets of %s: %w", resolved.String(), err)
	}
	return nil
}

func (r *Registry) RemoveService(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	service, ok := cfg.Services[name]
	if !ok {
		return fmt.Errorf("unknown service %q", name)
	}
	keys := []string{secrets.OAuthClientKey(name)}
	for accountName, account := range service.Accounts {
		keys = append(keys, accountSecretKeys(name, accountName, account)...)
	}

	err = r.store.Update(func(cfg *config.Config) error {
		if _, ok := cfg.Services[name]; !ok {
			return fmt.Errorf("unknown service %q", name)
		}
		delete(cfg.Services, name)
		return nil
	})
	if err != nil {
		return err
	}
	if err := r.deleteSecrets(keys); err != nil {
		return fmt.Errorf("deleting secrets of service %q: %w", name, err)
	}
	return nil
}

func (r *Registry) RenameAccount(ref Ref, newAccount string) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	if err := ValidateName(newAccount); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	resolved, err := Resolve(cfg, ref)
	if err != nil {
		return err
	}
	service := cfg.Services[resolved.Service]
	if _, ok := service.Accounts[newAccount]; ok {
		return fmt.Errorf("account %q already exists", resolved.Service+"/"+newAccount)
	}
	account := service.Accounts[resolved.Account]

	if err := r.moveAccountSecrets(resolved.Service, resolved.Account, newAccount, account); err != nil {
		return err
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
		if _, ok := service.Accounts[newAccount]; ok {
			return fmt.Errorf("account %q already exists", resolved.Service+"/"+newAccount)
		}
		delete(service.Accounts, resolved.Account)
		service.Accounts[newAccount] = account
		if service.DefaultAccount == resolved.Account {
			service.DefaultAccount = newAccount
		}
		for i := range cfg.Readers {
			if cfg.Readers[i].Service == resolved.Service && cfg.Readers[i].Account == resolved.Account {
				cfg.Readers[i].Account = newAccount
			}
		}
		return nil
	})
	if err != nil {
		_ = r.moveAccountSecrets(resolved.Service, newAccount, resolved.Account, account)
		return err
	}
	return nil
}

func (r *Registry) RenameService(oldName, newName string) error {
	if err := ValidateName(oldName); err != nil {
		return err
	}
	if err := ValidateName(newName); err != nil {
		return err
	}
	cfg, err := r.store.Load()
	if err != nil {
		return err
	}
	service, ok := cfg.Services[oldName]
	if !ok {
		return fmt.Errorf("unknown service %q", oldName)
	}
	if _, ok := cfg.Services[newName]; ok {
		return fmt.Errorf("service %q already exists", newName)
	}

	if err := r.moveServiceSecrets(oldName, newName, service); err != nil {
		return err
	}
	err = r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[oldName]
		if !ok {
			return fmt.Errorf("unknown service %q", oldName)
		}
		if _, ok := cfg.Services[newName]; ok {
			return fmt.Errorf("service %q already exists", newName)
		}
		delete(cfg.Services, oldName)
		cfg.Services[newName] = service
		for i := range cfg.Readers {
			if cfg.Readers[i].Service == oldName {
				cfg.Readers[i].Service = newName
			}
		}
		return nil
	})
	if err != nil {
		_ = r.moveServiceSecrets(newName, oldName, service)
		return err
	}
	return nil
}

func (r *Registry) SetDefault(ref Ref) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	if ref.Account == "" {
		cfg, err := r.store.Load()
		if err != nil {
			return err
		}
		resolved, err := Resolve(cfg, ref)
		if err != nil {
			return err
		}
		ref = resolved
	}

	return r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[ref.Service]
		if !ok {
			return fmt.Errorf("unknown service %q", ref.Service)
		}
		if _, ok := service.Accounts[ref.Account]; !ok {
			return fmt.Errorf("unknown account %q", ref.String())
		}
		service.DefaultAccount = ref.Account
		return nil
	})
}

func (r *Registry) SetDescription(ref Ref, text string) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	return r.store.Update(func(cfg *config.Config) error {
		service, ok := cfg.Services[ref.Service]
		if !ok {
			return fmt.Errorf("unknown service %q", ref.Service)
		}
		if ref.Account == "" {
			service.Description = text
			return nil
		}
		account, ok := service.Accounts[ref.Account]
		if !ok {
			return fmt.Errorf("unknown account %q", ref.String())
		}
		account.Description = text
		return nil
	})
}

func (r *Registry) SetAuth(service string, mode config.AuthMode) error {
	if err := ValidateName(service); err != nil {
		return err
	}
	return r.store.Update(func(cfg *config.Config) error {
		value, ok := cfg.Services[service]
		if !ok {
			return fmt.Errorf("unknown service %q", service)
		}
		value.Auth = mode
		if mode == config.AuthOAuth && value.OAuth == nil {
			value.OAuth = &config.OAuthConfig{}
		}
		return nil
	})
}

func validateRef(ref Ref) error {
	if err := ValidateName(ref.Service); err != nil {
		return err
	}
	if ref.Account != "" {
		return ValidateName(ref.Account)
	}
	return nil
}

func sortedNames(values map[string]string) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstAccount(accounts map[string]*config.Account) string {
	names := make([]string, 0, len(accounts))
	for name := range accounts {
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[0]
}

func accountSecretKeys(service, account string, acct *config.Account) []string {
	keys := make([]string, 0, len(acct.SecretEnv)+len(acct.SecretHeaders)+3)
	for _, name := range acct.SecretEnv {
		keys = append(keys, secrets.EnvKey(service, account, name))
	}
	for _, name := range acct.SecretHeaders {
		keys = append(keys, secrets.HeaderKey(service, account, name))
	}
	keys = append(keys, secrets.OAuthTokenKey(service, account))
	keys = append(keys, secrets.WebhookSigningKey(service, account))
	keys = append(keys, secrets.WebhookVerifyTokenKey(service, account))
	return keys
}

func (r *Registry) moveAccountSecrets(service, oldAccount, newAccount string, acct *config.Account) error {
	pairs := make([][2]string, 0, len(acct.SecretEnv)+len(acct.SecretHeaders)+3)
	for _, name := range acct.SecretEnv {
		pairs = append(pairs, [2]string{
			secrets.EnvKey(service, oldAccount, name),
			secrets.EnvKey(service, newAccount, name),
		})
	}
	for _, name := range acct.SecretHeaders {
		pairs = append(pairs, [2]string{
			secrets.HeaderKey(service, oldAccount, name),
			secrets.HeaderKey(service, newAccount, name),
		})
	}
	pairs = append(pairs, [2]string{
		secrets.OAuthTokenKey(service, oldAccount),
		secrets.OAuthTokenKey(service, newAccount),
	})
	pairs = append(pairs, [2]string{
		secrets.WebhookSigningKey(service, oldAccount),
		secrets.WebhookSigningKey(service, newAccount),
	})
	pairs = append(pairs, [2]string{
		secrets.WebhookVerifyTokenKey(service, oldAccount),
		secrets.WebhookVerifyTokenKey(service, newAccount),
	})
	if err := r.movePairs(pairs); err != nil {
		return fmt.Errorf("moving secrets of %s/%s: %w", service, oldAccount, err)
	}
	return nil
}

func (r *Registry) moveServiceSecrets(oldName, newName string, service *config.Service) error {
	pairs := make([][2]string, 0, len(service.Accounts)*5+1)
	for accountName, account := range service.Accounts {
		for _, name := range account.SecretEnv {
			pairs = append(pairs, [2]string{
				secrets.EnvKey(oldName, accountName, name),
				secrets.EnvKey(newName, accountName, name),
			})
		}
		for _, name := range account.SecretHeaders {
			pairs = append(pairs, [2]string{
				secrets.HeaderKey(oldName, accountName, name),
				secrets.HeaderKey(newName, accountName, name),
			})
		}
		pairs = append(pairs, [2]string{
			secrets.OAuthTokenKey(oldName, accountName),
			secrets.OAuthTokenKey(newName, accountName),
		})
		pairs = append(pairs, [2]string{
			secrets.WebhookSigningKey(oldName, accountName),
			secrets.WebhookSigningKey(newName, accountName),
		})
		pairs = append(pairs, [2]string{
			secrets.WebhookVerifyTokenKey(oldName, accountName),
			secrets.WebhookVerifyTokenKey(newName, accountName),
		})
	}
	pairs = append(pairs, [2]string{
		secrets.OAuthClientKey(oldName),
		secrets.OAuthClientKey(newName),
	})
	if err := r.movePairs(pairs); err != nil {
		return fmt.Errorf("moving secrets of service %s: %w", oldName, err)
	}
	return nil
}

func (r *Registry) movePairs(pairs [][2]string) error {
	for _, pair := range pairs {
		if err := r.moveSecret(pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) moveSecret(oldKey, newKey string) error {
	value, err := r.secrets.Get(oldKey)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := r.secrets.Set(newKey, value); err != nil {
		return err
	}
	return r.secrets.Delete(oldKey)
}

func (r *Registry) deleteSecrets(keys []string) error {
	for _, key := range keys {
		if err := r.secrets.Delete(key); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (r *Registry) cleanupSecrets(keys []string) {
	for _, key := range keys {
		_ = r.secrets.Delete(key)
	}
}

func newCredentialsID() string {
	var value [8]byte
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}
