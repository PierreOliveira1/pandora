# CLI reference

Every command accepts these global flags:

| Flag | Meaning |
| --- | --- |
| `-h, --help` | Print help for the command. |
| `--verbose` | Print backend details. |

`pandora --version` prints the version. `pandora help [command]` prints the same help as `--help`. The tables below list only the flags of each command.

## pandora add

Add a service account.

```
pandora add <service> --as <account> [flags] [-- command args...]
```

| Flag | Meaning |
| --- | --- |
| `--as string` | Account alias. Required. |
| `--url string` | MCP server URL. |
| `--openapi string` | Path or URL of an OpenAPI spec. |
| `--kind string` | Service kind. The only accepted value is `command`. |
| `--env stringArray` | Environment variable `KEY=VALUE`. Repeatable. |
| `--secret-env stringArray` | Secret environment variable `NAME` or `NAME=VALUE`. Repeatable. Without a value, Pandora prompts. |
| `--header stringArray` | Secret header `Name` or `Name: value`. Repeatable. Without a value, Pandora prompts. |
| `--oauth` | Use OAuth. |
| `--client-id string` | OAuth client ID. |
| `--client-secret string` | OAuth client secret. |
| `--scope stringArray` | OAuth scope. Repeatable. |
| `--description string` | Account description. |
| `--service-description string` | Service description, only when creating the service. |
| `--no-login` | Do not start the OAuth login. |

```sh
pandora add todoist --as personal --url https://ai.todoist.net/mcp
```

```sh
pandora add github --as work \
  --secret-env GITHUB_PERSONAL_ACCESS_TOKEN=YOUR_TOKEN \
  -- npx -y @modelcontextprotocol/server-github
```

## pandora auth

Log in to an OAuth account.

```
pandora auth <service|service/account> [flags]
```

This command has no flags of its own.

```sh
pandora auth todoist/work
```

## pandora call

Call a tool on an account.

```
pandora call <ref> <tool> [json-args|-] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--arg stringArray` | Tool argument `KEY=VALUE`. Repeatable. The value is read as JSON when possible, otherwise as a string. |
| `--json` | Output JSON. |

```sh
pandora call todoist/work YOUR_TOOL '{"arg": "value"}'
```

```sh
pandora call todoist/work YOUR_TOOL --arg arg=value
```

Pass `-` instead of JSON to read the arguments from stdin.

## pandora completion

Generate the autocompletion script for the specified shell.

```
pandora completion [command]
```

This command has no flags of its own.

### pandora completion bash

Generate the autocompletion script for the bash shell.

```
pandora completion bash
```

| Flag | Meaning |
| --- | --- |
| `--no-descriptions` | Disable completion descriptions. |

```sh
pandora completion bash --no-descriptions
```

### pandora completion zsh

Generate the autocompletion script for the zsh shell.

```
pandora completion zsh [flags]
```

| Flag | Meaning |
| --- | --- |
| `--no-descriptions` | Disable completion descriptions. |

```sh
pandora completion zsh --no-descriptions
```

### pandora completion fish

Generate the autocompletion script for the fish shell.

```
pandora completion fish [flags]
```

| Flag | Meaning |
| --- | --- |
| `--no-descriptions` | Disable completion descriptions. |

```sh
pandora completion fish --no-descriptions
```

### pandora completion powershell

Generate the autocompletion script for powershell.

```
pandora completion powershell [flags]
```

| Flag | Meaning |
| --- | --- |
| `--no-descriptions` | Disable completion descriptions. |

```sh
pandora completion powershell --no-descriptions
```

## pandora daemon

Run the shared connection daemon in the foreground.

```
pandora daemon [flags]
```

| Flag | Meaning |
| --- | --- |
| `--idle duration` | Stop after this much inactivity. Default `15m0s`. |

```sh
pandora daemon --idle 5m
```

Normally you do not run the daemon yourself; reading commands start it on demand.

### pandora daemon start

Start the daemon in the background.

```
pandora daemon start [flags]
```

This command has no flags of its own.

```sh
pandora daemon start
```

### pandora daemon status

Show the running daemon.

```
pandora daemon status [flags]
```

This command has no flags of its own. It prints `not running` when there is no daemon.

```sh
pandora daemon status
```

### pandora daemon stop

Stop the daemon.

```
pandora daemon stop [flags]
```

This command has no flags of its own. It prints `not running` when there is no daemon.

```sh
pandora daemon stop
```

## pandora default

Set the default account of a service.

```
pandora default <service/account> [flags]
```

This command has no flags of its own.

```sh
pandora default todoist/work
```

## pandora describe

Show a tool's description and input schema.

```
pandora describe <service> <tool> [flags]
```

| Flag | Meaning |
| --- | --- |
| `--json` | Output JSON. |

```sh
pandora describe todoist YOUR_TOOL
```

## pandora doctor

Diagnose configuration and connections.

```
pandora doctor [flags]
```

| Flag | Meaning |
| --- | --- |
| `--offline` | Skip live account checks. |
| `--json` | Output JSON. |

```sh
pandora doctor --offline
```

The command exits with status 1 when a check fails.

## pandora expose

Show or change how a service is exposed in MCP clients.

```
pandora expose <service> [compact|direct] [flags]
```

This command has no flags of its own.

```sh
pandora expose todoist direct
```

With no mode, it prints the current mode.

## pandora help

Help provides help for any command in the application.

```
pandora help [command] [flags]
```

This command has no flags of its own.

```sh
pandora help add
```

## pandora install

Install the Pandora skill or MCP server in your AI clients.

```
pandora install [client...] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--json` | Output JSON. |
| `--mcp` | Install Pandora as an MCP server. Requires a client name. |

```sh
pandora install
```

```sh
pandora install --mcp cursor
```

## pandora list

List services and accounts.

```
pandora list [flags]
```

| Flag | Meaning |
| --- | --- |
| `--json` | Output JSON. |

```sh
pandora list
```

## pandora log

Show the latest tool calls.

```
pandora log [flags]
```

| Flag | Meaning |
| --- | --- |
| `-n, --n int` | Number of calls to show. Default 50. |
| `--account string` | Filter by account or service. |
| `--errors` | Show only failed or blocked calls. |
| `--json` | Output JSON. |

```sh
pandora log -n 20 --errors
```

## pandora logout

Log out of an OAuth account.

```
pandora logout <service|service/account> [flags]
```

This command has no flags of its own.

```sh
pandora logout todoist/work
```

## pandora paths

Print the config, cache and state directories.

```
pandora paths [flags]
```

This command has no flags of its own.

```sh
pandora paths
```

## pandora policy

Show or change the policy of an account.

```
pandora policy <service/account> [flags]
```

| Flag | Meaning |
| --- | --- |
| `--readonly` | Make the account read-only. |
| `--writable` | Allow tools that change data. |
| `--allow stringArray` | Allow a tool pattern. Repeatable. |
| `--deny stringArray` | Deny a tool pattern. Repeatable. |
| `--remove stringArray` | Remove a pattern from the allow and deny lists. Repeatable. |
| `--clear` | Clear read-only, allow and deny before applying changes. |
| `--json` | Output JSON. |

```sh
pandora policy todoist/work --readonly --allow 'get-*'
```

## pandora remove

Remove an account or a service.

```
pandora remove <service/account> [flags]
```

| Flag | Meaning |
| --- | --- |
| `--yes` | Skip confirmation. |

```sh
pandora remove todoist/work
```

```sh
pandora remove todoist --yes
```

## pandora rename

Rename an account or a service.

```
pandora rename <service/account> <new-account> [flags]
```

This command has no flags of its own.

```sh
pandora rename todoist/personal home
```

Pass only the service name to rename the service: `pandora rename todoist tasks`.

## pandora search

Search tools by name and description.

```
pandora search <query...> [flags]
```

| Flag | Meaning |
| --- | --- |
| `--service string` | Search only this service. |
| `--limit int` | Maximum number of results. |
| `--json` | Output JSON. |

```sh
pandora search "create task" --service todoist
```

## pandora serve

Run Pandora as an MCP server on stdio.

```
pandora serve [flags]
```

This command has no flags of its own. MCP clients start it for you. Stdout carries the protocol; logs go to stderr.

```sh
pandora serve
```

## pandora set-description

Set the description of a service or account.

```
pandora set-description <service|service/account> <text> [flags]
```

This command has no flags of its own.

```sh
pandora set-description todoist/work "Work tasks"
```

## pandora status

Check the connection to accounts.

```
pandora status [ref] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--json` | Output JSON. |

```sh
pandora status todoist/work
```

Without a ref, it checks every account.

## pandora sync

Regenerate the Pandora skill in installed AI clients.

```
pandora sync [flags]
```

This command has no flags of its own.

```sh
pandora sync
```

## pandora tool

Manage tools of command services.

```
pandora tool [command]
```

### pandora tool add

Add a tool to a command service.

```
pandora tool add <service> <name> [flags] -- <command and args>
```

| Flag | Meaning |
| --- | --- |
| `--description string` | Tool description. |
| `--param stringArray` | Parameter as `name:type[:required][:description]`. Repeatable. |
| `--readonly` | Mark the tool as read-only. |
| `--timeout int` | Timeout in seconds. Default 60. |

```sh
pandora tool add git log \
  --description "Recent commits" \
  --param repo:string:required:"Repository path" \
  --param n:integer \
  --readonly \
  -- git -C {{repo}} log --max-count={{n}} --oneline
```

### pandora tool remove

Remove a tool from a command service.

```
pandora tool remove <service> <name> [flags]
```

This command has no flags of its own.

```sh
pandora tool remove git log
```

## pandora tools

List the tools of connected services.

```
pandora tools [service] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--refresh` | Fetch the tools again. |
| `--json` | Output JSON. |

```sh
pandora tools todoist
```

## pandora ui

Open the local Pandora UI.

```
pandora ui [flags]
```

| Flag | Meaning |
| --- | --- |
| `--port int` | Port to listen on. `0` chooses a random port. |
| `--no-browser` | Do not open a browser. |

```sh
pandora ui --port 8765 --no-browser
```

## pandora uninstall

Remove Pandora from your AI clients.

```
pandora uninstall [client...] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--json` | Output JSON. |
| `--mcp` | Show the MCP server removal in the output. |

```sh
pandora uninstall
```

```sh
pandora uninstall cursor claude-code
```
