package oauth

import (
	"context"
	"errors"
	"net/http"
)

type oauthTransport struct {
	base    http.RoundTripper
	token   func(ctx context.Context) (string, error)
	refresh func(ctx context.Context) (string, error)
}

func NewTransport(base http.RoundTripper, token func(ctx context.Context) (string, error), refresh func(ctx context.Context) (string, error)) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &oauthTransport{base: base, token: token, refresh: refresh}
}

func (t *oauthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token == nil {
		return nil, errors.New("oauth transport: token function is not configured")
	}
	ctx := req.Context()
	accessToken, err := t.token(ctx)
	if err != nil {
		return nil, err
	}
	first := req.Clone(ctx)
	first.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := t.base.RoundTrip(first)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized || req.Body != nil && req.Body != http.NoBody && req.GetBody == nil || t.refresh == nil {
		return resp, nil
	}
	_ = resp.Body.Close()
	accessToken, err = t.refresh(ctx)
	if err != nil {
		return nil, err
	}
	retry := req.Clone(ctx)
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		retry.Body = body
	}
	retry.Header.Set("Authorization", "Bearer "+accessToken)
	return t.base.RoundTrip(retry)
}
