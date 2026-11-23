# Safety

Pandora sits between your AI clients and every service. This page covers the rules that decide what may run, and the log that records what did.

## Policies

A policy belongs to an account. Show it:

```sh
pandora policy todoist/work
```

Make an account read-only:

```sh
pandora policy todoist/work --readonly
```

Allow tools that change data again:

```sh
pandora policy todoist/work --writable
```

Allow only some tools:

```sh
pandora policy todoist/work --allow 'get-*' --allow 'find-*'
```

Block some tools:

```sh
pandora policy todoist/work --deny 'delete-*'
```

Remove a pattern from the allow and deny lists:

```sh
pandora policy todoist/work --remove 'delete-*'
```

Clear the read-only flag, the allow list and the deny list, then apply the flags you pass:

```sh
pandora policy todoist/work --clear --readonly --allow 'get-*'
```

`--allow` and `--deny` are repeatable. `--json` prints the result as JSON.

### How a decision is made

1. A deny pattern blocks the tool, even when an allow pattern also names it.
2. If the allow list is not empty, every tool it does not name is blocked.
3. If the account is read-only, a tool that is not marked read-only is blocked.

Patterns use shell-style matching: `*` matches any sequence except `/`, `?` matches one character, and `[abc]` matches one character from the set. Matching is case-sensitive. There is no `**`.

A read-only account checks the tool annotations from the source. An OpenAPI `GET`, `HEAD` or `OPTIONS` operation is marked read-only. A command tool is read-only only when you add it with `pandora tool add --readonly`. An MCP server decides for its own tools. A tool without an annotation is treated as able to change data, so a read-only account blocks it.

`pandora describe` shows the annotation:

```sh
pandora describe todoist YOUR_TOOL
```

The tool name ends with ` (read-only)` or ` (destructive)` when the source declares it.

Policies apply in the CLI, the MCP server and the web UI, because all three go through the same policy check. A blocked call does not reach the service.

## Call log

Pandora records every tool call it makes. See the latest calls:

```sh
pandora log
pandora log -n 20 --errors
pandora log --account todoist/work
pandora log --json
```

Each row shows:

| Field | Meaning |
| --- | --- |
| Time | When the call started, in your local time. |
| Client | Who made the call: `terminal`, an AI client, `mcp:<name>`, or `pandora-ui`. |
| Account | The resolved account, such as `todoist/work`. |
| Tool | The tool name. |
| Result | `ok`, `error`, `blocked`, or a short error message. |
| Duration | How long the call took, in milliseconds. |

Pandora never logs tool arguments, response bodies, tokens or header values. Error text is kept up to 500 characters.

The log lives under the state directory as `calls.jsonl`; `pandora paths` prints the directory. It rotates when it passes 10 MB: the old file becomes `calls.jsonl.1` and a new file starts. `pandora log` reads both. The files are mode `0600` in a directory created with mode `0700`.

The web UI shows the same entries on the Activity screen.

## Client detection

Pandora detects the client that ran the call from environment variables set by the AI app:

| Variable | Client |
| --- | --- |
| `CLAUDECODE` | `claude-code` |
| `OPENCODE` | `opencode` |
| `CODEX_SANDBOX` or `CODEX_HOME` | `codex` |
| `CURSOR_TRACE_ID` or `CURSOR_AGENT` | `cursor` |
| `PANDORA_CLIENT` | The value you set, overriding the detection |

Without one of these variables the client is `terminal`. Calls that arrive through the MCP server use `mcp:` followed by the client name reported during the MCP handshake. Calls from the web UI are recorded as `pandora-ui`.

## Limits

A policy decides whether a tool may run. It does not sandbox what the tool does. Local MCP servers and command tools run with your user's permissions, and a remote service can do whatever its own credentials allow. Only add MCP servers, OpenAPI specs and commands you trust. Use read-only accounts for services where the AI should only read, and deny destructive tools you never want it to call.
