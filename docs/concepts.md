# Concepts

Pandora has a small vocabulary. Once these terms are clear, every command makes sense.

## Service

A service is a named definition of how to reach something. It has a kind (`mcp-http`, `mcp-stdio`, `openapi` or `command`), a URL, a command or a spec, an authentication mode and a description. Examples are `todoist`, `meta`, `github` and `git`. The name uses lowercase letters, digits and dashes, up to 32 characters.

## Account

An account is one set of credentials for a service, with an alias. The alias follows the same naming rule. A service can have many accounts:

- `todoist/personal`
- `todoist/work`

Each account has its own secrets, its own policy and, when needed, its own endpoint. The service holds what is shared; the account holds what identifies the person or workspace.

## Account ref

An account ref is `service/account`. You can also write only `service`, which means its default account. A service has a default account, usually the first one added. If a service has exactly one account, that account is used. If it has several and no default, Pandora asks you to name the account.

Commands that change account settings need the full ref: `pandora policy todoist/work`. Reading commands accept either form: `pandora call todoist YOUR_TOOL ...` uses the default account.

## Source kinds

The core knows no specific service. Anything enters through one of four kinds:

| Kind | What it is | How you add it |
| --- | --- | --- |
| `mcp-http` | A remote MCP server reached over HTTP | `pandora add SERVICE --as ACCOUNT --url https://example.com/mcp` |
| `mcp-stdio` | A local MCP server started as a command | `pandora add SERVICE --as ACCOUNT -- npx -y PACKAGE` |
| `openapi` | An HTTP API described by an OpenAPI 3.x or Swagger 2.0 document | `pandora add SERVICE --as ACCOUNT --openapi ./spec.yaml` |
| `command` | Your own commands declared as typed tools | `pandora add SERVICE --as default --kind command` |

A source is the live connection to one account. It can list tools and call them. Pandora opens a source on demand, keeps it open in a connection pool and closes it when it is idle or when the account changes.

## Catalog cache

Listing tools costs a round trip to the service. Pandora stores the last catalog for each service under the cache directory and reuses it for 24 hours. This lets the skill and `pandora tools` work when the upstream is briefly unavailable, and it keeps commands fast.

`pandora tools SERVICE --refresh` fetches the list again and updates the cache. Logging in also refreshes it, and adding an MCP account that connects right away refreshes it too. After adding a command tool, run `pandora tools SERVICE --refresh` to load the new list.

## Skill versus MCP

There are two ways an AI client reaches Pandora:

- **Skill.** Pandora writes a skill into Claude Code, Codex or OpenCode. The AI reads a short guide and runs the `pandora` CLI: `pandora search`, `pandora describe`, `pandora call`. The tool catalog stays out of the model context until the AI asks for it, so many services and many tools remain cheap.
- **MCP.** The client starts `pandora serve`, an MCP server on stdio. The client sees Pandora's tools directly. This is the only option for Cursor and Claude Desktop, and it is available for every client with `pandora install --mcp CLIENT`.

Both paths use the same accounts, the same rules and the same daemon.

## Expose mode

MCP clients see a service in one of two modes:

- `compact` (default): four Pandora tools, `pandora_accounts`, `pandora_search_tools`, `pandora_describe_tool` and `pandora_call`, cover every service.
- `direct`: every tool of the service is exposed with the name `service__tool` and an `account` argument.

Change it with `pandora expose SERVICE compact` or `pandora expose SERVICE direct`. Restart the MCP client to see the change.

## Daemon

Reading commands share a background daemon. It starts on demand, keeps upstream connections open, and stops after 15 minutes without activity. When at least one account has a webhook, it keeps running so deliveries keep arriving. It also runs the command readers. Every AI client talks to the same daemon, so a connection opened by one client is reused by another.

Commands that change the config always run in the calling process, never in the daemon. The daemon re-reads the config for every request, so it sees changes immediately. Set `PANDORA_NO_DAEMON=1` to bypass it and connect directly; this is useful when debugging.

`pandora autostart on` installs a login item that starts the daemon when you log in, on macOS and Linux. `pandora autostart status` shows it and `pandora autostart off` removes it.

## Webhooks and events

A webhook is a URL a service sends events to. Pandora verifies each delivery, reads its topic and appends it to a local event queue. The public address comes from a tunnel: Tailscale Funnel, or an HTTPS base you provide. The receiver accepts a body up to 1 MB and deduplicates deliveries for 24 hours.

A reader is a named position in the queue. `ai` is the reader AI clients use, `default` is the CLI default, and a command reader is a program the daemon runs for each event. Read and acknowledge events with `pandora events` and `pandora events ack`. See [Webhooks](webhooks.md) for the flags, the tests and the safety notes.

## The whole picture

```
Claude Code / Codex / OpenCode / Cursor / Claude Desktop
        |                                   |
        | skill: pandora CLI                | MCP: pandora serve
        +----------------+------------------+
                         |
                   pandora (CLI)
                         |
                 +-------+-------+
                 | shared daemon |
                 +-------+-------+
                         |
        +----------------+----------------+
        |                |                |
   mcp-http          mcp-stdio      openapi / command
        |                |                |
   remote MCP       local MCP      HTTP API / local program
        |                |                |
   service/account  service/account  service/account
```

The next pages cover each piece: [Sources](sources.md), [Accounts and login](accounts-and-login.md), [AI clients](ai-clients.md), [Safety](safety.md) and [Configuration](configuration.md).
