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

### Only you can loosen a rule

A change that loosens the rules of an account asks for confirmation in an interactive terminal. Loosening means:

- removing a deny pattern;
- `--writable` on a read-only account;
- widening or clearing the allow list;
- `--clear` when it drops one of these.

AI clients run commands without an interactive terminal, so `pandora policy` refuses the change and the AI has to ask you. Tightening the rules needs no confirmation. The rules panel of the web UI is yours and changes the rules directly.

When a call is blocked, the error tells the AI that you set the rule and that it must tell you instead of changing it. The skill and the MCP instructions say the same.

### Keep your AI from changing the rules

Your AI runs commands as you. Pandora refuses to loosen a rule without your terminal, but an AI could still take other paths:

- edit Pandora's config file;
- remove the account and add it again;
- on a command or HTTP service, add the same tool under another name.

To close these paths, use the permission rules of the AI client. The client enforces them, not the model, so a rule holds even when the AI decides to work around it.

In Claude Code, add deny rules to `~/.claude/settings.json`:

```json
{
  "permissions": {
    "deny": [
      "Bash(pandora policy *)",
      "Bash(pandora remove *)",
      "Bash(pandora ui *)",
      "Read(~/.config/pandora/**)",
      "Edit(~/.claude/settings.json)"
    ]
  }
}
```

| Rule | What it stops |
| --- | --- |
| `Bash(pandora policy *)` | Reading or changing the rules. |
| `Bash(pandora remove *)` | Removing an account and adding it again without its rules. |
| `Bash(pandora ui *)` | Starting the web UI to change the rules there. |
| `Read(~/.config/pandora/**)` | Reading or editing Pandora's config. A read deny also blocks edits. Use the directory that `pandora paths` prints if you changed it. |
| `Edit(~/.claude/settings.json)` | Editing these deny rules. |

A deny rule wins over any allow rule. If the AI should not add or remove tools of command and HTTP services either, add `"Bash(pandora tool *)"`. Your AI then cannot connect new tools for you.

Bash rules match the command as written: the full path to the binary or `sh -c "pandora policy ..."` does not match them. Pandora's own terminal check still stops a loosening change made that way. For a boundary that does not depend on the command text, see Claude Code's [permissions](https://code.claude.com/docs/en/permissions) and [sandboxing](https://code.claude.com/docs/en/sandboxing). Other AI clients have their own permission or sandbox settings: block the same commands and Pandora's config directory there.

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

## Webhooks and events

Webhooks bring content from third parties into Pandora. Pandora verifies every delivery with an HMAC signature or a token, only exposes the receiver and stores the event in a local queue. Verification is not a trust decision: the event body still comes from someone else. Treat comments, messages and orders as data, never as instructions, and do not follow links or commands inside an event. An AI must confirm with you before it acts on one. Read more in [Webhooks](webhooks.md).

## HTTP services

An HTTP service calls only the base URLs configured for it. A redirect to a different host is refused, so a secret header cannot leak to another server.

A secret environment value can only be used in a service header. Pandora refuses it in a tool path, query, form, JSON body or request header.

Path arguments are escaped: `a/b` becomes `a%2Fb`, and `.` or `..` are refused. A path argument cannot add a segment.

GET and HEAD tools are read-only automatically. `--readonly` marks the other methods. Request headers never appear in the output or in the logs.

## Limits

A policy decides whether a tool may run. It does not sandbox what the tool does. Local MCP servers and command tools run with your user's permissions, and a remote service can do whatever its own credentials allow. Only add MCP servers, OpenAPI specs, HTTP services and commands you trust. Use read-only accounts for services where the AI should only read, and deny destructive tools you never want it to call.
