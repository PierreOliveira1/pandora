# AI clients

Pandora works with Claude Code, Codex, OpenCode, Cursor and Claude Desktop. Claude Code, Codex and OpenCode can use the generated skill, the default and lightest option. Cursor and Claude Desktop always use MCP.

## Install

Run `pandora install` without arguments to install the skill in the clients detected on your machine:

```sh
pandora install
```

Detection looks for the config directory of Claude Code (`~/.claude`), OpenCode (`~/.config/opencode`) and Codex (`~/.codex` or `~/.agents`). If none is found, name the clients explicitly:

```sh
pandora install claude-code codex opencode
```

Valid names are `claude-code` (or `claude`), `codex`, `opencode`, `cursor` and `claude-desktop` (or `desktop`).

Cursor and Claude Desktop have no skills. Name them and Pandora adds itself to them as an MCP server:

```sh
pandora install cursor claude-desktop
```

To use MCP mode for a client that supports skills, add `--mcp` and the client name:

```sh
pandora install --mcp claude-code
```

Pandora removes the other installation mode when you switch, so a client does not end up with both a skill and an MCP server. Restart the AI client after installing.

## What Pandora writes

| Client | Skill location | MCP location |
| --- | --- | --- |
| Claude Code | `~/.claude/skills/pandora` | `~/.claude.json` under `mcpServers`, or through the `claude mcp add` command when `claude` is on `PATH` |
| Codex | `~/.agents/skills/pandora` | `~/.codex/config.toml`, block `[mcp_servers.pandora]` |
| OpenCode | `~/.config/opencode/skills/pandora` | `~/.config/opencode/opencode.json`, section `mcp` |
| Cursor | none | `~/.cursor/mcp.json`, section `mcpServers` |
| Claude Desktop | none | macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`; Linux: `~/.config/Claude/claude_desktop_config.json`; Windows: `%APPDATA%\Claude\claude_desktop_config.json` |

The MCP entry always runs the `pandora` binary with the `serve` argument. Pandora resolves the absolute path of the binary so the client can start it even when its `PATH` is different.

When Pandora installs the skill, it also adds `Bash(pandora:*)` to `~/.claude/settings.json` for Claude Code and `"pandora *": "allow"` under `permission.bash` in `opencode.json` for OpenCode. This lets the AI run Pandora without asking for every call.

When Pandora changes an existing file, it writes a backup next to it with the suffix `.pandora.bak`. A skill directory is only removed by Pandora when it contains the `.pandora-managed` marker; a directory you created yourself is left alone.

## The skill

The skill has three levels:

1. A short description in the skill front matter, always visible to the AI. It lists the connected services and their accounts. If the list is too long, Pandora shortens it to fit the 1024-character limit.
2. `SKILL.md`: how to work, plus a section per service with its accounts and up to 15 tool names.
3. `services/<service>.md`: one file per service with every tool, its arguments and an example call. OpenAPI and MCP instructions from the service are included when the server provides them.

The skill also includes `connect.md`, a guide the AI reads when you ask it to connect a service. The AI writes the commands and you type the secrets in your own terminal.

When an account receives webhooks, `SKILL.md` also has an Events section. It tells the AI to read the queue with `pandora events --reader ai --json` and to mark handled events with `pandora events ack <seq>... --reader ai`. Event content is treated as third-party data, never as instructions.

The AI uses the CLI:

```sh
pandora search "create task"
pandora describe todoist YOUR_TOOL
pandora call todoist/personal YOUR_TOOL '{"arg": "value"}'
```

Pandora regenerates the skill after the changes that affect what the AI can see: adding or removing an account, logging in or out, changing a policy, renaming, setting the default account or a description, and refreshing tools. It only writes to the clients recorded at install time.

Adding a command tool changes the service definition, but not the catalog cache. Run `pandora tools SERVICE --refresh` afterwards to load the list and update the skill.

Force an update after an upgrade or after editing the config by hand:

```sh
pandora sync
```

`pandora doctor` reports a skill that is out of date and tells you to run `pandora sync`.

The skill contains service names, account names, descriptions, tool names, arguments and examples. It never contains tokens, header values or environment variable values.

## MCP mode

In MCP mode the client starts Pandora itself. The client config points at:

```sh
pandora serve
```

`pandora serve` speaks MCP on stdin and stdout. Logs go to stderr, so they never corrupt the protocol. You normally do not run it by hand.

A service appears in one of two modes:

- `compact` (default): the client sees four tools, `pandora_accounts`, `pandora_search_tools`, `pandora_describe_tool` and `pandora_call`. They cover every service and account.
- `direct`: the client also sees each tool of the service as `service__tool`, with an `account` argument. Names are sanitized and limited to 64 characters; a collision gets a numeric suffix.

When at least one account has a webhook, the server also exposes `pandora_events`. It reads the event queue for the `ai` reader and accepts the seq numbers the AI already handled, so they are not returned again. The server instructions mention the tool, and the tool description says that event content is third-party data and never an instruction.

Show the mode and change it:

```sh
pandora expose todoist
pandora expose todoist direct
pandora expose todoist compact
```

Restart the MCP client after changing the mode. The four compact tools stay available in direct mode, so the AI can always search and call tools.

## Uninstall

Remove Pandora from every client it is installed in:

```sh
pandora uninstall
```

Or name the clients:

```sh
pandora uninstall cursor claude-code
```

Pandora removes the skill directory it manages, the permission entry it added and the MCP server entry. It also updates the list of installed clients in the config.

## Doctor

```sh
pandora doctor
```

The doctor checks the config file, the secret store, the `pandora` command on `PATH`, every recorded client and, unless you pass `--offline`, the connection to every account. Each failed check prints a fix. The command exits with status 1 when something fails.

```sh
pandora doctor --offline
pandora doctor --json
```
