# Contributing

Thanks for helping. Pandora is a small project and every clear bug report, doc fix and test helps.

## Ways to help

- Report a bug with the issue form, including your version, OS and the output of `pandora doctor` with secrets removed.
- Improve the docs when a step is missing or a command changed.
- Add a test for a behavior that is not covered.
- Add support for a source or an AI client in the place that already owns that rule.
- Answer questions and review pull requests.

## Development setup

You need Go 1.27 or newer, git, and bun 1.4 or newer if you work on the UI.

```sh
git clone https://github.com/PierreOliveira1/pandora.git
cd pandora
go build ./cmd/pandora
```

Keep a development config apart from your daily one:

```sh
export PANDORA_HOME=/tmp/pandora-dev
export PANDORA_SECRETS=file
./pandora list
```

`PANDORA_SECRETS=file` stores secrets in a `0600` JSON file under `PANDORA_HOME/config`, so development never touches your keychain. Remove the directory when you want a clean start.

Read [AGENTS.md](AGENTS.md) before changing code. It defines the layers, the no-secrets rule and the concurrency rules.

## Running tests

Go:

```sh
go test ./...
go vet ./...
gofmt -l .
```

`gofmt -l .` must print nothing. During development, you can run only the package you changed:

```sh
go test ./internal/<package>/...
```

UI, from `ui/`. Component tests run in a real Chromium, so install it once:

```sh
bun install
bunx playwright install chromium
bun run check
bun run test:unit -- --run
bun run build
```

`bun run build` regenerates `internal/webui/dist`, which is embedded in the Go binary. Commit that build output together with the UI change; CI rebuilds it and fails when it differs.

Format the UI with:

```sh
bun run format
bunx prettier --check .
```

Tests are named after the behavior they prove. Check outcomes: returned values, saved state, HTTP responses, what the user sees. Do not check internal calls unless the call is the effect outside the system.

## UI development

The UI can run without a backend, using in-memory sample data:

```sh
cd ui
bun install
VITE_MOCK=1 bun run dev
```

Check every UI change visually at desktop width and at 390 px, in light and dark mode.

## Code conventions

- Follow [AGENTS.md](AGENTS.md): layers, no global state, no code comments unless a line is misleading, errors that say what to do next, context first, user-facing text in English.
- Do not put secrets in the config file or in logs.
- Do not create generic `utils`, `helpers` or `common` packages.
- Keep config writes locked and atomic.
- Update the docs when a command, flag, path or behavior changes.

## Commits

Use Conventional Commits:

- `feat: add FreeBSD builds`
- `fix: keep the daemon alive while a call is running`
- `docs: explain the second account login`
- `test: cover a denied call`
- `chore: update dependencies`
- `refactor: split the tool cache`

The release changelog is built from these messages. `feat` and `fix` appear in their own sections; `refactor`, `perf` and other types go under other changes; `docs`, `test` and `chore` are left out of the changelog.

## Pull requests

A pull request should explain what changed and why, and how you tested it. Fill in the pull request template and keep the title in Conventional Commit style.

Before you open it:

- [ ] The behavior has a test at the lowest layer that proves it, or the change is docs only.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` passes and `gofmt -l .` prints nothing.
- [ ] If you changed `ui/`, you ran `bun run check`, `bun run test:unit -- --run`, `bun run build` and committed `internal/webui/dist`.
- [ ] If you changed a command, flag, path or environment variable, the docs and `--help` agree.
- [ ] No secret, token or private URL is in the diff or in a log.
- [ ] The title is a Conventional Commit.

## Releases

Releases are published by GitHub Actions. The release workflow calls `ci.yml` as a reusable workflow and runs the whole CI before GoReleaser: `gofmt`, `go vet` and `go test` on Linux and macOS; build and vet on Windows; the UI check, prettier, unit tests and build, including the check that `internal/webui/dist` is up to date; and a GoReleaser snapshot of every OS and architecture. Only when all of it passes does GoReleaser publish a GitHub Release with the binaries, `checksums.txt` and a changelog grouped from the commits. A failure publishes nothing.

The simplest way is to create and push a `v*` tag on `main`:

```sh
git tag v0.2.0
git push origin v0.2.0
```

Creating the tag or the release from the GitHub interface starts the same workflow.

You can also run it from **Actions > Release > Run workflow** on `main`. Choose `patch`, `minor` or `major`, or type an exact version. The workflow creates the `vX.Y.Z` tag on `main` and then runs the same checks and publish.

Builds cover Linux, macOS, Windows and FreeBSD on amd64 and arm64. Windows support is experimental.

Before you release, commit the UI build: after any change under `ui/`, run `bun run build` and commit `internal/webui/dist`. CI fails when the committed build differs from the UI source.

## License

Pandora is MIT. By contributing, you agree that your contribution is licensed under the same terms: inbound equals outbound.
