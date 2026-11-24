# Sources

A source is whatever gives Pandora tools: a remote MCP server, a local MCP server, an OpenAPI document, an HTTP API without a spec or your own commands. Every source is added as an account of a service. The service owns the kind and the connection details; the account owns the credentials.

`pandora add` accepts exactly one source per new service. Use a command after `--`, `--url`, `--openapi`, `--http` or `--kind command`, never more than one.

## Remote MCP server

Use a remote MCP server when the provider gives you an HTTP endpoint:

```sh
pandora add todoist --as personal --url https://ai.todoist.net/mcp
```

Pandora tries Streamable HTTP first and falls back to Server-Sent Events. If the server answers with a login challenge, Pandora discovers its OAuth settings and starts the login. To save the account without logging in:

```sh
pandora add todoist --as personal --url https://ai.todoist.net/mcp --no-login
pandora auth todoist/personal
```

To add another account of the same service, omit the URL:

```sh
pandora add todoist --as work
```

OAuth flags:

| Flag | Effect |
| --- | --- |
| `--oauth` | Use OAuth even when the first response does not ask for it. |
| `--client-id ID` | Use an OAuth app you created instead of dynamic registration. |
| `--client-secret SECRET` | Send the client secret of your OAuth app. It goes to the secret store. |
| `--scope SCOPE` | Request a scope. Repeat the flag for more scopes. |
| `--no-login` | Save the account without starting the OAuth login. |

A provider that does not support dynamic registration needs an app with this redirect URL:

```
http://localhost:33418/callback
```

Pass its ID with `--client-id`. If the port is busy, set `oauth.redirect_port` for the service in the config file. See [Accounts and login](accounts-and-login.md).

Static headers work when the server expects an API key or a bearer token:

```sh
pandora add weather --as work \
  --url https://api.example.com/mcp \
  --header 'X-API-Key: YOUR_API_KEY'
```

If you write only `--header 'X-API-Key'`, Pandora prompts for the value. Header values are stored in the secret store, not in the config file. Every account can have different headers:

```sh
pandora add weather --as personal --header 'X-API-Key: OTHER_KEY'
```

## Local MCP server

A local MCP server is a command that speaks MCP on stdin and stdout. Pass the command and its arguments after `--`:

```sh
pandora add github --as work -- npx -y @modelcontextprotocol/server-github
```

Pandora passes your environment to the process and overrides it with the account's variables. Use `--env` for ordinary values and `--secret-env` for secrets:

```sh
pandora add github --as work \
  --env GITHUB_API_URL=https://api.github.com \
  --secret-env GITHUB_PERSONAL_ACCESS_TOKEN=YOUR_TOKEN \
  -- npx -y @modelcontextprotocol/server-github
```

Write only the name to be prompted for the secret:

```sh
pandora add github --as work \
  --secret-env GITHUB_PERSONAL_ACCESS_TOKEN \
  -- npx -y @modelcontextprotocol/server-github
```

A value given only by name (`--secret-env NAME` or `--header Name`) can also come from stdin, one line per secret. On macOS, pipe the clipboard:

```sh
pbpaste | pandora add github --as personal --secret-env GITHUB_PERSONAL_ACCESS_TOKEN
```

With several secrets, the lines are read in flag order: all `--secret-env` first, then all `--header`. If there is no terminal and no piped value, Pandora stops and says how to provide it.

The stderr of the server goes to a log file under the state directory; `pandora paths` prints the directory. The server never writes to Pandora's stdout.

A second account of the same local server reuses the service command:

```sh
pandora add github --as personal --secret-env GITHUB_PERSONAL_ACCESS_TOKEN=OTHER_TOKEN
```

## OpenAPI

Describe an HTTP API with an OpenAPI 3.x or Swagger 2.0 document. The document can be a local file or a URL:

```sh
pandora add weather --as work \
  --openapi https://api.example.com/openapi.json \
  --header 'X-API-Key: YOUR_API_KEY'
```

```sh
pandora add weather --as work \
  --openapi ./openapi.yaml \
  --header 'X-API-Key: YOUR_API_KEY'
```

Each operation becomes a tool. The tool name comes from `operationId` when present, otherwise from the method and path. Path and query parameters become arguments, the request body becomes a `body` argument, and response schemas are not needed.

The base URL comes from the first `servers` entry (OpenAPI 3.x) or from `host` and `schemes` (Swagger 2.0). A relative server URL is resolved against the URL of the spec. If a local spec has no base URL, Pandora reports that it cannot build the API URL.

Header values are secrets and are sent to the spec URL when it is remote and to every API call. They are stored per account, so two accounts can use different API keys:

```sh
pandora add weather --as personal --header 'X-API-Key: OTHER_KEY'
```

Pandora follows redirects, but refuses to send secret headers to a different scheme or host. Specs are limited to 10 MB.

Read-only rules work out of the box: `GET`, `HEAD` and `OPTIONS` are marked read-only, and `DELETE` is marked destructive.

## HTTP API

Use an HTTP service for an HTTP API with no OpenAPI document. A tool is a request described in the config; there is no code to write. A service has one or more named base URLs, optional headers and accounts with their own values and secrets.

Create it with `--http name=URL` for each base URL. A URL without a name becomes `api`. The base must use `https`; `http` is allowed only for `localhost`, `127.0.0.1` and `::1`. It cannot have a query, a fragment or user info, and a trailing slash is removed.

```sh
pandora add shop --as store \
  --http api=https://api.example.com \
  --http admin=https://admin.example.com \
  --env STORE_ID=42 \
  --secret-env API_TOKEN
```

`--env` sets an ordinary account value, `--secret-env` a secret environment value and `--header` a secret header for the account. An HTTP service does not use OAuth.

Add or replace a base URL later:

```sh
pandora http url shop admin https://admin.example.com/v2
pandora http url shop admin --remove
```

A service must keep at least one URL. Pandora refuses to remove the last one or one that an HTTP tool uses. A command service cannot become an HTTP service; remove it and create it again.

Service headers are sent with every request. The template can only use `{{env.NAME}}`, from the account's environment or secret store. The config stores the template, never the value:

```sh
pandora http header shop Authorization "Bearer {{env.API_TOKEN}}"
pandora http header shop Authorization --remove
```

`Host`, `Content-Length`, `Content-Type`, `Transfer-Encoding` and `Connection` are refused.

Show the service:

```sh
pandora http show shop
```

It lists the URLs, the headers (templates) and the HTTP tools saved in the config.

Account values that are not secret:

```sh
pandora env shop/store STORE_ID=42 CURRENCY=USD
pandora env shop/store
pandora env shop/store --remove CURRENCY
```

The list shows non-secret values as `NAME=VALUE` and secret names as `NAME (secret)`. Pandora never prints a secret value and refuses to overwrite a secret with `pandora env`.

### HTTP tools

Add a GET tool with a query parameter:

```sh
pandora tool add shop orders \
  --http "GET /v1/stores/{{env.STORE_ID}}/orders" \
  --url api \
  --param limit:integer \
  --query "limit={{limit}}" \
  --readonly
```

Methods are `GET`, `HEAD`, `POST`, `PUT`, `PATCH` and `DELETE`. `--url` chooses a base URL; it is required when the service has several, and with a single URL the tool stores its name and uses it when the flag is omitted. `--query`, `--form`, `--json` and `--request-header` describe the request.

`{{param}}` and `{{env.NAME}}` work in the path, query, form, JSON and request headers. A secret environment value can only be used in service headers. A path value is escaped: `a/b` becomes `a%2Fb`, so it does not create a new path segment. `.` and `..` are refused.

An optional argument that is not given drops the query entry, form field, JSON field or request header that uses it. Arrays become `a,b` in text. In `--json`, a whole placeholder such as `{{n}}` keeps the argument type (number, boolean or array), and a dotted key nests objects:

```sh
pandora tool add shop publish \
  --http "POST /v1/stores/{{env.STORE_ID}}/orders" \
  --url api \
  --param total:number \
  --param currency:string \
  --json "order.total={{total}}" \
  --json "order.currency={{currency}}"
```

`--form` and `--json` cannot be used together. GET and HEAD have no body and are read-only; `--readonly` marks other methods. The default timeout is 60 seconds.

An HTTP service also accepts program tools. Use a command after `--` for a flow with several steps.

### The automatic `http_get`

Every HTTP service also gets the `http_get` tool. It makes a read-only GET request with a required `path` and optional `url` and `query`, so an AI can explore the API before saving a tool:

```sh
pandora tools shop
pandora describe shop http_get
```

`url` is required when the service has several base URLs.

### How a request is sent

Pandora calls only the base URLs of the service. A redirect to another host is refused. The account's secret headers are sent too. The order is: tool headers, then service headers, then account secret headers, and the last one wins.

A 2xx response is the result, truncated like a command result. Any other status becomes an error with `HTTP <status>` and the body. Request headers never appear in the output or in the logs.

Meta (Instagram, Facebook and WhatsApp) is an HTTP service. Use the Graph base URL, the account secret and the service header:

```sh
pandora add instagram --as store \
  --http graph=https://graph.facebook.com/v22.0 \
  --env IG_USER_ID=17841400000000000 \
  --secret-env META_TOKEN

pandora http header instagram Authorization "OAuth {{env.META_TOKEN}}"
```

## Commands

Use an HTTP service for an HTTP API without an OpenAPI document. A command tool remains the right choice for a flow that needs several steps, local files or a program.

A command service turns local programs into typed tools. It does not run a shell: the command and its arguments are executed directly, so pipes, globs and redirections are not interpreted.

Create the service first. A command service still needs an account alias; `default` is the usual choice:

```sh
pandora add git --as default --kind command \
  --service-description "Local git repositories"
```

Add a tool. Parameters use `name:type[:required][:description]`, and the command follows `--`:

```sh
pandora tool add git log \
  --description "Recent commits" \
  --param repo:string:required:"Repository path" \
  --param n:integer \
  --readonly \
  --timeout 30 \
  -- git -C {{repo}} log --max-count={{n}} --oneline
```

Supported types are `string`, `number`, `integer`, `boolean` and `array`. `{{param}}` can be a whole argument or part of one:

- A whole argument, such as `{{repo}}`, is replaced by the value.
- An argument with text around the placeholder, such as `--max-count={{n}}`, is expanded into one argument.
- A missing optional parameter drops the whole argument that contains it.
- An array used as a whole argument becomes several arguments. An array embedded in an argument is joined with commas.
- A value used as a whole argument that starts with `-` is refused unless the command contains a literal `--` before the placeholder, which stops Pandora from reading it as a flag.

Every tool has a timeout; the default is 60 seconds. The process output is captured, and output over 100 KB is truncated with a note. The exit status and stderr become a tool error.

Run it:

```sh
pandora call git/default log '{"repo": "/path/to/repo", "n": 5}'
pandora call git/default log --arg repo=/path/to/repo --arg n=5
```

List and remove tools:

```sh
pandora tools git
pandora tool remove git log
```

Use `--env` or `--secret-env` on the account to pass environment variables to the program. `--readonly` marks the tool for read-only accounts; without it, a read-only account refuses to run the tool.

## Common flags

| Flag | Applies to | Meaning |
| --- | --- | --- |
| `--as ACCOUNT` | every source | Account alias. Required. |
| `--url URL` | `mcp-http` | Remote MCP endpoint. |
| `--openapi SPEC` | `openapi` | Path or URL of the document. |
| `--http name=URL` | `http` | Base URL of an HTTP API. Repeatable; a URL without a name becomes `api`. |
| `--kind command` | `command` | Declares a command service. It is the only accepted value. |
| `--env NAME=VALUE` | `mcp-stdio`, `command`, `http` | Ordinary environment variable. Repeatable. |
| `--secret-env NAME[=VALUE]` | `mcp-stdio`, `command`, `http` | Secret environment variable. Repeatable. Prompts without a value. |
| `--header NAME[: VALUE]` | `mcp-http`, `openapi`, `http` | Secret HTTP header. Repeatable. Prompts without a value. |
| `--oauth`, `--client-id`, `--client-secret`, `--scope`, `--no-login` | `mcp-http` | OAuth settings. |
| `--description TEXT` | every source | Account description. |
| `--service-description TEXT` | new service | Service description, used in the skill. |
