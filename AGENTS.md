# AGENTS.md — TianGong

> Guidelines for AI coding agents operating in this repository.
> Update this file as the project evolves.

---

## Project Overview

**TianGong** (天工 — "Divine Workmanship") is a multi-channel AI agent platform
written in Go. It provides a server (`tiangong`) and a CLI (`tg`) for managing
AI-powered conversational agents across multiple messaging channels.

- **Language**: Go 1.24
- **Framework**: Cobra (CLI), stdlib `net/http` (server)
- **Package Manager**: Go modules (`go.mod`)
- **Database**: PostgreSQL with sqlc (pgx/v5) + golang-migrate
- **Linter**: golangci-lint v1.64+

---

## Build / Run / Test Commands

```bash
# Build — produces bin/tiangong and bin/tg
make build

# Run server
./bin/tiangong

# Run CLI
./bin/tg version

# Lint — runs golangci-lint
make lint

# Vet — runs go vet
make vet

# Test (all)
make test

# Test (single package)
go test -v ./internal/bus/...

# Test (single test by name)
go test -v -run TestPublishAndReceive ./internal/bus/...

# Test with race detector
go test -race ./internal/bus/...

# Clean build artifacts
make clean

# Docker build
docker build -t tiangong .

# Docker Compose (app + postgres)
docker compose up
```

---

## Code Style Guidelines

### General Principles

- Write clear, readable code. Favor explicitness over cleverness.
- Keep functions short and focused — one responsibility per function.
- Name things descriptively: `GetUserByID` not `Get`, `IsValid` not `Check`.
- No dead code. Remove unused imports, variables, and functions.
- No commented-out code in committed files.
- Use `gofmt` / `goimports` for formatting — do not fight the formatter.

### Formatting

- All Go code must be formatted with `gofmt` (enforced by golangci-lint).
- Use `goimports` for import grouping and sorting.
- Max line length: no hard limit in Go, but keep lines readable (~120 chars).

### Imports

- Group imports in three blocks separated by blank lines:
  1. Standard library (`fmt`, `context`, `net/http`)
  2. External dependencies (`github.com/spf13/cobra`)
  3. Internal packages (`github.com/PhantomMatthew/TianGong/internal/bus`)
- Sort alphabetically within each group.
- No unused imports (enforced by compiler).

### Types & Type Safety

- Use concrete types. Avoid `interface{}` / `any` unless genuinely needed (e.g., event payloads).
- Never suppress linter warnings with `//nolint` without a justification comment.
- Define explicit types for function parameters and return values.
- Use custom types for domain concepts (e.g., `type EventType string`).

### Error Handling

- Handle all errors explicitly. No `_ = err` unless justified with a comment.
- Use `fmt.Errorf("context: %w", err)` to wrap errors with context.
- Log errors with sufficient context (what operation failed, with what input).
- Return errors to callers — don't swallow them silently.
- Use `errors.Is()` and `errors.As()` for error checking, not string matching.

### Naming Conventions

- **Files**: lowercase, underscores for multi-word (`event_bus.go`, `bus_test.go`).
- **Exported** (public): PascalCase — `Publish`, `EventType`, `NewBus`.
- **Unexported** (private): camelCase — `subscribers`, `closed`, `mu`.
- **Constants**: PascalCase for exported (`EventMessageReceived`), camelCase for unexported.
- **Interfaces**: Named by behavior, not by "I" prefix — `Reader`, `Handler`, not `IReader`.
- **Structs**: Noun or noun phrase — `Bus`, `Subscription`, `Event`.
- **Test functions**: `TestPublishAndReceive`, `TestCloseStopsDelivery`.
- **Boolean variables**: prefix with `is`, `has`, `should`, `can`.

### Testing

- Every new feature or bugfix should include tests.
- Test names should describe the behavior: `TestReturnsErrorWhenInputIsEmpty`.
- Use table-driven tests where appropriate.
- Use `testify/assert` for assertions.
- Mock external dependencies, not internal logic.
- Tests must be deterministic — no flaky tests.
- Run `go test -race ./...` to catch race conditions.

### Git & Commits

- Commit messages: imperative mood, concise (`Add user auth endpoint`, not `Added stuff`).
- Use conventional commits: `feat:`, `fix:`, `chore:`, `docs:`, `test:`.
- One logical change per commit.
- Do not commit secrets, credentials, or `.env` files.
- Do not commit generated files (build artifacts, `bin/`).

### Documentation

- Exported symbols must have doc comments (enforced by golangci-lint).
- Doc comments start with the symbol name: `// Bus is a publish-subscribe event bus.`
- Complex logic should have inline comments explaining *why*, not *what*.
- Keep README.md updated with setup and usage instructions.

---

## Architecture Notes

```
TianGong/
├── AGENTS.md                           # Agent guidelines (this file)
├── README.md                           # Project documentation
├── go.mod                              # Go module definition
├── go.sum                              # Dependency checksums
├── .gitignore                          # Git ignore rules
├── .golangci.yml                       # Linter configuration
├── sqlc.yaml                           # sqlc code generation config
├── Makefile                            # Build/test/lint targets
├── Dockerfile                          # Multi-stage Go build
├── docker-compose.yml                  # App + PostgreSQL services
├── cmd/
│   ├── tiangong/
│   │   └── main.go                     # Server binary entry point (cobra)
│   └── tg/
│       └── main.go                     # CLI binary entry point (cobra)
└── internal/
    ├── bus/                            # Event bus (pub/sub, channels)
    │   ├── bus.go                      # Bus implementation
    │   ├── events.go                   # Event type constants
    │   └── bus_test.go                 # Bus tests
    ├── agent/                          # AI agent orchestration
    ├── provider/                       # LLM provider adapters
    ├── tool/                           # Tool interface and registry
    ├── channel/                        # Messaging channel adapters
    ├── session/                        # Chat session management
    ├── memory/                         # Conversation memory/context
    ├── media/                          # Media processing
    ├── gateway/                        # HTTP/WebSocket gateway
    ├── config/                         # Configuration loading
    ├── storage/                        # Database layer (sqlc)
    │   ├── migrations/                 # SQL migration files
    │   └── queries/                    # sqlc query files
    ├── plugin/                         # Plugin system
    ├── mcp/                            # Model Context Protocol
    ├── skill/                          # Agent skills
    ├── voice/                          # Voice I/O
    ├── canvas/                         # Rich content rendering
    ├── security/                       # Auth and permissions
    ├── scheduler/                      # Scheduled tasks
    ├── i18n/                           # Internationalization
    └── hooks/                          # Lifecycle hooks
```

### Key Patterns

- **`cmd/`**: Entry points only — minimal code, delegate to `internal/`.
- **`internal/`**: All business logic — not importable by external packages.
- **No `pkg/` directory**: Everything is internal by design.
- **Event bus**: Native Go channels with `sync.RWMutex`, buffer size 64.
- **Database**: PostgreSQL accessed via sqlc-generated code (pgx/v5 driver).

---

## Agent-Specific Instructions

### Do

- Read existing code before writing new code.
- Follow existing patterns in the codebase (check `internal/bus/` for reference).
- Run `make lint` and `make test` after making changes.
- Fix only what you are asked to fix — minimal changes.
- Verify your changes compile with `go build ./...` before reporting completion.
- Use `lsp_find_references` before renaming or removing exported symbols.

### Do Not

- Do not suppress linter warnings with `//nolint` without justification.
- Do not add dependencies without justification.
- Do not refactor while fixing bugs (separate concerns).
- Do not commit unless explicitly asked.
- Do not leave code in a broken state.
- Do not create documentation files unless asked.
- Do not use `TODO` or `FIXME` without a tracking issue.

### When Unsure

- Check for existing patterns in the codebase first (especially `internal/bus/`).
- If no pattern exists, follow Go community conventions (Effective Go, Go Code Review Comments).
- If still unclear, ask rather than guess.

---

## External Rules

No external AI coding rules files detected. Update this section when added.

---

## Changelog

- **2026-03-08**: Updated with Go 1.24 conventions, build commands, and project architecture.
- **2026-03-08**: Initial AGENTS.md created (placeholder for new project).
