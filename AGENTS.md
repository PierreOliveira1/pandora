# AGENTS.md

Guidance for AI coding agents (and humans) working on Pandora. Read this file before planning or changing anything. If a task seems to require breaking a rule, stop and ask.

## What Pandora is

Pandora is a local CLI, written in Go, that holds every integration a person uses and exposes all of them to any AI. The AI uses the `pandora` command guided by a generated skill; clients without skills get one MCP server instead. It is built around two ideas:

1. **Generic.** The core knows no specific service. Anything enters through a *source kind*: a remote MCP server (`mcp-http`), a local MCP server started as a command (`mcp-stdio`), an HTTP API described by OpenAPI (`openapi`) or shell commands declared as tools (`command`).
2. **Many accounts per service.** A *service* (`todoist`, `meta`, `github`) defines how to talk to something. An *account* is one set of credentials for that service, with an alias. The same service can have many accounts: `todoist/personal`, `todoist/work`.

Three entry points use the same core and never duplicate rules:

- the CLI (`pandora add`, `pandora call`, ...), which the AI also uses, guided by the generated skill (`internal/skill`, written into each client by `pandora install`);
- the MCP gateway (`pandora serve`), for clients without skills or installed with `pandora install --mcp`;
- the local web UI (`pandora ui`).

Reading commands (`call`, `tools`, `describe`, `search`, `status`) go through a background daemon (`internal/daemon`) that keeps upstream connections open and is shared by every AI client. It starts on demand, stops after being idle and is bypassed with `PANDORA_NO_DAEMON=1`. Commands that change the config always run in the calling process; the daemon re-reads the config on every request.

## Vocabulary

| Term | Meaning |
| --- | --- |
| service | A named definition of how to reach something: kind, URL or command, auth mode. Name rule: `^[a-z0-9][a-z0-9-]{0,31}$`. |
| account | One set of credentials of a service, with an alias. Same name rule as services. |
| account ref | `service/account`, or just `service` meaning its default account. |
| source | A live connection to one account, able to list and call tools. |
| kind | `mcp-stdio`, `mcp-http`, `openapi`, `command`. |
| expose mode | How a service's tools reach the AI: `compact` (default, 4 meta tools for everything) or `direct` (each tool once, with an `account` argument). |

## Folder layout

```
cmd/pandora/        # main.go: wiring only
internal/
  config/           # paths, config types, TOML load/save, file locks
  secrets/          # secret store interface + keychain, file and memory implementations
  registry/         # rules for creating, renaming, removing services and accounts
  source/           # Source interface and one sub-package per kind
  events/           # local event queue: append, dedup, readers, retention
  webhook/          # webhook receiver: verification, topic, delivery to the queue
  tunnel/           # public address for the receiver: Tailscale Funnel or a static URL
  oauth/            # OAuth discovery, client registration, login flow, token refresh
  core/             # the facade used by cli, daemon, gateway and webui
  guard/            # policies (readonly, allow, deny) and the call log
  skill/            # renders the skill the AI reads (SKILL.md + one file per service)
  daemon/           # background process sharing connections, the webhook receiver and command readers; its client and autostart
  gateway/          # MCP server for `pandora serve`
  install/          # writes the skill or the MCP entry into AI client configs
  webui/            # local HTTP API + embedded UI build (dist/ is generated, do not edit)
  cli/              # cobra commands, one file per command
  testutil/         # fakes and test servers shared by tests
ui/                 # SvelteKit (static SPA) + Svelte 5 + Tailwind 4, built into internal/webui/dist
```

Do not create generic `utils`, `helpers` or `common` packages.

## Rules

1. **Layers.** `cli`, `daemon`, `gateway` and `webui` only parse input, call `core` and format output. Business rules live in `registry`, `core`, `guard`, `source/*`, `oauth`, `events`, `webhook` and `tunnel`. Nothing in `cli` touches config files or the keychain directly.
2. **No secrets in the config file.** `config.toml` holds no token, password, API key or header value that grants access. Those live in the secret store under the keys described in `internal/secrets`. Never print a secret: not in logs, errors, `list` output or the UI.
3. **Stdout belongs to the protocol in `serve`.** In `pandora serve`, stdout carries MCP messages only. Logs go to stderr or to files under the state directory. Upstream stdio servers write their stderr to a log file, never to our stdout.
4. **Several processes run at once.** AI clients run `pandora` commands in parallel, MCP clients start their own `pandora serve`, and the daemon and UI run beside them. Every config write takes the config lock and writes atomically (temp file + rename). Every OAuth token refresh takes the account lock and re-reads the token before refreshing.
5. **No global state.** No package-level mutable variables (the only exception is `version` in `internal/cli`, set by the release build with `-ldflags`), no `init()` side effects. Wire dependencies with constructors in `cmd/pandora/main.go` and `internal/cli`.
6. **Errors.** Wrap with `fmt.Errorf("doing x: %w", err)`. Errors that reach a person say what to do next (`account todoist/personal needs login: run pandora auth todoist/personal`).
7. **Context first.** Anything that does I/O takes `context.Context` as its first argument.
8. **User-facing text in English.** CLI output, errors and UI copy are in English. Identifiers are in English.
9. **No comments in code** except when a line would be misleading without one. Names explain the code.
10. **Paths are injectable.** All paths derive from `config.Paths`. Setting `PANDORA_HOME` puts config, cache and state under that one directory; tests always use it with `t.TempDir()`. Setting `PANDORA_SECRETS=file` makes the secret store a 0600 JSON file under `PANDORA_HOME`, so tests never touch the real keychain.
11. **Every OS builds.** Releases ship for Linux, macOS, Windows and FreeBSD. Code that needs OS-specific calls lives in `_unix.go` / `_windows.go` files with build tags. Check with `GOOS=windows go vet ./...` before finishing.

## Stack

| Concern | Choice |
| --- | --- |
| Language | Go 1.27, one module (`github.com/PierreOliveira1/pandora`), one binary |
| MCP client and server | `github.com/modelcontextprotocol/go-sdk/mcp` |
| CLI | `github.com/spf13/cobra` |
| Config | TOML via `github.com/pelletier/go-toml/v2` |
| Secrets | macOS Keychain (and others) via `github.com/zalando/go-keyring` |
| Locks | `github.com/gofrs/flock` |
| OAuth | `golang.org/x/oauth2` + own discovery and registration code |
| OpenAPI | own loader over `go.yaml.in/yaml/v3` (3.x and Swagger 2.0) |
| HTTP for the UI | `net/http` from the standard library |
| UI | SvelteKit 3 with `adapter-static`, Svelte 5 runes, Tailwind 4, bun; embedded with `go:embed` |

## Tests

- Follow the global testing rules: name the behavior in one sentence, use the lowest layer that proves it, check outcomes, not internals.
- Unit tests for rules: name validation, account refs, policy matching, OpenAPI to tool mapping, argument templating.
- Integration tests for real boundaries: a real MCP server over stdio and HTTP (`internal/testutil`), OAuth against an `httptest` authorization server, config files written on a temp home.
- Run only the affected packages, with minimal parallelism:

```
go test -p 1 -parallel 1 ./internal/<package>/...
go vet ./...
go build ./...
```

`go build ./...` and `go vet ./...` are allowed and expected. Never run tests that hit real external services. Tests that start the daemon use a short temp directory for the socket and always shut it down.

UI (in `ui/`, with bun):

```
bun run check
bun run test:unit -- --run --maxWorkers=1
bun run build        # regenerates internal/webui/dist
VITE_MOCK=1 bun run dev   # UI with in-memory sample data
```

Every UI change is checked visually (desktop and 390 px wide, light and dark) before it is done.
