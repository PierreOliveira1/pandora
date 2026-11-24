# Getting started

This guide takes you from a clean machine to a first tool call from an AI client. You need a terminal and an AI client such as Claude Code, Codex, OpenCode, Cursor or Claude Desktop.

## Install

On macOS, Linux or FreeBSD:

```sh
curl -fsSL https://raw.githubusercontent.com/PierreOliveira1/pandora/main/install.sh | sh
```

The script installs `pandora` to `~/.local/bin` and verifies the sha256 checksum. If the directory is not on your `PATH`, add it:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Set `PANDORA_VERSION` to install a specific version, or `PANDORA_INSTALL_DIR` to install somewhere else.

Other options:

- Download an archive from [releases](https://github.com/PierreOliveira1/pandora/releases). Builds exist for Linux, macOS, Windows and FreeBSD, on amd64 and arm64. Windows support is experimental. Verify the file with `checksums.txt`.
- Install from source with Go 1.27 or newer. The web UI is already embedded in the binary:

  ```sh
  go install github.com/PierreOliveira1/pandora/cmd/pandora@latest
  ```

Check the installation:

```sh
pandora --version
```

Upgrade later with `pandora upgrade`. It checks the download, replaces the binary and updates the skill in your AI clients.

## Add your first account

A remote MCP server that uses OAuth needs only its URL. Todoist is a good first example. Run it in a terminal so Pandora can open the login page:

```sh
pandora add todoist --as personal --url https://ai.todoist.net/mcp
```

Pandora discovers the OAuth settings from the server, registers a client when the provider supports dynamic registration, and prints a login URL. It opens your browser unless `PANDORA_NO_BROWSER=1` is set. Finish the login in the browser. Pandora then lists the tools and prints how many are available.

If you prefer to save the account first, use `--no-login` and log in when you are ready:

```sh
pandora add todoist --as personal --url https://ai.todoist.net/mcp --no-login
pandora auth todoist/personal
```

Check the account:

```sh
pandora list
pandora status todoist/personal
```

## Add a second account of the same service

The service definition already exists, so do not repeat the URL:

```sh
pandora add todoist --as work
```

Open the printed login URL in a private window. A normal window may reuse the session of the first account and log you in to the wrong one. If Pandora only prints a login hint, run:

```sh
pandora auth todoist/work
```

Both accounts now appear in `pandora list`. The first account you added is the default account of the service, which you can change:

```sh
pandora default todoist/work
```

## Add a local MCP server

A local MCP server is a command that Pandora starts. Pass the command after `--`. Use `--secret-env` for tokens; the value goes to the secret store, not to the config file:

```sh
pandora add github --as work \
  --secret-env GITHUB_PERSONAL_ACCESS_TOKEN=YOUR_TOKEN \
  -- npx -y @modelcontextprotocol/server-github
```

If you write only the variable name, Pandora prompts for the value:

```sh
pandora add github --as work \
  --secret-env GITHUB_PERSONAL_ACCESS_TOKEN \
  -- npx -y @modelcontextprotocol/server-github
```

Use `--env` for values that are not secret. Check the connection:

```sh
pandora status github/work
```

## Connect your AI client

Install the generated skill in the clients Pandora detects on your machine:

```sh
pandora install
```

This covers Claude Code, Codex and OpenCode. Cursor and Claude Desktop have no skills, so name them and Pandora adds itself to them as an MCP server:

```sh
pandora install cursor claude-desktop
```

Restart the AI client so it reads the new skill or MCP server. Then check the setup:

```sh
pandora doctor
```

`pandora doctor` checks the config, the secret store, the `pandora` command on `PATH`, every installed client and the connection to each account. It prints a fix when something is wrong.

## Try it from the terminal

The AI uses the same commands you can run yourself:

```sh
pandora tools todoist
pandora search "add task" --service todoist
pandora describe todoist YOUR_TOOL
pandora call todoist/personal YOUR_TOOL '{"arg": "value"}'
```

`YOUR_TOOL` is a tool name returned by `pandora tools` or `pandora search`. `pandora describe` shows the tool description and the input schema. Pass the arguments as JSON, or pass `-` and send the JSON on stdin:

```sh
pandora call todoist/personal YOUR_TOOL - <<'JSON'
{"arg": "value"}
JSON
```

Add `--json` to any reading command for machine-readable output. Add `--arg name=value` to set a single argument without writing the whole JSON object.

## Open the web UI

```sh
pandora ui
```

Pandora opens the UI in your browser on a random port bound to `127.0.0.1`. The URL carries a session token. To choose a port or only print the URL:

```sh
pandora ui --port 8765
pandora ui --no-browser
```

Press Ctrl+C to stop the UI. See [Web UI](web-ui.md) for the screens and the security model.

## Next

- [Sources](sources.md) for OpenAPI specs and command tools.
- [Accounts and login](accounts-and-login.md) for OAuth details and secret storage.
- [AI clients](ai-clients.md) for what Pandora writes into each client.
- [Safety](safety.md) for read-only, allow and deny rules.
