# Configuration

Pandora keeps its files in three directories: config, cache and state. `pandora paths` prints the exact paths on your machine:

```sh
pandora paths
```

## Default locations

Pandora uses the same XDG-style layout on every OS:

| Directory | Default | Override |
| --- | --- | --- |
| Config | `~/.config/pandora` | `XDG_CONFIG_HOME` or `PANDORA_HOME` |
| Cache | `~/.cache/pandora` | `XDG_CACHE_HOME` or `PANDORA_HOME` |
| State | `~/.local/state/pandora` | `XDG_STATE_HOME` or `PANDORA_HOME` |

On Windows the same directory names sit under your user profile (`%USERPROFILE%\.config\pandora`, `%USERPROFILE%\.cache\pandora` and `%USERPROFILE%\.local\state\pandora`), unless `HOME` or an XDG variable points elsewhere.

`PANDORA_HOME=/some/dir` puts all three under one directory: `/some/dir/config`, `/some/dir/cache` and `/some/dir/state`. This is how the tests and the development setup keep a separate configuration.

The home directory comes from `HOME`; when it is empty, Pandora asks the operating system for the user's home directory. Directories are created with mode `0700`; the config file is written with mode `0600`.

## Files

| Path | Contents |
| --- | --- |
| `config/config.toml` | Services, accounts, rules and installed clients. Never secrets. |
| `config/config.lock` | Lock used while writing the config. |
| `config/secrets.json` | Secrets, only when `PANDORA_SECRETS=file`. Mode `0600`. |
| `cache/tools/<service>.json` | Last tool catalog of each service, reused for 24 hours. |
| `state/calls.jsonl` | Call log. Rotates to `calls.jsonl.1` at 10 MB. |
| `state/logs/daemon.log` | Output of the background daemon. |
| `state/logs/<service>__<account>.log` | stderr of a local MCP server. |
| `state/locks/<service>__<account>.lock` | Lock used while refreshing an OAuth token. |
| `state/pandora.sock` | Daemon socket. If the path is too long for a Unix socket, it goes under the system temp directory. |

## config.toml

The file has a version, the clients that have Pandora installed, and a table per service. Pandora writes it for you; edit it only when the CLI has no flag for what you need, and run `pandora sync` afterwards.

```toml
version = 1
integrations = ["claude-code", "codex", "opencode"]

[services.todoist]
kind = "mcp-http"
description = "Tasks and projects."
url = "https://ai.todoist.net/mcp"
auth = "oauth"
default_account = "personal"
expose = "compact"

[services.todoist.oauth]
client_id = "YOUR_CLIENT_ID"
scopes = ["data:read_write"]
redirect_port = 33418

[services.todoist.accounts.personal]
description = "Personal tasks"
readonly = true
allow = ["get-*", "find-*"]
deny = ["delete-*"]

[services.todoist.accounts.work]
description = "Work tasks"
# url = "https://work.example.com/mcp"   # optional endpoint override for one account

[services.github]
kind = "mcp-stdio"
description = "Issues and pull requests."
command = ["npx", "-y", "@modelcontextprotocol/server-github"]

[services.github.accounts.work]
secret_env = ["GITHUB_PERSONAL_ACCESS_TOKEN"]

[services.git]
kind = "command"
description = "Local git repositories."
default_account = "default"

[[services.git.tools]]
name = "log"
description = "Recent commits"
command = ["git", "-C", "{{repo}}", "log", "--max-count={{n}}", "--oneline"]
readonly = true
timeout_sec = 30

[services.git.tools.params.repo]
type = "string"
required = true
description = "Repository path"

[services.git.tools.params.n]
type = "integer"

[services.git.accounts.default]
```

### Service fields

| Key | Meaning |
| --- | --- |
| `kind` | `mcp-http`, `mcp-stdio`, `openapi` or `command`. |
| `description` | What the service is for. Shown in the skill and the UI. |
| `url` | Endpoint of an `mcp-http` service. |
| `command` | Command that starts an `mcp-stdio` service. |
| `spec` | Path or URL of an `openapi` document. |
| `auth` | `oauth`, `header` or `none`. |
| `oauth.client_id` | Client ID of your own OAuth app. |
| `oauth.scopes` | Requested scopes. |
| `oauth.redirect_port` | Callback port when 33418 is busy. |
| `default_account` | Account used when a ref names only the service. |
| `expose` | `compact` or `direct` in MCP clients. Empty means `compact`. |
| `env` | Environment variables shared by the service. |
| `tools` | Tools of a `command` service. |

### Account fields

| Key | Meaning |
| --- | --- |
| `description` | What the account is for. |
| `url` | Optional endpoint override for this account. |
| `env` | Environment variables for this account. |
| `secret_env` | Names of secret environment variables. Values are in the secret store. |
| `secret_headers` | Names of secret HTTP headers. Values are in the secret store. |
| `readonly` | Block tools that are not marked read-only. |
| `allow` | Patterns of tools that may run. |
| `deny` | Patterns of tools that may never run. |
| `credentials_id` | Identifier Pandora uses to tell account generations apart. Leave it alone. |

### Command tool fields

| Key | Meaning |
| --- | --- |
| `name` | Tool name. Lowercase letters, digits and dashes, up to 32 characters. |
| `description` | What the tool does. |
| `command` | Command and arguments. `{{param}}` marks a parameter. |
| `params` | Map of parameter definitions. |
| `params.<name>.type` | `string`, `number`, `integer`, `boolean` or `array`. |
| `params.<name>.required` | Whether the argument is mandatory. |
| `params.<name>.description` | Description shown to the AI. |
| `readonly` | Marks the tool as read-only. |
| `timeout_sec` | Timeout in seconds. Empty means 60. |

## Secrets

`config.toml` holds no secret value. It holds the names in `secret_env` and `secret_headers`; the values live in the secret store. The same applies to OAuth tokens and client secrets. Never copy a value into the config file, and never paste a config file into an issue without checking it.

## Environment variables

| Variable | Effect |
| --- | --- |
| `PANDORA_HOME` | Put config, cache and state under `<dir>/config`, `<dir>/cache` and `<dir>/state`. |
| `PANDORA_SECRETS=file` | Store secrets in `config/secrets.json` instead of the OS keychain. Default: the OS keychain. |
| `PANDORA_NO_BROWSER=1` | Do not open a browser for `pandora ui` or an OAuth login. The URL is still printed. |
| `PANDORA_NO_DAEMON=1` | Bypass the background daemon and connect directly. |
| `PANDORA_CLIENT` | Override the client name recorded in the call log. |
| `HOME` | Home directory used for the default paths. |
| `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_STATE_HOME` | Override the three base directories. |

`PANDORA_VERSION` and `PANDORA_INSTALL_DIR` are read by the install script, not by the `pandora` command. See [Getting started](getting-started.md).

See [Safety](safety.md) for the rules and the call log, and [AI clients](ai-clients.md) for the files Pandora writes into each client.
