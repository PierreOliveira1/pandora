package secrets

import (
	"errors"
	"net/http"
)

var ErrNotFound = errors.New("secret not found")

type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

func New(mode, filePath string) Store {
	switch mode {
	case "file":
		return NewFileStore(filePath)
	case "memory":
		return NewMemoryStore()
	default:
		return NewKeyringStore()
	}
}

func EnvKey(service, account, name string) string {
	return service + "/" + account + "/env/" + name
}

func HeaderKey(service, account, name string) string {
	return service + "/" + account + "/header/" + http.CanonicalHeaderKey(name)
}

func OAuthTokenKey(service, account string) string {
	return service + "/" + account + "/oauth/token"
}

func OAuthClientKey(service string) string {
	return service + "/_service/oauth/client"
}
