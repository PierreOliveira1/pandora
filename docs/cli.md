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
| `--http stringArray` | Base URL of an HTTP API as `name=URL`; a URL alone is named `api`. Repeatable. |
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

A secret given only by name (`--secret-env NAME`, `--header Name`) is read from the terminal without being shown. It can also come from stdin, one line per secret:

```sh
pbpaste | pandora add github --as personal --secret-env GITHUB_PERSONAL_ACCESS_TOKEN
```

With several secrets, the lines follow the flag order: `--secret-env` first, then `--header`. With no terminal and no piped value, the command stops and says how to provide it.

```sh
pandora add todoist --as personal --url https://ai.todoist.net/mcp
```

```sh
pandora add github --as work \
  --secret-env GITHUB_PERSONAL_ACCESS_TOKEN=YOUR_TOKEN \
  -- npx -y @modelcontextprotocol/server-github
```

```sh
pandora add shop --as store \
  --http api=https://api.example.com \
  --env STORE_ID=42 \
  --secret-env API_TOKEN
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

## pandora autostart

Start the Pandora daemon at login.

```
pandora autostart on|off|status
```

This command has no flags of its own.

| Subcommand | Meaning |
| --- | --- |
| `on` | Install the login item so the daemon starts at login. |
| `off` | Remove the login item. |
| `status` | Print `on` or `off`. |

```sh
pandora autostart on
pandora autostart status
pandora autostart off
```

Pandora suggests this after `pandora webhook add`; it never enables it by itself.

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

## pandora env

Manage the non-secret values of an account.

```
pandora env <service>/<account> [NAME=VALUE...] [--remove NAME] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--remove string` | Remove `NAME`. |

```sh
pandora env shop/store STORE_ID=42 CURRENCY=USD
```

```sh
pandora env shop/store
```

```sh
pandora env shop/store --remove CURRENCY
```

With no value arguments, it lists the account's values. Secret names appear as `NAME (secret)`; Pandora never prints the value. Setting a name that is a secret is refused.

## pandora events

Read webhook events from the local queue.

```
pandora events [<service>[/<account>]] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--topic string` | Only events with this topic. |
| `--reader string` | Reader name. Default `default`; the AI uses `ai`. |
| `--limit int` | Maximum events. Default 20; `0` reads all. |
| `--json` | Output JSON. |
| `--follow` | Keep printing new events. Checks the queue every 500 ms. With `--json`, prints one JSON object per line. |
| `--ack` | Mark the printed events as read. |

```sh
pandora events --reader ai --json
```

```sh
pandora events ig/store --topic comments --limit 50
```

```sh
pandora events --follow --json --reader ai --ack
```

Text mode prints one block per event: seq, local time, account, topic and the compact JSON payload.

### pandora events ack

Mark events as read.

```
pandora events ack <seq>... [flags]
pandora events ack --all [<service>[/<account>]] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--reader string` | Reader name. Default `default`; the AI uses `ai`. |
| `--topic string` | Only events with this topic. |
| `--all` | Mark every pending event matching the filter. |

```sh
pandora events ack 12 13 --reader ai
```

```sh
pandora events ack --all --topic comments --reader ai
```

### pandora events reader add

Run a program for each new event.

```
pandora events reader add <name> [flags] -- <program> <args>
```

| Flag | Meaning |
| --- | --- |
| `--from string` | Only events from `service` or `service/account`. |
| `--topic string` | Only events with this topic. |
| `--timeout int` | Timeout in seconds. Default 60. |
| `--replay` | Also run the program for the events already in the queue. |
| `--replay` | Also run the program for the events already in the queue. By default a new reader starts with the next event. |

```sh
pandora events reader add worker \
  --from ig/store --topic comments \
  -- /Users/me/scripts/reply.sh
```

The program receives the event as JSON on stdin and the variables `PANDORA_EVENT_SEQ`, `PANDORA_EVENT_ID`, `PANDORA_EVENT_SERVICE`, `PANDORA_EVENT_ACCOUNT` and `PANDORA_EVENT_TOPIC`. There is no shell and the program does not receive the account's secrets.

### pandora events reader list

List the command readers.

```
pandora events reader list [flags]
```

| Flag | Meaning |
| --- | --- |
| `--json` | Output JSON. |

```sh
pandora events reader list
```

### pandora events reader remove

Remove a command reader.

```
pandora events reader remove <name> [flags]
```

This command has no flags of its own.

```sh
pandora events reader remove worker
```

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

## pandora http

Manage the URLs and headers of HTTP services.

```
pandora http [command]
```

### pandora http header

Set or remove a header sent with every request of an HTTP service.

```
pandora http header <service> <name> <template> [flags]
pandora http header <service> <name> --remove [flags]
```

| Flag | Meaning |
| --- | --- |
| `--remove` | Remove the header. |

The template can only use `{{env.NAME}}`, which may be an environment or secret value of an account. The config stores the template, never the value.

```sh
pandora http header instagram Authorization "OAuth {{env.META_TOKEN}}"
```

```sh
pandora http header instagram Authorization --remove
```

`Host`, `Content-Length`, `Content-Type`, `Transfer-Encoding` and `Connection` are refused.

### pandora http show

Show the URLs, headers and HTTP tools of a service.

```
pandora http show <service> [flags]
```

This command has no flags of its own.

```sh
pandora http show instagram
```

### pandora http url

Set or remove a base URL of an HTTP service.

```
pandora http url <service> <name> <url> [flags]
pandora http url <service> <name> --remove [flags]
```

| Flag | Meaning |
| --- | --- |
| `--remove` | Remove the URL. |

```sh
pandora http url instagram beta https://api.example.com
```

```sh
pandora http url instagram beta --remove
```

A service must keep at least one URL. Pandora refuses to remove the last URL or one used by an HTTP tool. A command service cannot become an HTTP service; remove it and create it again.

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

A change that loosens the rules asks for confirmation in an interactive terminal and is refused without one. Loosening means removing a deny pattern, `--writable` on a read-only account, or widening or clearing the allow list. An AI cannot loosen the rules on its own. See [Safety](safety.md#only-you-can-loosen-a-rule).

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

Manage tools of command and HTTP services.

```
pandora tool [command]
```

### pandora tool add

Add a tool to a command or HTTP service.

```
pandora tool add <service> <name> [flags] [-- <command> | --http "METHOD /path"]
```

| Flag | Meaning |
| --- | --- |
| `--description string` | Tool description. |
| `--param stringArray` | Parameter as `name:type[:required][:description]`. Repeatable. |
| `--readonly` | Mark the tool as read-only. |
| `--timeout int` | Timeout in seconds. Default 60. |
| `--http string` | HTTP request as `"METHOD /path"`. |
| `--url string` | URL name of the HTTP service. Required when the service has several URLs. |
| `--query stringArray` | Query parameter `key=value`. Repeatable. |
| `--form stringArray` | Form field `key=value`. Repeatable. |
| `--json stringArray` | JSON field `key.path=value`. Repeatable. |
| `--request-header stringArray` | Request header `Name=value`. Repeatable. |

For a command service, each `{{name}}` in the command is replaced by the argument with that name, as a whole argument or inside one (`--title={{title}}`). A command argument that holds a missing optional parameter is dropped. An array used as a whole argument becomes one argument per item.

The command runs directly, without a shell: pipes, redirections and `$VARS` are not expanded, so call a script for more than one step. The account's `--env` and `--secret-env` values reach the program as environment variables.

```sh
pandora tool add git log \
  --description "Recent commits" \
  --param repo:string:required:"Repository path" \
  --param n:integer \
  --readonly \
  -- git -C {{repo}} log --max-count={{n}} --oneline
```

For an HTTP service, use `--http "METHOD /path"`. Methods are `GET`, `HEAD`, `POST`, `PUT`, `PATCH` and `DELETE`. `--url` chooses a base URL, and `--query`, `--form`, `--json` and `--request-header` describe the request. `{{param}}` and `{{env.NAME}}` work in these fields; a secret environment value is allowed only in service headers. A path value is escaped, so `a/b` becomes `a%2Fb`. `--form` and `--json` cannot be used together, GET and HEAD have no body and are read-only, and the timeout defaults to 60 seconds.

```sh
pandora tool add shop orders \
  --http "GET /v1/stores/{{env.STORE_ID}}/orders" \
  --url api \
  --param limit:integer \
  --query "limit={{limit}}" \
  --readonly
```

An HTTP service also accepts a command after `--` for a flow with several steps.

### pandora tool remove

Remove a tool from a command or HTTP service.

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

## pandora tunnel

Expose the webhook receiver to the internet.

```
pandora tunnel use tailscale
pandora tunnel use url <https-base>
pandora tunnel status
pandora tunnel off
```

### pandora tunnel use

Choose how webhooks reach this machine.

```
pandora tunnel use tailscale
pandora tunnel use url <https-base>
```

This command has no flags of its own.

```sh
pandora tunnel use tailscale
```

```sh
pandora tunnel use url https://hooks.example.com
```

`tailscale` uses Tailscale Funnel and prints the public address. `url` requires an HTTPS base that forwards to the local port. The command lists the Callback URLs that changed; update them in each service.

### pandora tunnel status

Show the tunnel.

```
pandora tunnel status [flags]
```

| Flag | Meaning |
| --- | --- |
| `--json` | Output JSON. |

```sh
pandora tunnel status
```

### pandora tunnel off

Stop publishing the receiver.

```
pandora tunnel off [flags]
```

This command has no flags of its own.

```sh
pandora tunnel off
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

## pandora upgrade

Upgrade pandora to the latest release.

```
pandora upgrade [flags]
```

| Flag | Meaning |
| --- | --- |
| `--check` | Only tell whether a newer release exists. |
| `--version string` | Install this version, even when it is older. |

```sh
pandora upgrade
```

```sh
pandora upgrade --check
```

```sh
pandora upgrade --version 0.3.0
```

It downloads the build for your system from the GitHub releases, checks it against `checksums.txt` and replaces the running binary in place. Then it regenerates the skill in your AI clients and restarts the daemon if it was running. The directory of the binary must be writable. A binary built from source has no release version: reinstall it, or pass `--version`.

## pandora webhook

Manage incoming webhooks.

```
pandora webhook [command]
```

### pandora webhook add

Receive webhook deliveries for an account.

```
pandora webhook add <service>[/<account>] --style hmac|token|none [flags]
```

| Flag | Meaning |
| --- | --- |
| `--style string` | Verification style: `hmac`, `token` or `none`. Required. |
| `--challenge string` | Use `hub` to answer the GET verification with `hub.challenge`. |
| `--signature-header string` | HMAC signature header. Default `X-Hub-Signature-256`. |
| `--signature-prefix string` | HMAC value prefix. Default `sha256=`. |
| `--encoding string` | HMAC encoding: `hex` or `base64`. Default `hex`. |
| `--token-header string` | Header that carries a token style token. |
| `--token-query string` | Query parameter that carries a token style token. |
| `--topic-header string` | Header that carries the event topic. |
| `--topic-field stringArray` | JSON path that carries the event topic. Repeatable. |
| `--id-header string` | Header that carries the delivery id, used for deduplication. |
| `--insecure` | Accept unsigned deliveries with `--style none`. |

For `--style hmac`, Pandora asks for the signing secret without showing it, or reads it from stdin. For `--style token`, Pandora generates the token. `--style none` requires `--insecure`.

```sh
pandora webhook add instagram/store --style hmac --challenge hub \
  --topic-field entry.0.changes.0.field --topic-field entry.0.messaging
```

```sh
pandora webhook add github/work --style hmac \
  --topic-header X-GitHub-Event --id-header X-GitHub-Delivery
```

The command prints the Callback URL, the Verify token when there is a hub challenge, and how to test it. The token style token is printed only in a terminal; run `pandora webhook show <s>/<a> --token` there to see it.

### pandora webhook list

List webhooks.

```
pandora webhook list [flags]
```

| Flag | Meaning |
| --- | --- |
| `--json` | Output JSON. |

```sh
pandora webhook list
```

### pandora webhook show

Show one webhook.

```
pandora webhook show <service>[/<account>] [flags]
```

| Flag | Meaning |
| --- | --- |
| `--token` | Show the token style token, only in a terminal. |
| `--json` | Output JSON. |

```sh
pandora webhook show instagram/store
```

```sh
pandora webhook show instagram/store --token
```

### pandora webhook remove

Remove a webhook and its signing secrets.

```
pandora webhook remove <service>[/<account>] [flags]
```

This command has no flags of its own.

```sh
pandora webhook remove instagram/store
```

Delete the Callback URL from the service's settings too.

### pandora webhook test

Send a verification and a signed test delivery.

```
pandora webhook test <service>[/<account>] [flags]
```

This command has no flags of its own.

```sh
pandora webhook test instagram/store
```

Read the test event with `pandora events instagram/store`.
