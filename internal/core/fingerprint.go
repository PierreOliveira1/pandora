package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func accountFingerprint(svc *config.Service, acc *config.Account) string {
	if svc == nil || acc == nil {
		return ""
	}
	data, err := json.Marshal(struct {
		Kind          config.Kind
		Command       []string
		ServiceEnv    map[string]string
		ServiceURL    string
		ServiceURLs   map[string]string
		Headers       map[string]string
		Spec          string
		Auth          config.AuthMode
		OAuth         *config.OAuthConfig
		Tools         []config.CommandTool
		AccountURL    string
		AccountEnv    map[string]string
		SecretEnv     []string
		SecretHeaders []string
		CredentialsID string
	}{
		Kind:          svc.Kind,
		Command:       svc.Command,
		ServiceEnv:    svc.Env,
		ServiceURL:    svc.URL,
		ServiceURLs:   svc.URLs,
		Headers:       svc.Headers,
		Spec:          svc.Spec,
		Auth:          svc.Auth,
		OAuth:         svc.OAuth,
		Tools:         svc.Tools,
		AccountURL:    acc.URL,
		AccountEnv:    acc.Env,
		SecretEnv:     acc.SecretEnv,
		SecretHeaders: acc.SecretHeaders,
		CredentialsID: acc.CredentialsID,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
