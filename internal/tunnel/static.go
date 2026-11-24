package tunnel

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

type Static struct {
	Base string
}

func (s Static) Name() string {
	return "url"
}

func (s Static) Expose(ctx context.Context, port int, path string) (string, error) {
	return s.base()
}

func (s Static) Check(ctx context.Context, port int, path string) (Status, error) {
	base, err := s.base()
	if err != nil {
		return Status{}, err
	}
	return Status{Exposed: true, PublicBase: base}, nil
}

func (s Static) Unexpose(ctx context.Context, path string) error {
	return nil
}

func (s Static) base() (string, error) {
	base := strings.TrimRight(s.Base, "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("public URL must start with https://")
	}
	return base, nil
}
