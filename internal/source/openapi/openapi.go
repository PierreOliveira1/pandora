package openapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/source"
)

type Source struct {
	baseURL      string
	headers      map[string]string
	client       *http.Client
	instructions string
	tools        []*mcp.Tool
	operations   map[string]*operation
}

type operation struct {
	method     string
	path       string
	pathParams []parameterRef
	query      []parameterRef
	headers    []parameterRef
	hasBody    bool
	form       bool
}

type parameterRef struct {
	property string
	name     string
}

func Open(ctx context.Context, spec source.Spec) (source.Source, error) {
	if spec.Svc == nil || spec.Svc.Spec == "" {
		return nil, fmt.Errorf("service %q has no OpenAPI spec", spec.Service)
	}
	client := clientWithRedirectCheck(spec.HTTPClient, len(spec.Headers) > 0)
	spec.HTTPClient = client
	doc, err := load(ctx, spec)
	if err != nil {
		return nil, err
	}
	base, err := baseURL(doc, spec.URL)
	if err != nil {
		return nil, err
	}
	tools, operations, instructions := buildTools(doc)
	return &Source{
		baseURL:      base,
		headers:      spec.Headers,
		client:       client,
		instructions: instructions,
		tools:        tools,
		operations:   operations,
	}, nil
}

func Register(openers source.Openers) {
	openers[config.KindOpenAPI] = Open
}

func (s *Source) ListTools(context.Context) ([]*mcp.Tool, error) {
	return append([]*mcp.Tool(nil), s.tools...), nil
}

func (s *Source) Instructions() string {
	return s.instructions
}

func (s *Source) Close() error {
	return nil
}

type redirectRefusedError struct {
	host string
}

func (e *redirectRefusedError) Error() string {
	return "refused to follow a redirect to " + e.host
}

func clientWithRedirectCheck(base *http.Client, hasSecrets bool) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	copied := *base
	original := base.CheckRedirect
	copied.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if hasSecrets && len(via) > 0 {
			first := via[0].URL
			if req.URL.Scheme != first.Scheme || req.URL.Host != first.Host {
				return &redirectRefusedError{host: req.URL.Host}
			}
		}
		if original != nil {
			return original(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &copied
}
