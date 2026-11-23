## What and why

<!-- What changed, and which problem it solves. Link the issue when there is one. -->

## How to test

<!-- Steps a reviewer can follow, or the tests that prove it. -->

## Checklist

- [ ] The behavior has a test at the lowest layer that proves it, or the change is docs only.
- [ ] `go test ./...` passes.
- [ ] `go vet ./...` passes and `gofmt -l .` prints nothing.
- [ ] If I changed `ui/`, I ran `bun run check`, `bun run test:unit -- --run` and `bun run build`, and committed the regenerated UI build.
- [ ] I updated the docs for any command, flag, path or behavior change.
- [ ] No secret, token or private URL is in the diff or in a log.
- [ ] The pull request title is a Conventional Commit.
