package config

type Kind string

const (
	KindMCPStdio Kind = "mcp-stdio"
	KindMCPHTTP  Kind = "mcp-http"
	KindOpenAPI  Kind = "openapi"
	KindCommand  Kind = "command"
)

type AuthMode string

const (
	AuthNone   AuthMode = "none"
	AuthHeader AuthMode = "header"
	AuthOAuth  AuthMode = "oauth"
)

type ExposeMode string

const (
	ExposeCompact ExposeMode = "compact"
	ExposeDirect  ExposeMode = "direct"
)

type Config struct {
	Version      int                 `toml:"version" json:"version"`
	Integrations []string            `toml:"integrations,omitempty" json:"integrations,omitempty"`
	Webhooks     *WebhookSettings    `toml:"webhooks,omitempty" json:"webhooks,omitempty"`
	Readers      []EventReader       `toml:"readers,omitempty" json:"readers,omitempty"`
	Services     map[string]*Service `toml:"services" json:"services"`
}

type WebhookSettings struct {
	Port      int    `toml:"port,omitempty" json:"port,omitempty"`
	Tunnel    string `toml:"tunnel,omitempty" json:"tunnel,omitempty"`
	PublicURL string `toml:"public_url,omitempty" json:"public_url,omitempty"`
}

type EventReader struct {
	Name       string   `toml:"name" json:"name"`
	Service    string   `toml:"service,omitempty" json:"service,omitempty"`
	Account    string   `toml:"account,omitempty" json:"account,omitempty"`
	Topic      string   `toml:"topic,omitempty" json:"topic,omitempty"`
	Command    []string `toml:"command" json:"command"`
	TimeoutSec int      `toml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
}

type Service struct {
	Kind           Kind                `toml:"kind" json:"kind"`
	Description    string              `toml:"description,omitempty" json:"description,omitempty"`
	Expose         ExposeMode          `toml:"expose,omitempty" json:"expose,omitempty"`
	DefaultAccount string              `toml:"default_account,omitempty" json:"default_account,omitempty"`
	Command        []string            `toml:"command,omitempty" json:"command,omitempty"`
	Env            map[string]string   `toml:"env,omitempty" json:"env,omitempty"`
	URL            string              `toml:"url,omitempty" json:"url,omitempty"`
	Spec           string              `toml:"spec,omitempty" json:"spec,omitempty"`
	Auth           AuthMode            `toml:"auth,omitempty" json:"auth,omitempty"`
	OAuth          *OAuthConfig        `toml:"oauth,omitempty" json:"oauth,omitempty"`
	Tools          []CommandTool       `toml:"tools,omitempty" json:"tools,omitempty"`
	Accounts       map[string]*Account `toml:"accounts,omitempty" json:"accounts,omitempty"`
}

type OAuthConfig struct {
	ClientID     string   `toml:"client_id,omitempty" json:"client_id,omitempty"`
	Scopes       []string `toml:"scopes,omitempty" json:"scopes,omitempty"`
	RedirectPort int      `toml:"redirect_port,omitempty" json:"redirect_port,omitempty"`
}

type Account struct {
	Description   string            `toml:"description,omitempty" json:"description,omitempty"`
	URL           string            `toml:"url,omitempty" json:"url,omitempty"`
	Env           map[string]string `toml:"env,omitempty" json:"env,omitempty"`
	SecretEnv     []string          `toml:"secret_env,omitempty" json:"secret_env,omitempty"`
	SecretHeaders []string          `toml:"secret_headers,omitempty" json:"secret_headers,omitempty"`
	ReadOnly      bool              `toml:"readonly,omitempty" json:"readonly,omitempty"`
	Allow         []string          `toml:"allow,omitempty" json:"allow,omitempty"`
	Deny          []string          `toml:"deny,omitempty" json:"deny,omitempty"`
	CredentialsID string            `toml:"credentials_id,omitempty" json:"-"`
	Webhook       *Webhook          `toml:"webhook,omitempty" json:"webhook,omitempty"`
}

type Webhook struct {
	ID              string   `toml:"id" json:"id"`
	Style           string   `toml:"style" json:"style"`
	Challenge       string   `toml:"challenge,omitempty" json:"challenge,omitempty"`
	SignatureHeader string   `toml:"signature_header,omitempty" json:"signature_header,omitempty"`
	SignaturePrefix string   `toml:"signature_prefix,omitempty" json:"signature_prefix,omitempty"`
	Encoding        string   `toml:"encoding,omitempty" json:"encoding,omitempty"`
	TokenHeader     string   `toml:"token_header,omitempty" json:"token_header,omitempty"`
	TokenQuery      string   `toml:"token_query,omitempty" json:"token_query,omitempty"`
	TopicHeader     string   `toml:"topic_header,omitempty" json:"topic_header,omitempty"`
	TopicFields     []string `toml:"topic_fields,omitempty" json:"topic_fields,omitempty"`
	IDHeader        string   `toml:"id_header,omitempty" json:"id_header,omitempty"`
}

type CommandTool struct {
	Name        string                  `toml:"name" json:"name"`
	Description string                  `toml:"description,omitempty" json:"description,omitempty"`
	Command     []string                `toml:"command" json:"command"`
	Params      map[string]CommandParam `toml:"params,omitempty" json:"params,omitempty"`
	ReadOnly    bool                    `toml:"readonly,omitempty" json:"readonly,omitempty"`
	TimeoutSec  int                     `toml:"timeout_sec,omitempty" json:"timeout_sec,omitempty"`
}

type CommandParam struct {
	Type        string `toml:"type" json:"type"`
	Description string `toml:"description,omitempty" json:"description,omitempty"`
	Required    bool   `toml:"required,omitempty" json:"required,omitempty"`
}
