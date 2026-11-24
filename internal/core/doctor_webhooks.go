package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/PierreOliveira1/pandora/internal/secrets"
)

func (c *Core) doctorWebhooks(ctx context.Context) []DoctorCheck {
	active, port, err := c.WebhooksActive()
	if err != nil || !active {
		return nil
	}

	checks := make([]DoctorCheck, 0, 2)
	tunnel := DoctorCheck{Name: "tunnel"}
	status, err := c.TunnelStatus(ctx)
	switch {
	case err != nil:
		tunnel.Info = err.Error()
		tunnel.Fix = "pandora tunnel use tailscale"
	case !status.Exposed:
		tunnel.Info = status.Problem
		if tunnel.Info == "" {
			tunnel.Info = "not exposed"
		}
		tunnel.Fix = "pandora tunnel use tailscale"
	default:
		tunnel.OK = true
		tunnel.Info = status.PublicURL
	}
	checks = append(checks, tunnel)

	receiver := DoctorCheck{Name: "webhook receiver"}
	fix := "pandora daemon start, or pandora autostart on to start it at login"
	if port == 0 {
		receiver.Info = "no webhook port configured"
		receiver.Fix = fix
		checks = append(checks, receiver)
		return checks
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		receiver.Info = err.Error()
		receiver.Fix = fix
	} else {
		_ = connection.Close()
		receiver.OK = true
		receiver.Info = address
	}
	checks = append(checks, receiver)
	return checks
}

func (c *Core) WebhookSigningSecret(ctx context.Context, ref string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resolved, _, _, _, err := c.resolveWebhook(ref)
	if err != nil {
		return "", err
	}
	value, err := c.secretValue(secrets.WebhookSigningKey(resolved.Service, resolved.Account))
	if err != nil {
		return "", fmt.Errorf("reading webhook signing secret for %s: %w", resolved.String(), err)
	}
	if value == "" {
		return "", errors.New("this webhook has no signing secret")
	}
	return value, nil
}
