# CLAUDE.md

Guidelines for working on this Go project.

## Project

SSF receiver that transforms and forwards events to various sinks.

## Go conventions

- Target the latest stable Go version.
- Use standard library packages before reaching for third-party ones. Check `net/http`, `encoding/json`, `log/slog`, `context`, `sync`, `errors`, etc.
- Use `log/slog` for structured logging — not `fmt.Println`, `log.Printf`, or third-party loggers.
- Use `errors.Is` / `errors.As` for error inspection. Wrap errors with `fmt.Errorf("...: %w", err)`.
- Prefer `context.Context` threading through call chains over global state.
- Use table-driven tests with `t.Run` subtests. Prefer `testing` from stdlib; reach for `github.com/stretchr/testify` only when it meaningfully reduces boilerplate.

## Tooling

- **Format:** `gofmt` / `goimports` — code must be formatted before committing.
- **Lint:** `go vet ./...` is the baseline. Use `golangci-lint` for broader checks if configured.
- **Test:** `go test ./...` — use `-race` flag when testing concurrent code.
- **Build:** `go build ./...`
- **Dependencies:** `go get` / `go mod tidy` — keep `go.sum` committed. Remove unused deps promptly.
- Avoid build scripts or Makefiles for tasks that `go` commands handle natively.

## Code style

- Keep functions small and focused. If a function needs a comment to explain what it does, consider renaming or splitting it.
- Avoid unnecessary abstractions — don't create interfaces for types that have only one implementation.
- Return early on errors rather than nesting happy-path logic.
- Unexported types and functions are the default; export only what external packages need.
- Avoid `init()` functions.

## Project structure

Follow standard Go project layout:
- `cmd/<name>/main.go` — entry points
- Internal packages under `internal/`
- Avoid `pkg/` unless sharing across multiple binaries

## Standards reference

The full text of the CAEP and SSF specifications are located in `docs/openid-caep-1_0-final.txt` and `openid-sharedsignals-framework-1_0-final.txt` respectively. Reference them to ensure compliance with each standard.

Use the official reference SSF receiver library: https://github.com/SGNL-ai/caep.dev/tree/main/ssfreceiver
