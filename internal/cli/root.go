package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/config"
	"github.com/PierreOliveira1/pandora/internal/core"
	"github.com/PierreOliveira1/pandora/internal/daemon"
	"github.com/PierreOliveira1/pandora/internal/guard"
	"github.com/PierreOliveira1/pandora/internal/install"
	"github.com/PierreOliveira1/pandora/internal/oauth"
	"github.com/PierreOliveira1/pandora/internal/registry"
	"github.com/PierreOliveira1/pandora/internal/secrets"
	"github.com/PierreOliveira1/pandora/internal/source"
	"github.com/PierreOliveira1/pandora/internal/source/command"
	"github.com/PierreOliveira1/pandora/internal/source/mcpsource"
	"github.com/PierreOliveira1/pandora/internal/source/openapi"
)

const syncAnnotation = "pandora/sync"

type deps struct {
	paths      config.Paths
	registry   *registry.Registry
	core       *core.Core
	installEnv install.Env
	verbose    *bool
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
}

type backend interface {
	Tools(ctx context.Context, service string, refresh bool) (*core.Catalog, error)
	Describe(ctx context.Context, service, tool string) (*mcp.Tool, error)
	Search(ctx context.Context, query, service string, limit int) ([]core.SearchHit, int, error)
	Call(ctx context.Context, ref, tool string, args map[string]any) (*mcp.CallToolResult, error)
	StatusAll(ctx context.Context) []core.StatusInfo
	Status(ctx context.Context, ref string) core.StatusInfo
}

type silentExitError struct {
	code int
}

func (e *silentExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

func Execute(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	paths, err := config.DefaultPaths(os.Getenv)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	store := config.NewStore(paths)
	sec := secrets.New(os.Getenv("PANDORA_SECRETS"), paths.SecretsFile())
	reg := registry.New(store, sec)
	openers := source.Openers{}
	mcpsource.Register(openers)
	openapi.Register(openers)
	command.Register(openers)
	installEnv := defaultInstallEnv()
	oauthManager := oauth.NewManager(oauth.Options{
		HTTPClient:  http.DefaultClient,
		Secrets:     sec,
		Paths:       paths,
		OpenBrowser: defaultOpenBrowser(),
		Out:         stderr,
	})
	app := core.New(core.Options{
		Store:      store,
		Secrets:    sec,
		Openers:    openers,
		Version:    appVersion(),
		InstallEnv: installEnv,
		OAuth:      oauthManager,
		CallLog:    guard.NewLog(paths.CallLog()),
	})
	defer func() {
		_ = app.Close()
	}()
	verbose := false

	root := newRoot(deps{
		paths:      paths,
		registry:   reg,
		core:       app,
		installEnv: installEnv,
		verbose:    &verbose,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
	})
	root.SetArgs(args)

	ctx = core.WithClient(ctx, guard.DetectClient(os.Getenv))
	if err := root.ExecuteContext(ctx); err != nil {
		var silent *silentExitError
		if errors.As(err, &silent) {
			return silent.code
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func newRoot(d deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "pandora",
		Short:         "One CLI for all your accounts and services",
		Version:       appVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Annotations[syncAnnotation] != "true" {
				return nil
			}
			if _, err := d.core.SyncSkills(cmd.Context()); err != nil {
				fmt.Fprintf(d.stderr, "warning: could not update the AI skill: %v\n", err)
			}
			return nil
		},
	}
	root.SetIn(d.stdin)
	root.SetOut(d.stdout)
	root.SetErr(d.stderr)
	verbose := d.verbose
	if verbose == nil {
		verbose = new(bool)
	}
	root.PersistentFlags().BoolVar(verbose, "verbose", false, "show backend details")
	root.AddCommand(
		newAdd(d),
		newList(d),
		newRemove(d),
		newRename(d),
		newDefault(d),
		newPaths(d),
		newTools(d),
		newDescribe(d),
		newSearch(d),
		newCall(d),
		newStatus(d),
		newSetDescription(d),
		newTool(d),
		newPolicy(d),
		newLog(d),
		newAuth(d),
		newLogout(d),
		newInstall(d),
		newUninstall(d),
		newSync(d),
		newDoctor(d),
		newDaemon(d),
		newServe(d),
		newExpose(d),
		newUI(d),
	)
	return root
}

func (d deps) backend(ctx context.Context) backend {
	if os.Getenv("PANDORA_NO_DAEMON") == "1" {
		d.debugf("daemon disabled; using direct connections")
		return d.core
	}
	exe, err := os.Executable()
	if err != nil {
		d.debugf("daemon unavailable (%v); using direct connections", err)
		return d.core
	}
	ensureCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	client, err := daemon.Ensure(ensureCtx, d.paths, exe, buildID())
	if err != nil {
		d.debugf("daemon unavailable (%v); using direct connections", err)
		return d.core
	}
	return daemonBackend{client: client}
}

func (d deps) debugf(format string, args ...any) {
	if d.verbose != nil && *d.verbose {
		fmt.Fprintf(d.stderr, format+"\n", args...)
	}
}

func defaultInstallEnv() install.Env {
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return install.Env{
		Home:     home,
		GOOS:     runtime.GOOS,
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
	}
}

func defaultOpenBrowser() func(string) error {
	if os.Getenv("PANDORA_NO_BROWSER") == "1" {
		return nil
	}
	return func(rawURL string) error {
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.Command("open", rawURL)
		case "linux":
			command = exec.Command("xdg-open", rawURL)
		case "windows":
			command = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
		default:
			return fmt.Errorf("opening a browser is not supported on %s", runtime.GOOS)
		}
		return command.Start()
	}
}

func buildID() string {
	exe, err := os.Executable()
	if err != nil {
		return appVersion()
	}
	info, err := os.Stat(exe)
	if err != nil {
		return appVersion()
	}
	return fmt.Sprintf("%s+%d", appVersion(), info.ModTime().UnixNano())
}
