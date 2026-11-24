# Connect a new service or account

Read this when the user asks to connect a service, an API or another account. Follow the steps in order.

Never ask the user to paste a password, token or API key into the chat, and never put a secret value in a command you run. The user types secrets in their own terminal (step 3).

## 1. Choose the source

Look in the service's official docs and use the first option that exists:

| The service offers | Command |
| --- | --- |
| A remote MCP server (an `https://` URL) | `pandora add <service> --as <account> --url <url>` |
| A local MCP server (a package run with `npx`, `uvx` or similar) | `pandora add <service> --as <account> -- <command> <args>` |
| An OpenAPI (Swagger) spec of its HTTP API | `pandora add <service> --as <account> --openapi <url-or-path>` |
| Only an HTTP API without a spec, a CLI or a script | a command service, see step 4 |

- Prefer the service's official MCP server. Ask the user before you use a third-party package or a paid middleman service.
- From the same docs, tell the user what they must set up on the service's side: account type, developer app, permissions or scopes, and how long a token lasts. Get ids your tools need (like a user or account id) from the API itself when it offers an endpoint for that, instead of asking the user.
- Names use lowercase letters, digits and hyphens, up to 32 characters. The account name says whose account it is: `personal`, `work`, `store-a`.
- `--description "<text>"` describes the account. `--service-description "<text>"` describes the service when it is created.
- To add another account to a service that already exists, run `pandora add <service> --as <new-account>` with only that account's credentials (`--secret-env`, `--header`, `--env`). All accounts of a service share its tools.
- `pandora add --help` lists every flag.

## 2. Login (OAuth)

`pandora add` detects remote MCP servers that use OAuth. When it prints `Login required`, ask the user to run `pandora auth <service>/<account>` in their terminal. It opens the browser. For a second account of the same service, tell them to use a private window or to sign out first. If the server rejects automatic client registration, add `--client-id <id>` (and `--client-secret`, `--scope`) from the service's developer console.

Pandora can only log in to servers that publish OAuth discovery metadata, which most remote MCP servers do. For other APIs, the user creates a long-lived token or API key in the service's developer console and stores it as a secret (step 3).

## 3. Secrets

Write the full command for the user to run in their own terminal. Include the secret names, never the values:

- `--secret-env NAME` sets an environment variable, for local MCP servers and command services.
- `--header "Name"` sets an HTTP header, for remote MCP servers and OpenAPI. For a bearer token, use `--header "Authorization"` and type `Bearer <token>` as the value.

Pandora asks for each value without showing it and saves it in the system keychain. The user can also pipe the value, for example from the clipboard on macOS: `pbpaste | pandora add ... --secret-env NAME`.

If you run such a command yourself, it stops with `value for NAME required`. That is expected: give the command to the user.

Values that are not secret, like an account id, go in `--env NAME=value`.

Tokens that expire must be replaced by the user. Tell them when it will expire, and that they replace it with `pandora remove <service>/<account>` followed by the same `pandora add` command.

## 4. Command services

A command service turns any program into tools: a CLI, a script, or `curl` calls to an HTTP API.

```
pandora add <service> --as <account> --kind command --secret-env API_TOKEN
pandora tool add <service> <tool> --description "<what it does>" \
  --param <name>:<type>[:required][:<description>] \
  -- <program> <arguments with {{name}} placeholders>
```

- Parameter types are `string`, `number`, `integer`, `boolean` and `array`. Repeat `--param` for each parameter.
- `{{name}}` is replaced by the value of the argument with that name. It can be a whole argument (`{{file}}`) or part of one (`--title={{title}}`). When an optional argument is not given, the command argument that holds it is dropped. An array placeholder that is a whole argument becomes one argument per item.
- A value that starts with `-` is refused in a whole-argument placeholder, unless the placeholder comes after a literal `--` in the command.
- There is no shell: `|`, `&&`, `>` and `$VAR` are passed as plain text. For more than one step, write a script and pass the placeholders to it: `-- /path/to/publish.sh {{video}} {{caption}}`. Save scripts in a folder the user chose, not in a temporary one.
- The account's `--env` and `--secret-env` values reach the program as environment variables, so a script reads its token from `$API_TOKEN`. Each account has its own values.
- The program's stdout is the result. A non-zero exit is an error, shown with its stderr.
- `--readonly` marks tools that only read. `--timeout <seconds>` (default 60) gives slow work, like uploads, more time.
- `pandora tool remove <service> <tool>` removes a tool.
- The service and its tools stay when its last account is removed, so you can prepare the tools first and let the user add the account with the secret.
- Check what the API accepts before you design the tool. For example, if it only takes a public URL for a file, say so to the user instead of taking a local path.

## 5. Check it

1. `pandora tools <service>` lists the tools. `pandora describe <service> <tool>` shows a tool's arguments.
2. Run a read-only tool first. Ask the user before you run a tool that posts, changes or deletes data.
3. `pandora doctor` checks every account.

To experiment without touching the user's setup, set `PANDORA_HOME` to one empty temporary folder, with `PANDORA_SECRETS=file` and `PANDORA_NO_DAEMON=1`, on every command. If you add something by mistake, remove it with `pandora remove <service>/<account>`, or `pandora remove <service> --yes` for a whole service.
