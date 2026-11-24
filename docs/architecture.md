# Architecture

This page is for contributors. It maps the code, follows one `pandora call` and one webhook delivery, and explains where state lives. Read [AGENTS](../AGENTS.md) for the rules that keep the layers apart.

## Layout

Pandora is one Go module and one binary. The CLI, the daemon, the MCP gateway and the web UI all call the same core; none of them duplicates a business rule.

| Path | Responsibility |
| --- | --- |
| `cmd/pandora` | `main.go`: reads arguments and signal, calls `cli.Execute`. Wiring only. |
| `internal/cli` | Cobra commands. Parse input, call the core or the registry, format output. |
| `internal/config` | Paths, config types and TOML tags, load and save with a lock and atomic rename, call log path. |
| `internal/secrets` | Secret store interface and the keychain, file and memory implementations. Secret key names. |
| `internal/registry` | Rules for adding, renaming and removing services and accounts; default account; policies; command tools; expose mode. Writes config and secrets. |
| `internal/source` | `Source` interface, opener registry and shared errors. |
| `internal/source/mcpsource` | Remote and local MCP servers: Streamable HTTP with SSE fallback, stdio process, headers, OAuth transport. |
| `internal/source/openapi` | Loads OpenAPI 3.x and Swagger 2.0, resolves `$ref`, turns operations into tools, builds requests. |
| `internal/source/command` | Compiles command templates, validates typed arguments and runs local programs. |
| `internal/oauth` | Discovery, protected resource metadata, dynamic client registration, PKCE login, token refresh and transport. |
| `internal/events` | Local event queue: append, deduplication, readers, retention and pruning. |
| `internal/webhook` | Webhook receiver: signature or token verification, hub challenge, topic extraction and delivery to the queue. |
| `internal/tunnel` | Public address for the receiver: Tailscale Funnel or a static HTTPS URL. |
| `internal/core` | Facade used by every entry point: connection pool, tool catalog and cache, calls, policies, auth, install, skills, events, webhooks, tunnel and doctor. |
| `internal/guard` | Policy decision (read-only, allow, deny) and the call log. |
| `internal/skill` | Renders `SKILL.md` and one service file per service. |
| `internal/install` | Writes skills, MCP entries, permission entries and backups into AI client configs. |
| `internal/daemon` | Unix socket server, client, autostart, webhook receiver and command readers. Holds the shared pool of upstream connections. |
| `internal/gateway` | MCP server for `pandora serve`: four compact tools, optional direct tools and `pandora_events` when a webhook exists. |
| `internal/webui` | Local HTTP API, embedded static files, session and CSRF checks, login flow, webhook and event views, autostart. |
| `internal/testutil` | Fakes and servers shared by tests: `fakemcp`, `fakeauth`. |
| `ui` | SvelteKit, Svelte 5 and Tailwind source for the web UI. Built into `internal/webui/dist`. |

## One `pandora call`

```
pandora call todoist/work get-tasks '{"project": "inbox"}'
  |
  | cli parses the ref, the tool and the JSON arguments
  v
deps.backend
  |  PANDORA_NO_DAEMON=1 -> core directly
  |  otherwise -> daemon.Ensure -> unix socket -> daemon
  v
core.Call
  |  resolve service/account from config (default account if the ref has none)
  |  guard.Check: deny, allow, read-only
  |  record start time
  v
source.Source.CallTool
  |  mcp-http: HTTP request with headers or an OAuth token
  |  mcp-stdio: JSON-RPC on the child process stdin/stdout
  |  openapi: build the HTTP request from the operation
  |  command: substitute the arguments into argv and run the program
  v
upstream service
  |
  v
core.recordCall -> state/calls.jsonl
```

The daemon runs the same core. It opens a source once per account, keeps it in a pool, and invalidates it when the account's config fingerprint changes. The config is re-read on every request, so adding, renaming or removing an account takes effect without restarting the daemon.

A config change never runs in the daemon. `pandora add`, `pandora policy` and the other changing commands run in the calling process. They take the config lock, write a temporary file and rename it over the old one. OAuth refreshes take the per-account lock and re-read the token before refreshing, so two processes cannot lose a rotated token.

`pandora serve` builds a gateway over the core and a daemon-backed client. The web UI calls the core and the registry directly. Both still go through `core.Call`, so policies and the call log apply everywhere.

## One webhook delivery

A service sends an event to the callback URL. The public address comes from the tunnel; the daemon owns the receiver and the queue.

```
service
  |  signed delivery
  v
https://<host>/pandora/hooks/<service>/<account>/<id>
  |
  |  Tailscale Funnel, or the static HTTPS base
  v
daemon: 127.0.0.1:<port>
  |
  v
webhook.Handler
  |  lookup service, account and id in the config
  |  reject bodies over 1 MB
  |  verify hmac or token in constant time
  |  answer hub.challenge on GET
  |  read the topic and the delivery id
  |  deduplicate for 24 hours
  v
events.Queue.Append -> state/events/events.jsonl
  |
  |  ai and default readers read through the CLI and the MCP gateway
  |  command readers: the daemon runs them in order, with retry
  v
reader position -> readers.json
```

The receiver is the only Pandora port exposed to the internet. The daemon logs each verification, each accepted delivery and each refused delivery to `logs/daemon.log`. The port and the public address are under `[webhooks]` in the config.

## State

All paths come from `config.Paths`, so `PANDORA_HOME` moves everything at once.

- Config: `config.toml` (including `[webhooks]`, `[[readers]]` and the webhook of each account), `config.lock`, and `secrets.json` when `PANDORA_SECRETS=file`.
- Cache: `tools/<service>.json`, the catalog cache with a 24-hour lifetime.
- State: `calls.jsonl` and its rotation, `logs/daemon.log` with webhook verifications and deliveries, one upstream log per local MCP account, `events/` with the event queue, account locks and the daemon socket.

Secrets are never part of the config. The config stores the names of secret environment variables and headers, and the secret store holds the values under keys built from service, account and name.

## UI build

The UI is a static SvelteKit application. Its source is in `ui/`; `bun run build` writes the static build into `internal/webui/dist`. The Go package embeds that directory with `go:embed all:dist` and serves it from the same server as the JSON API.

`internal/webui/dist` is generated. Do not edit it by hand. After any change under `ui/`, run the build and commit the result. CI runs the same build and fails when `internal/webui/dist` differs from the UI source.

## Testing layers

- Unit tests prove one rule: name validation, ref parsing, policy matching, command templating, OpenAPI operation mapping, skill rendering.
- Integration tests prove a real boundary: an MCP server over stdio or HTTP through `testutil/fakemcp`, OAuth against an `httptest` authorization server through `testutil/fakeauth`, config files written under a temporary `PANDORA_HOME`, the daemon over a temporary socket.
- UI tests prove behavior in the browser through Vitest.

Tests never call a real external service. During development, run the affected packages with minimal parallelism:

```sh
go test -p 1 -parallel 1 ./internal/<package>/...
go vet ./...
go build ./...
```

Before a pull request, the full suite:

```sh
go test ./...
```

See [CONTRIBUTING](../CONTRIBUTING.md) for the UI commands, commit style and release process.
