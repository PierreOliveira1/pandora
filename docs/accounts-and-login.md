# Accounts and login

An account is one set of credentials for a service. A service can have many accounts, each with its own alias, secrets and rules. Read [Concepts](concepts.md) first if these words are new.

## OAuth

Remote MCP servers (`mcp-http`) can use OAuth. When you add an account, Pandora opens the server, reads the login challenge and discovers the authorization server. It requires PKCE with S256, registers a client when the provider supports dynamic registration, and opens the authorization URL in your browser.

```sh
pandora add todoist --as personal --url https://ai.todoist.net/mcp
```

The login listens on `http://localhost:33418/callback` and waits up to 5 minutes. If your browser does not open, copy the URL printed in the terminal. `PANDORA_NO_BROWSER=1` stops Pandora from opening a browser but still prints the URL:

```sh
PANDORA_NO_BROWSER=1 pandora add todoist --as personal --url https://ai.todoist.net/mcp
```

To save an account without logging in, add `--no-login`, then log in when you are ready:

```sh
pandora add todoist --as personal --url https://ai.todoist.net/mcp --no-login
pandora auth todoist/personal
```

`pandora auth` works on an account that already exists. If you pass only a service, Pandora uses its default account.

Pandora refreshes tokens automatically. A refresh takes an account lock and re-reads the saved token, so two processes cannot overwrite each other. If the provider rejects the refresh token, Pandora reports that the account needs login; run `pandora auth service/account` again.

To log out, remove the saved token but keep the account:

```sh
pandora logout todoist/personal
```

### Your own OAuth app

Some providers do not support dynamic client registration. Create an OAuth app with this redirect URL:

```
http://localhost:33418/callback
```

Then pass its client ID when you add the service:

```sh
pandora add meta --as store \
  --url https://mcp.example.com/mcp \
  --client-id YOUR_CLIENT_ID
```

If the provider gives you a client secret, pass `--client-secret`. It is stored in the secret store, never in the config file:

```sh
pandora add meta --as store \
  --url https://mcp.example.com/mcp \
  --client-id YOUR_CLIENT_ID \
  --client-secret YOUR_CLIENT_SECRET \
  --scope ads_read \
  --scope ads_write
```

Repeat `--scope` for every scope you need. If port 33418 is busy, set `oauth.redirect_port` for the service in the config file, and use the same port in the redirect URL of your OAuth app.

### Second account of the same service

The service definition already exists, so add only the account:

```sh
pandora add todoist --as work
```

Open the login URL in a private window. A normal window may reuse the session of the first account and authorize the wrong one. If Pandora only prints a login hint because it is not running in a terminal, log in explicitly:

```sh
pandora auth todoist/work
```

## Static headers

Services that use an API key instead of OAuth accept secret headers. This works for `mcp-http` and `openapi` services:

```sh
pandora add weather --as work \
  --url https://api.example.com/mcp \
  --header 'X-API-Key: YOUR_API_KEY'
```

Write only the name to be prompted:

```sh
pandora add weather --as work \
  --url https://api.example.com/mcp \
  --header 'X-API-Key'
```

Header values are stored in the secret store under the service and account. `pandora list` shows the account, never the value. A service that uses static headers cannot switch to OAuth: `pandora auth` tells you to remove the account and add it again with new headers.

## Secret storage

By default, secrets go to the operating system keychain: tokens, headers, environment variables and OAuth client secrets. The config file stores only the names of the secret variables and headers.

On a headless machine, or when the keychain is not reachable, use a local file:

```sh
export PANDORA_SECRETS=file
```

Pandora then stores secrets in `secrets.json` inside the config directory, which `pandora paths` prints. The file is plain JSON with mode `0600`, and every write is atomic and locked. Protect it like a password file. You can switch modes at any time, but secrets saved in one store are not copied to the other; run `pandora auth` and re-add static headers after switching.

`pandora doctor` tests the secret store and suggests `PANDORA_SECRETS=file` when the keychain fails.

## Default account

The first account added to a service becomes its default. Reading commands accept only the service name and use that account:

```sh
pandora call todoist YOUR_TOOL '{"arg": "value"}'
```

Change the default:

```sh
pandora default todoist/work
```

If a service has one account, it is used even without a default. If a service has several accounts and no default, name the account explicitly.

## Rename

Rename an account:

```sh
pandora rename todoist/personal home
```

Pandora moves the account's secrets to the new alias and keeps the default setting. Rename a service by passing only its name:

```sh
pandora rename todoist tasks
```

The new name must use lowercase letters, digits and dashes, up to 32 characters.

## Remove

Remove one account. Its credentials are deleted:

```sh
pandora remove todoist/personal
```

Remove a service and all its accounts:

```sh
pandora remove todoist --yes
```

Without `--yes`, Pandora asks for confirmation in a terminal. In a script or any non-interactive process, `--yes` is required.

## Descriptions

A description helps the AI choose the right service or account. Set it when you add the account:

```sh
pandora add todoist --as work --description "Work tasks"
```

Change it later for an account or a service:

```sh
pandora set-description todoist/work "Work tasks"
pandora set-description todoist "Tasks and projects"
```

Descriptions appear in `pandora list`, the generated skill and the web UI.
