# Security Policy

## Supported versions

Pandora supports the latest release only. If you report a problem, update to the latest release first and confirm that it still happens.

## Reporting a vulnerability

Do not open a public issue for a security problem. Use GitHub private vulnerability reporting:

https://github.com/PierreOliveira1/pandora/security/advisories/new

If that link is unavailable, contact the maintainers privately through the contact in the maintainer's GitHub profile.

Please include:

- The Pandora version (`pandora --version`) and your operating system.
- The AI client or entry point involved: CLI, `pandora serve`, or the web UI.
- The steps to reproduce, the impact and, when possible, a minimal proof of concept.
- Whether the problem needs a local attacker, a malicious MCP server, a malicious OpenAPI document or a malicious command.
- Any relevant logs with tokens, secrets, account names and private URLs removed.

## Response

We acknowledge a report within 3 business days. We then confirm the impact, prepare a fix and a release, and credit the reporter unless you ask us not to. Please give us time to ship a fix before you disclose the problem.

## Scope

Pandora-specific areas we care about:

- Secret storage: the keychain store, the `PANDORA_SECRETS=file` store, its permissions and its locking.
- OAuth: discovery, dynamic client registration, PKCE, the loopback callback, token storage and refresh.
- The local web UI: the session token and cookie, the Host and Origin checks, the CSRF header, the Content Security Policy and the HTTP API.
- The daemon socket: who can connect, how it is created and how requests are authorized.
- Command tools: argument validation, template expansion and the program that runs.
- MCP servers you add: they run with your user's permissions, so a malicious server is a security problem for the whole machine.

Out of scope: problems in a third-party service, a vulnerability that requires an already compromised machine, and reports that only show a missing hardening header without an impact.

## Good practices for users

- Only add MCP servers, OpenAPI documents and commands you trust. They run with your permissions and can read what you can read.
- Use read-only accounts and deny rules for services where the AI should not change data.
- Keep tokens in the keychain. If you use `PANDORA_SECRETS=file`, protect the file and do not sync it to a public location.
- Do not share the URL printed by `pandora ui`; it contains the session token. Stop the UI when you are done.
- Keep Pandora updated.
