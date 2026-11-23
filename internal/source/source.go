package source

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
)

type Source interface {
	ListTools(ctx context.Context) ([]*mcp.Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error)
	Instructions() string
	Close() error
}

type Spec struct {
	Service    string
	Account    string
	Kind       config.Kind
	Svc        *config.Service
	Acc        *config.Account
	URL        string
	Env        map[string]string
	Headers    map[string]string
	Token      func(ctx context.Context) (string, error)
	Refresh    func(ctx context.Context) (string, error)
	HTTPClient *http.Client
	LogPath    string
}

type Opener func(ctx context.Context, spec Spec) (Source, error)

type Openers map[config.Kind]Opener

type AuthRequiredError struct {
	Ref string
}

func (e *AuthRequiredError) Error() string {
	return fmt.Sprintf("account %s needs login: run pandora auth %s", e.Ref, e.Ref)
}
