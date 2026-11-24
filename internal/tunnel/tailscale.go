package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Tailscale struct {
	Runner     Runner
	LookPath   func(string) (string, error)
	Candidates []string
	Exists     func(string) bool
}

func NewTailscale(runner Runner) *Tailscale {
	t := &Tailscale{
		Runner:   runner,
		LookPath: exec.LookPath,
		Exists: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		},
	}
	if runtime.GOOS == "darwin" {
		t.Candidates = []string{"/Applications/Tailscale.app/Contents/MacOS/Tailscale"}
	}
	return t
}

func (t Tailscale) Name() string {
	return "tailscale"
}

func (t Tailscale) Expose(ctx context.Context, port int, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cli, err := t.cli()
	if err != nil {
		return "", err
	}
	base, err := t.publicBase(ctx, cli, path)
	if err != nil {
		return "", err
	}
	target := fmt.Sprintf("http://127.0.0.1:%d", port)
	stdout, stderr, err := t.runner().Run(ctx, cli,
		"funnel", "--bg", "--yes", "--https=443", "--set-path="+path, target)
	if link := loginLink(stdout, stderr); link != "" {
		return "", fmt.Errorf("Tailscale Funnel is not enabled for this machine: open %s, enable it, then run the command again", link)
	}
	if err != nil {
		return "", commandFailure("running tailscale funnel", stderr, err)
	}
	return base, nil
}

func (t Tailscale) Check(ctx context.Context, port int, path string) (Status, error) {
	cli, err := t.cli()
	if err != nil {
		return Status{}, err
	}
	base, err := t.publicBase(ctx, cli, path)
	if err != nil {
		return Status{}, err
	}
	stdout, stderr, err := t.runner().Run(ctx, cli, "funnel", "status", "--json")
	if err != nil {
		return Status{}, commandFailure("reading Tailscale Funnel status", stderr, err)
	}
	var config serveConfig
	if err := json.Unmarshal(stdout, &config); err != nil {
		return Status{}, fmt.Errorf("reading Tailscale Funnel status: %w", err)
	}
	target := fmt.Sprintf("http://127.0.0.1:%d", port)
	for hostport, web := range config.Web {
		if web == nil {
			continue
		}
		handler := web.Handlers[path]
		if handler != nil && handler.Proxy == target && config.AllowFunnel[hostport] {
			return Status{Exposed: true, PublicBase: base}, nil
		}
	}
	return Status{
		PublicBase: base,
		Problem:    fmt.Sprintf("Tailscale Funnel is not serving %s: run pandora tunnel use tailscale", path),
	}, nil
}

func (t Tailscale) Unexpose(ctx context.Context, path string) error {
	cli, err := t.cli()
	if err != nil {
		return err
	}
	_, stderr, err := t.runner().Run(ctx, cli, "funnel", "--https=443", "--set-path="+path, "off")
	if err != nil {
		return commandFailure("removing tailscale funnel", stderr, err)
	}
	return nil
}

func (t Tailscale) publicBase(ctx context.Context, cli, path string) (string, error) {
	host, err := t.host(ctx, cli)
	if err != nil {
		return "", err
	}
	return "https://" + host + path, nil
}

func (t Tailscale) host(ctx context.Context, cli string) (string, error) {
	stdout, stderr, err := t.runner().Run(ctx, cli, "status", "--json")
	if err != nil {
		return "", commandFailure("reading Tailscale status", stderr, err)
	}
	var status tailscaleStatus
	if err := json.Unmarshal(stdout, &status); err != nil {
		return "", fmt.Errorf("reading Tailscale status: %w", err)
	}
	if status.BackendState != "Running" {
		return "", errors.New("Tailscale is not connected: open Tailscale and sign in")
	}
	host := strings.TrimSuffix(status.Self.DNSName, ".")
	if host == "" {
		return "", errors.New("Tailscale status does not include the machine DNS name: enable MagicDNS and HTTPS in the tailnet")
	}
	return host, nil
}

func (t Tailscale) cli() (string, error) {
	lookPath := t.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if path, err := lookPath("tailscale"); err == nil && path != "" {
		return path, nil
	}
	exists := t.Exists
	if exists == nil {
		exists = func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		}
	}
	for _, candidate := range t.Candidates {
		if exists(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("Tailscale is not installed: install it from https://tailscale.com/download and sign in")
}

func (t Tailscale) runner() Runner {
	if t.Runner != nil {
		return t.Runner
	}
	return ExecRunner{}
}

type tailscaleStatus struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		DNSName string `json:"DNSName"`
	} `json:"Self"`
}

type serveConfig struct {
	Web         map[string]*webServerConfig `json:"Web"`
	AllowFunnel map[string]bool             `json:"AllowFunnel"`
}

type webServerConfig struct {
	Handlers map[string]*httpHandler `json:"Handlers"`
}

type httpHandler struct {
	Proxy string `json:"Proxy"`
}

func loginLink(stdout, stderr []byte) string {
	text := string(stdout) + "\n" + string(stderr)
	index := strings.Index(text, "https://login.tailscale.com")
	if index < 0 {
		return ""
	}
	link := text[index:]
	if end := strings.IndexAny(link, " \t\r\n\"'"); end >= 0 {
		link = link[:end]
	}
	return strings.TrimRight(link, ".,;:!?)]}\"'")
}

func commandFailure(action string, stderr []byte, err error) error {
	summary := stderrSummary(stderr)
	if summary == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, summary)
}

func stderrSummary(stderr []byte) string {
	line := strings.TrimSpace(string(stderr))
	if line == "" {
		return ""
	}
	line, _, _ = strings.Cut(line, "\n")
	const maxLength = 300
	if len(line) > maxLength {
		line = line[:maxLength-3] + "..."
	}
	return line
}
