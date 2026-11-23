package core

import (
	"context"
	"os"
	"path/filepath"

	"github.com/PierreOliveira1/pandora/internal/install"
)

type IntegrationView struct {
	Client        install.Client `json:"client"`
	Detected      bool           `json:"detected"`
	SupportsSkill bool           `json:"supports_skill"`
	Mode          string         `json:"mode"`
}

func (c *Core) Integrations(ctx context.Context) []IntegrationView {
	detected := map[install.Client]bool{}
	for _, client := range install.DetectSkillClients(c.installEnv) {
		detected[client] = true
	}
	views := make([]IntegrationView, 0, len(install.Clients()))
	for _, client := range install.Clients() {
		view := IntegrationView{Client: client, SupportsSkill: supportsSkillClient(client)}
		if !view.SupportsSkill {
			if path, err := install.ConfigPath(c.installEnv, client); err == nil {
				if _, err := os.Stat(filepath.Dir(path)); err == nil {
					detected[client] = true
				}
			}
		}
		view.Detected = detected[client]
		switch {
		case c.usesMCP(ctx, client):
			view.Mode = "mcp"
		case view.SupportsSkill && hasSkill(c.installEnv, client):
			view.Mode = "skill"
		}
		views = append(views, view)
	}
	return views
}

func hasSkill(env install.Env, client install.Client) bool {
	has, err := install.HasSkill(env, client)
	return err == nil && has
}
