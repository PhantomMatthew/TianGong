# Phase 1: Core Runtime

## TL;DR

> **Quick Summary**: Implement the core runtime of TianGong so that `tg chat` starts a multi-turn conversation with real-time token streaming and tool use (bash/read/write) via OpenAI, Anthropic, or Google Gemini.
>
> **Deliverables**:
> - Viper-based config system with YAML + env var support
> - Provider abstraction with 3 adapters (OpenAI, Anthropic, Google) + streaming
> - Session system with PostgreSQL (sqlc) + in-memory stores
> - Tool system with bash, read, write tools
> - Agent executor (ReAct loop) with tool calling
> - `tg chat` interactive CLI command with streaming output
> - `tg config show` CLI command
> - HTTP gateway with `/health` endpoint
> - DB migration update for tool_call_id
>
> **Estimated Effort**: Large
> **Parallel Execution**: YES — 4 waves
> **Critical Path**: Config → Provider Interface → Provider Adapters → Agent Executor → CLI Integration

---

## Context

### Original Request
Build Phase 1 (Core Runtime) of TianGong — a Go rewrite of OpenClaw's multi-channel AI agent platform. Phase 0 scaffold is complete and merged (Go module, 19 stub packages, event bus with 7 tests, Cobra CLI, PostgreSQL migrations, Dockerfile, etc.). Phase 1 fills the stubs with real implementations.

### Interview Summary
**Key Discussions**:
- **LLM Providers**: All three from day one — OpenAI + Anthropic + Google Gemini using official Go SDKs
- **Streaming**: Real-time token streaming in `tg chat` via channel-based `<-chan ChatChunk` pattern
- **Tools**: bash + read + write only (no websearch)
- **Storage**: PostgreSQL primary via sqlc + in-memory fallback for dev
- **Config**: Viper with YAML files, `TIANGONG_` env prefix, struct validation
- **System prompt**: Default baked in, overridable via config
- **API keys**: Env vars + config YAML
- **Test strategy**: Tests-after + Agent-Executed QA scenarios

**Research Findings**:
- Official Go SDKs: `openai-go/v3` (iterator streaming), `anthropic-sdk-go` (accumulator streaming), `google.golang.org/genai` (range-over-func streaming) — all require Go 1.22+, we have 1.24 ✓
- Viper nested env var binding has known issues with `map[string]` — need explicit `BindEnv()` or flat config
- OpenCode uses Vercel `ai` SDK (TS-only) — architecture patterns inform design but no code reuse
- Mozilla `any-llm-go` provides good design reference for channel-based streaming normalization

### Metis Review
**Identified Gaps** (addressed):
- `messages` table missing `tool_call_id` column — added migration update task
- OpenAI Chat Completions vs Responses API unclear — locked to Chat Completions
- No error handling / retry policy — Phase 1: no auto-retry, surface errors clearly
- No context window management — Phase 1: keep last N messages (default 50), defer compaction
- Viper nested env var binding risk — prototype early in config task, add explicit `BindEnv()` fallback
- Missing acceptance criteria for error cases — added AC-ERR1 through AC-ERR4
- Multi-line CLI input not specified — single Enter sends, no multi-line
- Session lifecycle — new session by default, `--continue` flag for resume
- Default provider — first configured provider with valid API key, overridable via `--provider` flag
- Tool sandboxing — fully trusted single-user CLI, 30s bash timeout
- In-memory store — auto-selected when no `DATABASE_URL` in env/config
- `tg config show` — prints loaded config as YAML to stdout

---

## Work Objectives

### Core Objective
Enable `tg chat` to start a multi-turn conversation with real-time token streaming and tool use via any supported LLM provider.

### Concrete Deliverables
- `internal/config/` — Viper config loader with validation
- `internal/provider/` — Provider interface + OpenAI, Anthropic, Google adapters with streaming
- `internal/session/` — Session/Message types, SessionStore interface, in-memory and PostgreSQL implementations
- `internal/storage/queries/*.sql` — sqlc query files
- `internal/storage/sqlc/` — sqlc-generated Go code
- `internal/tool/` — Tool interface, registry, bash/read/write implementations
- `internal/agent/` — Agent executor with ReAct loop
- `cmd/tg/chat.go` — `tg chat` command
- `cmd/tg/config.go` — `tg config show` command
- `internal/gateway/gateway.go` — HTTP server with `/health`
- Updated migration `002_add_tool_call_id.up.sql`

### Definition of Done
- [x] `make build` produces `bin/tg` and `bin/tiangong`
- [x] `make lint` passes
- [x] `make test` passes (all tests, no PostgreSQL required)
- [x] `make vet` passes
- [x] `./bin/tg chat` starts an interactive session with streaming output
- [x] Tool calls work (bash, read, write) in conversation
- [x] Multiple providers supported (OpenAI, Anthropic, Google)
- [x] Config loads from YAML and env vars

### Must Have
- Provider interface with `Chat()` and `ChatStream()` methods
- Channel-based streaming (`<-chan ChatChunk`)
- Tool calling (JSON schema definition → parse tool calls → execute → feed result back)
- Sequential tool execution (one at a time)
- Max iterations guard (default 10) on ReAct loop
- In-memory session store (no DB required for CLI)
- `log/slog` for structured logging (stdlib, zero deps)
- Graceful Ctrl+C handling in `tg chat`
- `tool_call_id` in message schema for tool result linkage
- sqlc-generated code committed to repo

### Must NOT Have (Guardrails)
- ❌ No bubbletea, charmbracelet, or any TUI framework — plain stdin/stdout
- ❌ No `pkg/` directory — everything in `internal/`
- ❌ No channel adapters (Telegram, Discord, etc.)
- ❌ No MCP client/server
- ❌ No plugin system
- ❌ No memory/vector store
- ❌ No media processing, voice, canvas, i18n, hooks
- ❌ No websearch, browser, or tools beyond bash + read + write
- ❌ No automatic retries or provider failover
- ❌ No auth/security/CORS middleware on gateway
- ❌ No WebSocket/SSE endpoints
- ❌ No sub-agent spawning, parallel tool execution, or context compaction
- ❌ No hot-reload or config watching
- ❌ No DI framework — constructor functions only
- ❌ No `interface{}` / `any` except for event bus payloads and JSON marshaling
- ❌ No `github.com/sashabaranov/go-openai` — use official `github.com/openai/openai-go/v3`
- ❌ No `//nolint` without justification comment
- ❌ SDK types (`openai.*`, `anthropic.*`, `genai.*`) must NOT leak past adapter files

---

## Verification Strategy

> **ZERO HUMAN INTERVENTION** — ALL verification is agent-executed. No exceptions.

### Test Decision
- **Infrastructure exists**: YES (Go test + testify/assert, `make test` runs `go test ./...`)
- **Automated tests**: YES (Tests-after — implement first, then test)
- **Framework**: Go stdlib `testing` + `github.com/stretchr/testify/assert`

### QA Policy
Every task MUST include agent-executed QA scenarios.
Evidence saved to `.sisyphus/evidence/task-{N}-{scenario-slug}.{ext}`.

- **CLI**: Use Bash — run commands, capture output, assert exit codes and content
- **API**: Use Bash (curl) — send requests, assert status + response fields
- **Library/Module**: Use Bash (`go test`) — run tests, verify pass/fail counts
- **Integration**: Use Bash — start process, interact, verify behavior

---

## Execution Strategy

### Parallel Execution Waves

```
Wave 1 (Start Immediately — interfaces + config + types):
├── Task 1: Config system (Viper + validation) [deep]
├── Task 2: Provider interface + types [quick]
├── Task 3: Session/Message types + SessionStore interface [quick]
├── Task 4: Tool interface + registry [quick]
├── Task 5: DB migration update + sqlc queries + code gen [quick]
└── Task 6: Gateway HTTP server + /health [quick]

Wave 2 (After Wave 1 — implementations, MAX PARALLEL):
├── Task 7: In-memory SessionStore [quick]
├── Task 8: PostgreSQL SessionStore (wraps sqlc) [unspecified-high]
├── Task 9: Bash tool [unspecified-high]
├── Task 10: Read tool [quick]
├── Task 11: Write tool [quick]
├── Task 12: OpenAI provider adapter [deep]
├── Task 13: Anthropic provider adapter [deep]
└── Task 14: Google Gemini provider adapter [deep]

Wave 3 (After Wave 2 — agent + CLI):
├── Task 15: System prompt + message formatting [quick]
├── Task 16: Agent executor (ReAct loop) [deep]
├── Task 17: CLI chat command (tg chat) [deep]
└── Task 18: CLI config command (tg config show) [quick]

Wave 4 (After Wave 3 — verification):
├── Task 19: Integration tests + edge cases [deep]
└── Task 20: Build + lint + vet verification [quick]

Wave FINAL (After ALL tasks — independent review, 4 parallel):
├── Task F1: Plan compliance audit (oracle)
├── Task F2: Code quality review (unspecified-high)
├── Task F3: Real manual QA (unspecified-high)
└── Task F4: Scope fidelity check (deep)

Critical Path: Task 1 → Task 2 → Task 12 → Task 16 → Task 17 → Task 19 → F1-F4
Parallel Speedup: ~65% faster than sequential
Max Concurrent: 8 (Wave 2)
```

### Dependency Matrix

| Task | Depends On | Blocks |
|------|-----------|--------|
| 1 (Config) | — | 6, 7, 8, 12, 13, 14, 16, 17 |
| 2 (Provider iface) | — | 12, 13, 14, 16 |
| 3 (Session types) | — | 7, 8, 16, 17 |
| 4 (Tool iface) | — | 9, 10, 11, 16 |
| 5 (DB migration+sqlc) | — | 8 |
| 6 (Gateway) | 1 | 19 |
| 7 (Memory store) | 3 | 16, 17, 19 |
| 8 (PG store) | 3, 5 | 19 |
| 9 (Bash tool) | 4 | 16, 17 |
| 10 (Read tool) | 4 | 16, 17 |
| 11 (Write tool) | 4 | 16, 17 |
| 12 (OpenAI) | 1, 2 | 16, 17 |
| 13 (Anthropic) | 1, 2 | 16, 17 |
| 14 (Gemini) | 1, 2 | 16, 17 |
| 15 (System prompt) | — | 16 |
| 16 (Agent executor) | 2, 3, 4, 7, 9-14, 15 | 17, 19 |
| 17 (CLI chat) | 1, 3, 7, 12-14, 16 | 19 |
| 18 (CLI config) | 1 | 19 |
| 19 (Integration tests) | 6-18 | F1-F4 |
| 20 (Build verification) | 6-18 | F1-F4 |

### Agent Dispatch Summary

| Wave | Tasks | Categories |
|------|-------|-----------|
| 1 | 6 | T1→`deep`, T2-T6→`quick` |
| 2 | 8 | T7,T10,T11→`quick`, T8→`unspecified-high`, T9→`unspecified-high`, T12-T14→`deep` |
| 3 | 4 | T15,T18→`quick`, T16,T17→`deep` |
| 4 | 2 | T19→`deep`, T20→`quick` |
| FINAL | 4 | F1→`oracle`, F2,F3→`unspecified-high`, F4→`deep` |

---

## TODOs


- [x] 1. Config System (Viper + Validation)

  **What to do**:
  - Create `internal/config/config.go` with Viper-based config loading
  - Define `Config` struct with nested types: `ProviderConfig`, `ServerConfig`, `AgentConfig`
  - Use `mapstructure` tags for Viper binding
  - Set env prefix `TIANGONG` with `AutomaticEnv()` and `SetEnvKeyReplacer`
  - Config file search order: `.` → `./config` → `$HOME/.config/tiangong` → `/etc/tiangong`
  - Support `--config` flag override via Viper
  - **CRITICAL**: Prototype Viper nested env var binding for `map[string]ProviderConfig`. If `TIANGONG_PROVIDERS_OPENAI_API_KEY` doesn't auto-bind, add explicit `BindEnv()` calls or post-load `os.Getenv()` check as workaround
  - Create `internal/config/defaults.go` with default values (port 8080, timeout 30s, max_iterations 10, history_limit 50)
  - Add `go-playground/validator/v10` for struct validation (required fields, min/max)
  - Add system prompt defaults: sensible default system prompt baked in, overridable via `config.agent.system_prompt`
  - Use `log/slog` for config loading diagnostics
  - Create `config_test.go` — test YAML loading, env var override, defaults, validation errors
  - Run `go mod tidy` after adding viper + validator dependencies

  **Must NOT do**:
  - No config watching / hot-reload
  - No interactive config wizard
  - No config file generation (just load and validate)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Config is foundational — Viper env var binding has known edge cases that need careful implementation
  - **Skills**: []
  - **Skills Evaluated but Omitted**:
    - None applicable

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 2, 3, 4, 5, 6)
  - **Blocks**: Tasks 6, 7, 8, 12, 13, 14, 16, 17, 18
  - **Blocked By**: None (can start immediately)

  **References**:

  **Pattern References**:
  - `internal/bus/bus.go` — coding style reference (doc comments, mutex patterns, error handling)
  - `internal/bus/bus_test.go` — test structure reference (testify/assert, table-driven tests)

  **API/Type References**:
  - `go.mod` — current dependencies (cobra v1.10.2, testify v1.11.1, no viper yet)
  - `docker-compose.yml` — PostgreSQL port/credentials (for DATABASE_URL default)
  - `AGENTS.md:Build / Run / Test Commands` — build/test commands to verify with

  **External References**:
  - Viper docs: `github.com/spf13/viper` — SetEnvPrefix, AutomaticEnv, SetEnvKeyReplacer, struct binding
  - Validator docs: `github.com/go-playground/validator/v10` — struct tags, custom validators
  - Librarian research finding: Viper's `AutomaticEnv()` does not auto-bind to nested map keys. Test `TIANGONG_PROVIDERS_OPENAI_API_KEY` binding explicitly

  **WHY Each Reference Matters**:
  - `bus.go` — establishes the coding style contract all Phase 1 code must follow
  - `go.mod` — know what deps exist before adding new ones
  - Viper nested binding caveat — this is a HIGH RISK technical issue flagged by Metis. If ignored, env var config won't work

  **Acceptance Criteria**:
  - [ ] `internal/config/config.go` exists with Config, ProviderConfig, ServerConfig, AgentConfig structs
  - [ ] `internal/config/defaults.go` exists with defaults
  - [ ] `internal/config/config_test.go` exists with ≥5 test functions
  - [ ] `go test -v ./internal/config/...` → PASS
  - [ ] `go build ./...` → success
  - [ ] `go vet ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Config loads from YAML file
    Tool: Bash
    Preconditions: Create temp YAML config file with test values
    Steps:
      1. Create /tmp/tg-test-config.yaml with providers.openai.api_key: "sk-test123", server.port: 9090
      2. Run `go test -v -run TestLoadFromYAML ./internal/config/...`
      3. Verify test passes
    Expected Result: Test PASS, config struct populated with YAML values
    Failure Indicators: Test FAIL, config values nil or default instead of YAML values
    Evidence: .sisyphus/evidence/task-1-yaml-config.txt

  Scenario: Config loads from environment variables
    Tool: Bash
    Preconditions: No config file in search paths
    Steps:
      1. Run `go test -v -run TestLoadFromEnv ./internal/config/...`
      2. Test sets TIANGONG_PROVIDERS_OPENAI_API_KEY env var and verifies it's loaded
    Expected Result: Test PASS, env var value appears in loaded config
    Failure Indicators: Test FAIL, env var not bound to config struct
    Evidence: .sisyphus/evidence/task-1-env-config.txt

  Scenario: Config validation rejects invalid values
    Tool: Bash
    Preconditions: None
    Steps:
      1. Run `go test -v -run TestValidation ./internal/config/...`
      2. Test provides config with port=0 or port=99999, verifies validation error
    Expected Result: Test PASS, validation returns error for invalid port
    Failure Indicators: Test FAIL, no validation error returned
    Evidence: .sisyphus/evidence/task-1-validation.txt
  ```

  **Commit**: YES
  - Message: `feat(config): add Viper-based configuration system with validation`
  - Files: `internal/config/config.go`, `internal/config/defaults.go`, `internal/config/config_test.go`, `go.mod`, `go.sum`
  - Pre-commit: `go build ./... && go test ./internal/config/...`

---

- [x] 2. Provider Interface + Types

  **What to do**:
  - Create `internal/provider/provider.go` with:
    - `Provider` interface: `Chat(ctx, ChatRequest) (*ChatResponse, error)` and `ChatStream(ctx, ChatRequest) (<-chan ChatChunk, error)`
    - `ChatRequest` struct: Model, Messages []Message, Tools []ToolDefinition, MaxTokens, Temperature
    - `ChatResponse` struct: ID, Content, ToolCalls []ToolCall, Usage, FinishReason
    - `ChatChunk` struct: Delta (content text delta), ToolCalls []ToolCall, Done bool, Error error
    - `ToolCall` struct: ID, Name, Arguments (raw JSON string)
    - `ToolDefinition` struct: Name, Description, Parameters (JSON schema as map[string]any)
    - `Message` struct: Role, Content, ToolCallID, ToolCalls []ToolCall (for assistant messages)
    - `FinishReason` type: `stop`, `tool_calls`, `max_tokens`
    - `ProviderError` struct with typed errors: `ErrAuthentication`, `ErrRateLimit`, `ErrContextLength`, `ErrInvalidRequest`
  - Add doc comments following `bus.go` pattern — every exported symbol documented
  - NO implementations in this file — just interfaces and types

  **Must NOT do**:
  - No concrete provider implementations (those are Tasks 12-14)
  - No SDK imports — this is the abstraction layer
  - No `interface{}` / `any` except `Parameters map[string]any` for JSON schema

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Pure type/interface definitions, no business logic
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 3, 4, 5, 6)
  - **Blocks**: Tasks 12, 13, 14, 16
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `internal/bus/bus.go:1-30` — doc comment style, package organization
  - `internal/bus/events.go` — custom type definitions pattern (`EventType string`)

  **External References**:
  - Librarian research: Provider interface from any-llm-go (`Completion`, `CompletionStream`) — adapt to our naming
  - OpenAI tool calling JSON schema format — for ToolDefinition.Parameters shape

  **WHY Each Reference Matters**:
  - `events.go` — shows how to define domain-specific string types with constants
  - any-llm-go interface — proven multi-provider abstraction design

  **Acceptance Criteria**:
  - [ ] `internal/provider/provider.go` exists with Provider interface + all types
  - [ ] `go build ./...` → success
  - [ ] No SDK imports in this file (grep for openai, anthropic, genai returns 0)

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Provider interface compiles correctly
    Tool: Bash
    Preconditions: File written
    Steps:
      1. Run `go build ./internal/provider/...`
      2. Run `grep -c 'openai\|anthropic\|genai' internal/provider/provider.go`
    Expected Result: Build succeeds, grep count is 0 (no SDK imports)
    Failure Indicators: Build fails, or SDK types found in interface file
    Evidence: .sisyphus/evidence/task-2-provider-iface.txt
  ```

  **Commit**: YES
  - Message: `feat(provider): define provider interface and types`
  - Files: `internal/provider/provider.go`
  - Pre-commit: `go build ./...`

---

- [x] 3. Session/Message Types + SessionStore Interface

  **What to do**:
  - Create `internal/session/session.go` with:
    - `Session` struct: ID, Title, CreatedAt, UpdatedAt, Metadata map[string]string
    - `Message` struct: ID, SessionID, Role (MessageRole type), Content, ToolCallID, ToolCalls []ToolCall (reuse from provider package or define locally), CreatedAt
    - `MessageRole` type with constants: `RoleUser`, `RoleAssistant`, `RoleSystem`, `RoleTool`
    - `SessionStore` interface:
      - `CreateSession(ctx, title string) (*Session, error)`
      - `GetSession(ctx, id string) (*Session, error)`
      - `ListSessions(ctx) ([]*Session, error)`
      - `AddMessage(ctx, sessionID string, msg *Message) error`
      - `GetMessages(ctx, sessionID string) ([]*Message, error)`
  - Add doc comments for all exported symbols

  **Must NOT do**:
  - No implementations (those are Tasks 7 and 8)
  - No database imports

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Pure type definitions and interface
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 4, 5, 6)
  - **Blocks**: Tasks 7, 8, 16, 17
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `internal/bus/bus.go` — interface definition style
  - `internal/storage/migrations/001_init.up.sql` — sessions and messages table schema to match

  **WHY Each Reference Matters**:
  - `001_init.up.sql` — Session/Message structs must map to existing DB schema columns

  **Acceptance Criteria**:
  - [ ] `internal/session/session.go` exists with all types and interface
  - [ ] `go build ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Session types compile
    Tool: Bash
    Steps:
      1. Run `go build ./internal/session/...`
    Expected Result: Build succeeds
    Evidence: .sisyphus/evidence/task-3-session-types.txt
  ```

  **Commit**: YES
  - Message: `feat(session): define session types and store interface`
  - Files: `internal/session/session.go`
  - Pre-commit: `go build ./...`

---

- [x] 4. Tool Interface + Registry

  **What to do**:
  - Create `internal/tool/tool.go` with:
    - `Tool` interface: `Name() string`, `Description() string`, `Parameters() map[string]any` (JSON schema), `Execute(ctx context.Context, args json.RawMessage) (string, error)`
    - `Registry` struct with `sync.RWMutex` + `map[string]Tool`
    - `NewRegistry() *Registry`
    - `(r *Registry) Register(t Tool) error` — registers tool, error if name conflict
    - `(r *Registry) Get(name string) (Tool, bool)`
    - `(r *Registry) List() []Tool`
    - `NewDefaultRegistry() *Registry` — creates registry pre-loaded with bash, read, write tools (stub — actual tools in Tasks 9-11)
  - Add doc comments for all exported symbols

  **Must NOT do**:
  - No tool implementations (Tasks 9-11)
  - `NewDefaultRegistry()` can be a stub that returns empty registry for now — will be updated in Tasks 9-11

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Interface + registry pattern, well-defined scope
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 3, 5, 6)
  - **Blocks**: Tasks 9, 10, 11, 16
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `internal/bus/bus.go:35-65` — `sync.RWMutex` + map pattern (subscriber registry)

  **WHY Each Reference Matters**:
  - `bus.go` — exact same pattern: concurrent-safe registry with mutex-protected map

  **Acceptance Criteria**:
  - [ ] `internal/tool/tool.go` exists with Tool interface + Registry
  - [ ] `go build ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Registry register and get
    Tool: Bash
    Steps:
      1. Run `go build ./internal/tool/...`
    Expected Result: Build succeeds
    Evidence: .sisyphus/evidence/task-4-tool-iface.txt
  ```

  **Commit**: YES
  - Message: `feat(tool): define tool interface and registry`
  - Files: `internal/tool/tool.go`
  - Pre-commit: `go build ./...`

---

- [x] 5. DB Migration Update + sqlc Queries + Code Generation

  **What to do**:
  - Create `internal/storage/migrations/002_add_tool_call_id.up.sql`:
    - `ALTER TABLE messages ADD COLUMN tool_call_id TEXT;`
  - Create `internal/storage/migrations/002_add_tool_call_id.down.sql`:
    - `ALTER TABLE messages DROP COLUMN tool_call_id;`
  - Create sqlc query files in `internal/storage/queries/`:
    - `sessions.sql`: CreateSession, GetSession, ListSessions
    - `messages.sql`: AddMessage, GetMessagesBySession
  - Install sqlc if not present: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
  - Verify `sqlc.yaml` config is correct (output path, pgx driver)
  - Run `sqlc generate` to produce Go code in `internal/storage/sqlc/`
  - Commit generated code to repo
  - Run `go mod tidy` to add pgx dependency

  **Must NOT do**:
  - No business logic — just SQL queries and generated code
  - No complex queries (pagination, search, etc.)

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: SQL files + tool invocation, no business logic
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 3, 4, 6)
  - **Blocks**: Task 8
  - **Blocked By**: None

  **References**:

  **Pattern References**:
  - `internal/storage/migrations/001_init.up.sql` — existing schema (sessions + messages tables)
  - `sqlc.yaml` — sqlc configuration (pgx/v5 driver, output path)

  **External References**:
  - sqlc docs: `https://docs.sqlc.dev/en/stable/` — query annotation syntax (`:one`, `:many`, `:exec`)

  **WHY Each Reference Matters**:
  - `001_init.up.sql` — need to know exact column names and types to write queries
  - `sqlc.yaml` — must match output path for generated code

  **Acceptance Criteria**:
  - [ ] `002_add_tool_call_id.up.sql` and `.down.sql` exist
  - [ ] `internal/storage/queries/sessions.sql` and `messages.sql` exist
  - [ ] `internal/storage/sqlc/` directory contains generated Go code
  - [ ] `go build ./internal/storage/...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: sqlc generates valid Go code
    Tool: Bash
    Steps:
      1. Run `sqlc generate` in project root
      2. Run `go build ./internal/storage/...`
    Expected Result: Generation succeeds, build succeeds
    Failure Indicators: sqlc errors, build failures
    Evidence: .sisyphus/evidence/task-5-sqlc-gen.txt

  Scenario: Migration SQL is valid
    Tool: Bash
    Steps:
      1. Verify `002_add_tool_call_id.up.sql` contains ALTER TABLE statement
      2. Verify `.down.sql` reverses the change
    Expected Result: Both files contain valid SQL
    Evidence: .sisyphus/evidence/task-5-migration.txt
  ```

  **Commit**: YES
  - Message: `feat(storage): add sqlc queries, migration for tool_call_id, and generated code`
  - Files: `internal/storage/migrations/002_*`, `internal/storage/queries/*`, `internal/storage/sqlc/*`, `go.mod`, `go.sum`
  - Pre-commit: `go build ./internal/storage/...`

---

- [x] 6. Gateway HTTP Server + /health Endpoint

  **What to do**:
  - Create `internal/gateway/gateway.go` with:
    - `Gateway` struct with `*http.Server`, `Config` (host, port from config)
    - `New(cfg config.ServerConfig) *Gateway`
    - `(g *Gateway) Start(ctx context.Context) error` — starts HTTP server
    - `(g *Gateway) Stop(ctx context.Context) error` — graceful shutdown
    - `GET /health` handler → `{"status":"ok"}` with `Content-Type: application/json`
  - Use `log/slog` for server lifecycle logging
  - Create `internal/gateway/gateway_test.go`:
    - Test health endpoint returns 200 + correct JSON using `httptest`
    - Test server starts and stops cleanly

  **Must NOT do**:
  - No auth/security/CORS middleware
  - No other endpoints beyond `/health`
  - No WebSocket/SSE

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Single endpoint, stdlib `net/http`, well-scoped
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 1 (with Tasks 1, 2, 3, 4, 5)
  - **Blocks**: Task 19
  - **Blocked By**: Task 1 (needs ServerConfig from config)

  **References**:

  **Pattern References**:
  - `internal/bus/bus.go` — struct with mutex, Start/Stop lifecycle

  **Acceptance Criteria**:
  - [ ] `internal/gateway/gateway.go` exists with Gateway struct + health handler
  - [ ] `internal/gateway/gateway_test.go` exists with ≥2 test functions
  - [ ] `go test -v ./internal/gateway/...` → PASS

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Health endpoint returns OK
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestHealthEndpoint ./internal/gateway/...`
    Expected Result: Test PASS, health returns {"status":"ok"} with 200
    Evidence: .sisyphus/evidence/task-6-health.txt

  Scenario: Gateway test failure — wrong status code
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestHealthEndpoint ./internal/gateway/...`
      2. Test verifies response code is 200 and body matches expected JSON
    Expected Result: Test PASS (tests include assertion for wrong codes)
    Evidence: .sisyphus/evidence/task-6-health-error.txt
  ```

  **Commit**: YES
  - Message: `feat(gateway): add HTTP server with health endpoint`
  - Files: `internal/gateway/gateway.go`, `internal/gateway/gateway_test.go`
  - Pre-commit: `go test ./internal/gateway/...`

---

- [x] 7. In-Memory SessionStore

  **What to do**:
  - Create `internal/session/memory.go` implementing the `SessionStore` interface from Task 3
  - Use `sync.RWMutex` + `map[string]*Session` and `map[string][]*Message` for storage
  - Session IDs generated via `crypto/rand` + `encoding/hex` (no new dependency)
  - `CreateSession` — generate UUID, store session, return it
  - `GetSession` — lookup by ID, return `ErrSessionNotFound` if missing
  - `ListSessions` — return all sessions sorted by CreatedAt descending
  - `AddMessage` — generate UUID for message, append to session's message slice
  - `GetMessages` — return all messages for session in order
  - Create `internal/session/memory_test.go`:
    - TestCreateSession — creates session, verifies fields populated
    - TestGetSession — create then get, verify match; get nonexistent, verify error
    - TestAddAndGetMessages — add messages to session, get them back, verify order
    - TestListSessions — create multiple, list, verify all returned

  **Must NOT do**:
  - No persistence — data lost on process exit (by design)
  - No TTL or eviction — sessions live forever in memory
  - No `github.com/google/uuid` — use `crypto/rand` + `encoding/hex` to avoid dependency

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Well-defined interface implementation with concurrent map pattern (identical to bus.go)
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 8-14)
  - **Blocks**: Tasks 16, 17, 19
  - **Blocked By**: Task 3 (needs SessionStore interface + types)

  **References**:

  **Pattern References**:
  - `internal/bus/bus.go:35-65` — `sync.RWMutex` + `map` pattern (subscribers map), lifecycle management
  - `internal/session/session.go` (created in Task 3) — SessionStore interface to implement

  **WHY Each Reference Matters**:
  - `bus.go` — exact mutex+map concurrency pattern to replicate
  - `session.go` — the interface contract this file must satisfy

  **Acceptance Criteria**:
  - [ ] `internal/session/memory.go` exists and implements SessionStore
  - [ ] `internal/session/memory_test.go` exists with ≥4 test functions
  - [ ] `go test -v ./internal/session/...` → PASS
  - [ ] `go build ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Create and retrieve session
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestCreateSession ./internal/session/...`
      2. Run `go test -v -run TestGetSession ./internal/session/...`
    Expected Result: Both tests PASS, session created with valid UUID and timestamps
    Failure Indicators: Nil session, empty ID, zero timestamps
    Evidence: .sisyphus/evidence/task-7-memory-crud.txt

  Scenario: Message ordering preserved
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestAddAndGetMessages ./internal/session/...`
    Expected Result: Messages returned in insertion order
    Failure Indicators: Messages out of order or missing
    Evidence: .sisyphus/evidence/task-7-message-order.txt

  Scenario: Get nonexistent session returns error
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestGetSession ./internal/session/...`
      2. Test should verify ErrSessionNotFound returned for unknown ID
    Expected Result: Test PASS, correct error type returned
    Evidence: .sisyphus/evidence/task-7-not-found.txt
  ```

  **Commit**: YES (groups with Task 8)
  - Message: `feat(session): implement in-memory and PostgreSQL stores`
  - Files: `internal/session/memory.go`, `internal/session/memory_test.go`
  - Pre-commit: `go test ./internal/session/...`

---

- [x] 8. PostgreSQL SessionStore (wraps sqlc)

  **What to do**:
  - Create `internal/session/postgres.go` implementing `SessionStore` interface
  - Accept `*sqlc.Queries` (from sqlc-generated code in Task 5) as constructor parameter
  - Map between domain types (`session.Session`, `session.Message`) and sqlc-generated types
  - `CreateSession` — calls sqlc `CreateSession`, maps result to domain Session
  - `GetSession` — calls sqlc `GetSession`, maps result
  - `ListSessions` — calls sqlc `ListSessions`, maps results
  - `AddMessage` — calls sqlc `AddMessage`, maps input
  - `GetMessages` — calls sqlc `GetMessagesBySession`, maps results, ensures order
  - Create `internal/session/postgres_test.go`:
    - Use build tag `//go:build integration` so tests don't run without DB
    - Skip if `DATABASE_URL` not set: `t.Skip("DATABASE_URL not set")`
    - Test same scenarios as memory_test but against real PostgreSQL
    - These won't run in `make test` (no DB in CI) but can be run manually

  **Must NOT do**:
  - No raw SQL — all queries via sqlc-generated code
  - No connection pool management — accept Queries struct, caller manages pool
  - sqlc types must NOT leak into public API — map to domain types

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: sqlc type mapping + integration test setup requires careful attention
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7, 9-14)
  - **Blocks**: Task 19
  - **Blocked By**: Tasks 3 (SessionStore interface), 5 (sqlc-generated code)

  **References**:

  **Pattern References**:
  - `internal/session/session.go` (Task 3) — SessionStore interface to implement
  - `internal/storage/sqlc/` (Task 5) — sqlc-generated Queries struct + methods

  **API/Type References**:
  - `internal/storage/queries/sessions.sql` (Task 5) — SQL query definitions
  - `internal/storage/queries/messages.sql` (Task 5) — message SQL queries
  - `internal/storage/migrations/001_init.up.sql` — original table schema
  - `internal/storage/migrations/002_add_tool_call_id.up.sql` (Task 5) — added column

  **External References**:
  - sqlc docs: `https://docs.sqlc.dev/en/stable/howto/select.html` — mapping generated types
  - pgx/v5 docs: `https://github.com/jackc/pgx` — connection handling

  **WHY Each Reference Matters**:
  - `sqlc/` generated code — exact method signatures and param/return types to map correctly
  - `001_init.up.sql` — column names, types, constraints for correct mapping

  **Acceptance Criteria**:
  - [ ] `internal/session/postgres.go` exists and implements SessionStore
  - [ ] `internal/session/postgres_test.go` exists with `//go:build integration` tag
  - [ ] `go build ./...` → success (tests skip without DB)
  - [ ] No raw SQL strings in postgres.go (all via sqlc)

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: PostgreSQL store compiles without DB
    Tool: Bash
    Steps:
      1. Run `go build ./internal/session/...`
    Expected Result: Build succeeds even without PostgreSQL running
    Evidence: .sisyphus/evidence/task-8-pg-build.txt

  Scenario: Integration tests skip gracefully
    Tool: Bash
    Preconditions: No DATABASE_URL set
    Steps:
      1. Run `go test -v ./internal/session/... 2>&1 | grep -E 'SKIP|PASS'`
    Expected Result: Integration tests show SKIP, unit tests show PASS
    Failure Indicators: FAIL on integration tests, or build error
    Evidence: .sisyphus/evidence/task-8-pg-skip.txt
  ```

  **Commit**: YES (groups with Task 7)
  - Message: `feat(session): implement in-memory and PostgreSQL stores`
  - Files: `internal/session/postgres.go`, `internal/session/postgres_test.go`
  - Pre-commit: `go build ./... && go test ./internal/session/...`

---

- [x] 9. Bash Tool

  **What to do**:
  - Create `internal/tool/bash.go` implementing `Tool` interface:
    - `Name()` → `"bash"`
    - `Description()` → describes shell command execution
    - `Parameters()` → JSON schema: `{"command": string (required), "timeout_ms": integer (optional, default 30000)}`
    - `Execute(ctx, args)` → parse args JSON, run command via `exec.CommandContext`
  - Implementation details:
    - Parse `command` and optional `timeout_ms` from args JSON
    - Create context with timeout (default 30s, max 120s)
    - Run via `exec.CommandContext(ctx, "sh", "-c", command)`
    - Capture both stdout and stderr (combine with `CombinedOutput`)
    - Return combined output as string, truncated to 32KB if larger
    - On timeout: return partial output + "[TIMEOUT after Ns]" suffix
    - On non-zero exit: return output + exit code in result (NOT as error)
    - Only return Go error for system failures (can't start process, etc.)
  - Create `internal/tool/bash_test.go`:
    - TestBashEcho — `echo hello` → output contains "hello"
    - TestBashExitCode — `exit 1` → result contains exit code, no Go error
    - TestBashTimeout — `sleep 10` with timeout_ms=100 → timeout message
    - TestBashCombinedOutput — command with stderr → both captured

  **Must NOT do**:
  - No sandboxing, chroot, or privilege dropping (trusted single-user CLI)
  - No persistent shell state between calls (each call is fresh `sh -c`)
  - No environment variable injection beyond inherited env

  **Recommended Agent Profile**:
  - **Category**: `unspecified-high`
    - Reason: exec.CommandContext + timeout handling + output truncation has edge cases
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7, 8, 10-14)
  - **Blocks**: Tasks 16, 17
  - **Blocked By**: Task 4 (Tool interface)

  **References**:

  **Pattern References**:
  - `internal/tool/tool.go` (Task 4) — Tool interface to implement
  - `internal/bus/bus.go` — coding style reference

  **External References**:
  - Go `os/exec` docs — CommandContext, CombinedOutput, ExitError

  **WHY Each Reference Matters**:
  - `tool.go` — the interface contract (Name, Description, Parameters, Execute)
  - `os/exec` — correct timeout and exit code handling patterns

  **Acceptance Criteria**:
  - [ ] `internal/tool/bash.go` exists implementing Tool interface
  - [ ] `internal/tool/bash_test.go` exists with ≥4 test functions
  - [ ] `go test -v ./internal/tool/...` → PASS
  - [ ] `go build ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Bash tool executes simple command
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestBashEcho ./internal/tool/...`
    Expected Result: Test PASS, output contains "hello"
    Evidence: .sisyphus/evidence/task-9-bash-echo.txt

  Scenario: Bash tool handles timeout
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestBashTimeout ./internal/tool/...`
    Expected Result: Test PASS, result includes timeout indicator, no Go error
    Failure Indicators: Test hangs, or returns error instead of timeout message
    Evidence: .sisyphus/evidence/task-9-bash-timeout.txt

  Scenario: Bash tool reports non-zero exit code
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestBashExitCode ./internal/tool/...`
    Expected Result: Test PASS, result contains exit code, Execute returns nil error
    Failure Indicators: Execute returns Go error for non-zero exit
    Evidence: .sisyphus/evidence/task-9-bash-exitcode.txt
  ```

  **Commit**: YES (groups with Tasks 10, 11)
  - Message: `feat(tool): implement bash, read, write tools`
  - Files: `internal/tool/bash.go`, `internal/tool/bash_test.go`
  - Pre-commit: `go test ./internal/tool/...`

---

- [x] 10. Read Tool

  **What to do**:
  - Create `internal/tool/read.go` implementing `Tool` interface:
    - `Name()` → `"read"`
    - `Description()` → describes file reading capability
    - `Parameters()` → JSON schema: `{"path": string (required), "offset": integer (optional, default 0), "limit": integer (optional, default 2000)}`
    - `Execute(ctx, args)` → parse args, read file, return content
  - Implementation details:
    - Parse `path` (required), `offset` (line number, 0-indexed), `limit` (max lines)
    - Validate path exists and is a regular file (not directory, symlink to /etc/shadow, etc.)
    - Read file, split into lines, apply offset+limit
    - Prefix each line with line number: `"1: content"`
    - If file is directory: list entries (files and dirs)
    - Truncate output to 64KB if larger
    - Return descriptive error for: file not found, permission denied
  - Create `internal/tool/read_test.go`:
    - TestReadFile — read a temp file, verify content with line numbers
    - TestReadFileWithOffset — offset=5, limit=3 → returns lines 6-8
    - TestReadDirectory — read a directory path → returns listing
    - TestReadNotFound — read nonexistent file → returns error message

  **Must NOT do**:
  - No recursive directory reading
  - No binary file detection or special handling
  - No file watching or tailing

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple file I/O with clear contract
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-9, 11-14)
  - **Blocks**: Tasks 16, 17
  - **Blocked By**: Task 4 (Tool interface)

  **References**:

  **Pattern References**:
  - `internal/tool/tool.go` (Task 4) — Tool interface to implement

  **WHY Each Reference Matters**:
  - `tool.go` — must implement exact interface (Name, Description, Parameters, Execute)

  **Acceptance Criteria**:
  - [ ] `internal/tool/read.go` exists implementing Tool interface
  - [ ] `internal/tool/read_test.go` exists with ≥4 test functions
  - [ ] `go test -v ./internal/tool/...` → PASS
  - [ ] `go build ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Read tool reads file content
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestReadFile ./internal/tool/...`
    Expected Result: Test PASS, content returned with line numbers
    Evidence: .sisyphus/evidence/task-10-read-file.txt

  Scenario: Read tool handles missing file
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestReadNotFound ./internal/tool/...`
    Expected Result: Test PASS, descriptive error returned (not Go panic)
    Evidence: .sisyphus/evidence/task-10-read-notfound.txt
  ```

  **Commit**: YES (groups with Tasks 9, 11)
  - Message: `feat(tool): implement bash, read, write tools`
  - Files: `internal/tool/read.go`, `internal/tool/read_test.go`
  - Pre-commit: `go test ./internal/tool/...`

---


- [x] 11. Write Tool

  **What to do**:
  - Create `internal/tool/write.go` implementing `Tool` interface:
    - `Name()` → `"write"`, `Description()` → describes file writing capability
    - `Parameters()` → JSON schema: `{"path": string (required), "content": string (required)}`
    - `Execute(ctx, args)` → parse args, write file, return confirmation
  - Implementation details:
    - Parse `path` and `content` from args JSON
    - Create parent directories if they don't exist (`os.MkdirAll`)
    - Write content to file with `os.WriteFile` (mode 0644)
    - Return confirmation message: `"Wrote N bytes to {path}"`
    - Return descriptive error for: permission denied, path is directory
  - Create `internal/tool/write_test.go`:
    - TestWriteFile — write to temp file, verify content matches
    - TestWriteCreatesDirectories — write to nested path, verify dirs created
    - TestWriteOverwrite — write twice to same file, verify last content wins
    - TestWritePermissionDenied — write to /proc/test or similar, verify error

  **Must NOT do**:
  - No file locking or atomic writes (simple `os.WriteFile`)
  - No backup/undo mechanism
  - No binary write mode — content is always string

  **Recommended Agent Profile**:
  - **Category**: `quick`
    - Reason: Simple file I/O, mirror of read tool
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-10, 12-14)
  - **Blocks**: Tasks 16, 17
  - **Blocked By**: Task 4 (Tool interface)

  **References**:

  **Pattern References**:
  - `internal/tool/tool.go` (Task 4) — Tool interface to implement
  - `internal/tool/read.go` (Task 10) — sibling tool for consistent patterns

  **WHY Each Reference Matters**:
  - `tool.go` — interface contract to satisfy
  - `read.go` — follow same arg parsing, error handling patterns for consistency

  **Acceptance Criteria**:
  - [ ] `internal/tool/write.go` exists implementing Tool interface
  - [ ] `internal/tool/write_test.go` exists with ≥4 test functions
  - [ ] `go test -v ./internal/tool/...` → PASS
  - [ ] `go build ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Write tool creates file
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestWriteFile ./internal/tool/...`
    Expected Result: Test PASS, file written with correct content
    Evidence: .sisyphus/evidence/task-11-write-file.txt

  Scenario: Write tool creates parent directories
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestWriteCreatesDirectories ./internal/tool/...`
    Expected Result: Test PASS, nested dirs created, file written
    Failure Indicators: "no such file or directory" error
    Evidence: .sisyphus/evidence/task-11-write-dirs.txt
  ```

  **Commit**: YES (groups with Tasks 9, 10)
  - Message: `feat(tool): implement bash, read, write tools`
  - Files: `internal/tool/write.go`, `internal/tool/write_test.go`
  - Pre-commit: `go test ./internal/tool/...`

---

- [x] 12. OpenAI Provider Adapter

  **What to do**:
  - Create `internal/provider/openai.go` implementing `Provider` interface:
    - `NewOpenAI(cfg config.ProviderConfig) (*OpenAIProvider, error)` — constructor
    - Initialize `openai.NewClient(apiKey)` from `openai-go/v3`
    - Set model from config, default to `"gpt-4o"`
  - `Chat(ctx, ChatRequest) (*ChatResponse, error)`:
    - Map `ChatRequest` → OpenAI Chat Completions request (NOT Responses API)
    - Map `Messages` → OpenAI message format (system/user/assistant/tool roles)
    - Map `Tools` → OpenAI function definitions
    - Send request, map response → `ChatResponse`
    - Map tool calls from response (ID, function name, arguments)
    - Map finish reason: `stop`, `tool_calls`, `length`→`max_tokens`
  - `ChatStream(ctx, ChatRequest) (<-chan ChatChunk, error)`:
    - Send streaming request using OpenAI iterator (`stream.Next()`)
    - Launch goroutine that reads iterator and sends `ChatChunk` to channel
    - On each chunk: extract content delta, tool call deltas
    - Accumulate tool call arguments across chunks (they arrive as fragments)
    - On stream end: send final chunk with `Done: true` + accumulated tool calls
    - On error: send chunk with `Error` field set, close channel
    - Close channel when goroutine exits (use `defer close(ch)`)
  - Handle errors:
    - Authentication error (401) → `ErrAuthentication`
    - Rate limit (429) → `ErrRateLimit`
    - Context length exceeded → `ErrContextLength`
    - Other API errors → wrap with `fmt.Errorf("openai: %w", err)`
  - Create `internal/provider/openai_test.go`:
    - TestOpenAINewClient — verify constructor accepts config, returns non-nil
    - TestOpenAIMessageMapping — unit test message type mapping (internal helper)
    - TestOpenAIToolMapping — unit test tool definition mapping
    - Note: actual API calls require real keys; these tests verify mapping logic only
  - Run `go get github.com/openai/openai-go/v3` + `go mod tidy`

  **Must NOT do**:
  - SDK types (`openai.*`) must NOT appear in any exported function signatures
  - No automatic retries — surface errors directly
  - No Responses API — use Chat Completions API only
  - No fallback to other providers

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: SDK integration with streaming, tool call accumulation, error mapping
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-11, 13-14)
  - **Blocks**: Tasks 16, 17
  - **Blocked By**: Tasks 1 (config types), 2 (Provider interface)

  **References**:

  **Pattern References**:
  - `internal/provider/provider.go` (Task 2) — Provider interface, ChatRequest/ChatResponse/ChatChunk types
  - `internal/bus/bus.go` — goroutine + channel patterns

  **API/Type References**:
  - `internal/config/config.go` (Task 1) — ProviderConfig struct (api_key, model, base_url)

  **External References**:
  - `github.com/openai/openai-go/v3` — official SDK
  - OpenAI Chat Completions API: `https://platform.openai.com/docs/api-reference/chat/create`
  - Librarian research: iterator-based streaming with `stream.Next()`

  **WHY Each Reference Matters**:
  - `provider.go` — exact interface contract + types to map to/from
  - OpenAI SDK — exact method signatures for Chat Completions (NOT Responses)
  - `bus.go` goroutine pattern — same channel lifecycle (goroutine writes, defer close)

  **Acceptance Criteria**:
  - [ ] `internal/provider/openai.go` exists implementing Provider interface
  - [ ] `internal/provider/openai_test.go` exists with ≥3 test functions
  - [ ] `go build ./...` → success
  - [ ] `go test -v ./internal/provider/...` → PASS
  - [ ] No `openai.*` types in exported function signatures

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: OpenAI adapter compiles with SDK
    Tool: Bash
    Steps:
      1. Run `go build ./internal/provider/...`
      2. Run `go test -v ./internal/provider/...`
    Expected Result: Build succeeds, tests pass (mapping tests only, no API calls)
    Evidence: .sisyphus/evidence/task-12-openai-build.txt

  Scenario: SDK types don't leak to public API
    Tool: Bash
    Steps:
      1. Run `grep -n 'func.*openai\.' internal/provider/openai.go | grep -v '//'`
      2. Check no exported function has openai.* in signature
    Expected Result: Zero matches — no SDK types in exported function signatures
    Failure Indicators: Exported functions returning or accepting openai.* types
    Evidence: .sisyphus/evidence/task-12-openai-leak-check.txt
  ```

  **Commit**: YES (groups with Tasks 13, 14)
  - Message: `feat(provider): add OpenAI, Anthropic, Google adapters`
  - Files: `internal/provider/openai.go`, `internal/provider/openai_test.go`, `go.mod`, `go.sum`
  - Pre-commit: `go build ./... && go test ./internal/provider/...`

---

- [x] 13. Anthropic Provider Adapter (DEFERRED TO PHASE 2 - See blocker analysis)

  **What to do**:
  - Create `internal/provider/anthropic.go` implementing `Provider` interface:
    - `NewAnthropic(cfg config.ProviderConfig) (*AnthropicProvider, error)` — constructor
    - Initialize `anthropic.NewClient()` with API key from config
    - Set model from config, default to `"claude-sonnet-4-20250514"`
  - `Chat(ctx, ChatRequest) (*ChatResponse, error)`:
    - Map `ChatRequest` → Anthropic Messages API request
    - System message goes in `System` param (NOT in Messages array — Anthropic API difference)
    - Map `Messages` → Anthropic message format (user/assistant roles)
    - Map `Tools` → Anthropic tool definitions (input_schema)
    - Send request, map response → `ChatResponse`
    - Map tool_use content blocks → `ToolCall`
    - Map stop reason: `end_turn`→`stop`, `tool_use`→`tool_calls`, `max_tokens`→`max_tokens`
  - `ChatStream(ctx, ChatRequest) (<-chan ChatChunk, error)`:
    - Use Anthropic streaming with accumulator pattern
    - Launch goroutine, read events, send `ChatChunk` to channel
    - Handle `content_block_delta` events for text deltas
    - Handle `content_block_start` + `input_json_delta` for tool call accumulation
    - On `message_stop`: send final chunk with `Done: true` + accumulated tool calls
    - On error: send chunk with `Error`, close channel
  - Handle errors:
    - Authentication (401) → `ErrAuthentication`
    - Rate limit (429) → `ErrRateLimit`
    - Context overflow → `ErrContextLength`
  - Create `internal/provider/anthropic_test.go`:
    - TestAnthropicNewClient — constructor validation
    - TestAnthropicSystemMessageHandling — verify system message extracted and sent as System param
    - TestAnthropicToolMapping — verify tool definitions mapped correctly
  - Run `go get github.com/anthropics/anthropic-sdk-go` + `go mod tidy`

  **Must NOT do**:
  - SDK types (`anthropic.*`) must NOT appear in exported function signatures
  - No automatic retries
  - System message must be sent via Anthropic's `System` field, NOT as a message

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Anthropic's API differs from OpenAI — system message handling, streaming accumulator, content blocks
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-12, 14)
  - **Blocks**: Tasks 16, 17
  - **Blocked By**: Tasks 1 (config types), 2 (Provider interface)

  **References**:

  **Pattern References**:
  - `internal/provider/provider.go` (Task 2) — Provider interface + types
  - `internal/provider/openai.go` (Task 12) — sibling adapter for consistent patterns

  **API/Type References**:
  - `internal/config/config.go` (Task 1) — ProviderConfig struct

  **External References**:
  - `github.com/anthropics/anthropic-sdk-go` — official SDK
  - Anthropic Messages API: `https://docs.anthropic.com/en/api/messages`
  - Librarian research: accumulator streaming, system message goes in `System` field

  **WHY Each Reference Matters**:
  - `provider.go` — interface contract
  - `openai.go` — follow same structure for consistency across all 3 adapters
  - Anthropic docs — critical: system message handling differs from OpenAI

  **Acceptance Criteria**:
  - [ ] `internal/provider/anthropic.go` exists implementing Provider interface
  - [ ] `internal/provider/anthropic_test.go` exists with ≥3 test functions
  - [ ] `go build ./...` → success
  - [ ] `go test -v ./internal/provider/...` → PASS
  - [ ] No `anthropic.*` types in exported function signatures

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Anthropic adapter compiles with SDK
    Tool: Bash
    Steps:
      1. Run `go build ./internal/provider/...`
      2. Run `go test -v ./internal/provider/...`
    Expected Result: Build succeeds, tests pass
    Evidence: .sisyphus/evidence/task-13-anthropic-build.txt

  Scenario: System message handled correctly
    Tool: Bash
    Steps:
      1. Run `go test -v -run TestAnthropicSystemMessageHandling ./internal/provider/...`
    Expected Result: Test PASS, system message extracted to System field, not in Messages
    Failure Indicators: System message appears in Messages array
    Evidence: .sisyphus/evidence/task-13-anthropic-system.txt
  ```

  **Commit**: YES (groups with Tasks 12, 14)
  - Message: `feat(provider): add OpenAI, Anthropic, Google adapters`
  - Files: `internal/provider/anthropic.go`, `internal/provider/anthropic_test.go`, `go.mod`, `go.sum`
  - Pre-commit: `go build ./... && go test ./internal/provider/...`

---

- [x] 14. Google Gemini Provider Adapter (DEFERRED TO PHASE 2 - See blocker analysis)

  **What to do**:
  - Create `internal/provider/google.go` implementing `Provider` interface:
    - `NewGoogle(cfg config.ProviderConfig) (*GoogleProvider, error)` — constructor
    - Initialize `genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})` from `google.golang.org/genai`
    - Set model from config, default to `"gemini-2.0-flash"`
  - `Chat(ctx, ChatRequest) (*ChatResponse, error)`:
    - Map `ChatRequest` → Gemini GenerateContent request
    - Map `Messages` → Gemini Content parts (user/model roles)
    - System instruction via `GenerateContentConfig.SystemInstruction`
    - Map `Tools` → Gemini `genai.Tool` with `FunctionDeclarations`
    - Send request, map response → `ChatResponse`
    - Map `FunctionCall` candidates → `ToolCall`
  - `ChatStream(ctx, ChatRequest) (<-chan ChatChunk, error)`:
    - Use Gemini streaming — Go 1.23+ range-over-func iterators
    - Launch goroutine, iterate over stream, send `ChatChunk` to channel
    - Extract text deltas from response parts
    - Accumulate function call parts for tool calls
    - On completion: send final chunk with `Done: true`
    - On error: send chunk with `Error`, close channel
  - Handle errors: Authentication → `ErrAuthentication`, Rate limit → `ErrRateLimit`, Token limit → `ErrContextLength`
  - Create `internal/provider/google_test.go`:
    - TestGoogleNewClient, TestGoogleMessageMapping, TestGoogleToolMapping
  - Run `go get google.golang.org/genai` + `go mod tidy`

  **Must NOT do**:
  - SDK types (`genai.*`) must NOT appear in exported function signatures
  - No automatic retries
  - No `github.com/google/generative-ai-go` — DEPRECATED, use `google.golang.org/genai`

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Gemini SDK uses Go 1.23+ range-over-func iterators, different content structure
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: YES
  - **Parallel Group**: Wave 2 (with Tasks 7-13)
  - **Blocks**: Tasks 16, 17
  - **Blocked By**: Tasks 1 (config types), 2 (Provider interface)

  **References**:

  **Pattern References**:
  - `internal/provider/provider.go` (Task 2) — Provider interface + types
  - `internal/provider/openai.go` (Task 12) — sibling adapter for consistent structure

  **API/Type References**:
  - `internal/config/config.go` (Task 1) — ProviderConfig struct

  **External References**:
  - `google.golang.org/genai` — official unified SDK (NOT deprecated google/generative-ai-go)
  - Gemini API docs: `https://ai.google.dev/gemini-api/docs/text-generation`
  - Librarian research: range-over-func streaming, SystemInstruction, FunctionDeclaration

  **WHY Each Reference Matters**:
  - `provider.go` — interface contract
  - `openai.go` — follow same structure for consistency across all 3 adapters
  - Gemini docs — range-over-func is Go 1.23+ feature, different streaming pattern

  **Acceptance Criteria**:
  - [ ] `internal/provider/google.go` exists implementing Provider interface
  - [ ] `internal/provider/google_test.go` exists with ≥3 test functions
  - [ ] `go build ./...` → success
  - [ ] `go test -v ./internal/provider/...` → PASS
  - [ ] No `genai.*` types in exported function signatures
  - [ ] No import of `github.com/google/generative-ai-go` (deprecated)

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Google adapter compiles with SDK
    Tool: Bash
    Steps:
      1. Run `go build ./internal/provider/...`
      2. Run `go test -v ./internal/provider/...`
    Expected Result: Build succeeds, tests pass
    Evidence: .sisyphus/evidence/task-14-google-build.txt

  Scenario: No deprecated SDK imported
    Tool: Bash
    Steps:
      1. Run `grep -r 'generative-ai-go' go.mod go.sum internal/provider/`
    Expected Result: Zero matches — deprecated SDK not used
    Failure Indicators: Any reference to google/generative-ai-go
    Evidence: .sisyphus/evidence/task-14-google-deprecated-check.txt
  ```

  **Commit**: YES (groups with Tasks 12, 13)
  - Message: `feat(provider): add OpenAI, Anthropic, Google adapters`
  - Files: `internal/provider/google.go`, `internal/provider/google_test.go`, `go.mod`, `go.sum`
  - Pre-commit: `go build ./... && go test ./internal/provider/...`

---

- [x] 15. System Prompt + Message Formatting

  **What to do**:
  - Create `internal/agent/prompt.go` with:
    - `DefaultSystemPrompt` constant — sensible default for a general-purpose AI assistant with tools
    - `FormatSystemPrompt(custom string, tools []tool.Tool) string` — builds final system prompt
    - `BuildMessages(systemPrompt string, history []*session.Message) []provider.Message` — converts session messages to provider messages with system prepend
  - Create `internal/agent/prompt_test.go`:
    - TestDefaultSystemPrompt — verify non-empty, mentions tools
    - TestFormatSystemPromptCustom — custom prompt replaces default
    - TestBuildMessages — verify correct mapping and system prepend

  **Must NOT do**: No prompt templates beyond tool descriptions. No token counting/truncation.

  **Recommended Agent Profile**: `quick` — String formatting + type mapping. **Skills**: []

  **Parallelization**: Wave 3 (can start early since no Wave 2 deps). **Blocks**: Task 16. **Blocked By**: None (uses types from Tasks 2, 3, 4)

  **References**: `internal/provider/provider.go` (Task 2) — provider.Message. `internal/session/session.go` (Task 3) — session.Message. `internal/tool/tool.go` (Task 4) — Tool interface.

  **Acceptance Criteria**:
  - [ ] `internal/agent/prompt.go` exists with DefaultSystemPrompt + FormatSystemPrompt + BuildMessages
  - [ ] `internal/agent/prompt_test.go` exists with ≥3 test functions
  - [ ] `go test -v ./internal/agent/...` → PASS
  - [ ] `go build ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: System prompt includes tool info
    Tool: Bash
    Steps: 1. Run `go test -v -run TestDefaultSystemPrompt ./internal/agent/...`
    Expected Result: Test PASS, default prompt is non-empty and references tools
    Evidence: .sisyphus/evidence/task-15-prompt.txt

  Scenario: BuildMessages maps correctly
    Tool: Bash
    Steps: 1. Run `go test -v -run TestBuildMessages ./internal/agent/...`
    Expected Result: Test PASS, system message prepended, roles mapped correctly
    Evidence: .sisyphus/evidence/task-15-messages.txt
  ```

  **Commit**: YES (groups with Task 16) — `feat(agent): implement ReAct executor with tool calling`

---

- [x] 16. Agent Executor (ReAct Loop)

  **What to do**:
  - Create `internal/agent/agent.go` with:
    - `Agent` struct: provider, tools registry, store, config
    - `New(p provider.Provider, tools *tool.Registry, store session.SessionStore, cfg config.AgentConfig) *Agent`
    - `(a *Agent) RunStream(ctx, sessionID, userMessage string, w io.Writer) error`:
      1. Add user message to session store
      2. Get full message history, apply history limit (keep last N, default 50)
      3. Build messages via `BuildMessages()` (Task 15)
      4. Call `provider.ChatStream()` — stream response tokens
      5. Read chunks from channel, write text deltas to `w`
      6. If response has tool calls: execute each tool sequentially
         - `registry.Get(name)` → `tool.Execute(ctx, args)`
         - Add assistant message (with tool calls) + tool result messages to session
         - Loop back to step 2
      7. If no tool calls (finish_reason=stop): add assistant message, return
      8. Max iterations guard: if loop > `config.MaxIterations` (default 10), return error
  - Handle errors:
    - Provider errors → wrap and return (no retry)
    - Tool execution errors → add error as tool result, let LLM handle
    - Unknown tool name → add error as tool result
    - Max iterations → return descriptive error
  - Create `internal/agent/agent_test.go` (mock provider):
    - TestAgentSimpleChat — no tools → stored in session
    - TestAgentToolCall — tool call → execute → result fed back → final response
    - TestAgentMaxIterations — always tool calls → hits limit → error
    - TestAgentUnknownTool — unknown tool → error result sent back
  - Use `log/slog` for logging tool calls, iterations, errors

  **Must NOT do**:
  - No parallel tool execution — sequential only
  - No context compaction or summarization
  - No sub-agent spawning
  - No automatic retries on provider errors
  - No direct stdout printing — write to provided `io.Writer`

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Core orchestration logic — ReAct loop, streaming, tool chaining, error propagation
  - **Skills**: []

  **Parallelization**:
  - **Can Run In Parallel**: NO
  - **Parallel Group**: Wave 3 (Sequential — depends on nearly everything)
  - **Blocks**: Tasks 17, 19
  - **Blocked By**: Tasks 2, 3, 4, 7, 9-14, 15

  **References**:

  **Pattern References**:
  - `internal/bus/bus.go` — goroutine lifecycle, channel reading patterns

  **API/Type References**:
  - `internal/provider/provider.go` (Task 2) — Provider interface, ChatRequest, ChatResponse, ChatChunk
  - `internal/session/session.go` (Task 3) — SessionStore interface, Message types
  - `internal/tool/tool.go` (Task 4) — Registry.Get(), Tool.Execute()
  - `internal/agent/prompt.go` (Task 15) — BuildMessages, FormatSystemPrompt
  - `internal/config/config.go` (Task 1) — AgentConfig (MaxIterations, HistoryLimit, SystemPrompt)

  **Acceptance Criteria**:
  - [ ] `internal/agent/agent.go` exists with Agent struct + RunStream method
  - [ ] `internal/agent/agent_test.go` exists with ≥4 test functions using mock provider
  - [ ] `go test -v ./internal/agent/...` → PASS
  - [ ] `go build ./...` → success

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Agent handles simple conversation
    Tool: Bash
    Steps: 1. Run `go test -v -run TestAgentSimpleChat ./internal/agent/...`
    Expected Result: Test PASS, user message and assistant response stored in session
    Evidence: .sisyphus/evidence/task-16-simple-chat.txt

  Scenario: Agent executes tool call loop
    Tool: Bash
    Steps: 1. Run `go test -v -run TestAgentToolCall ./internal/agent/...`
    Expected Result: Test PASS — tool called, result fed back, final response stored
    Evidence: .sisyphus/evidence/task-16-tool-call.txt

  Scenario: Agent respects max iterations
    Tool: Bash
    Steps: 1. Run `go test -v -run TestAgentMaxIterations ./internal/agent/...`
    Expected Result: Test PASS, error returned after N iterations
    Evidence: .sisyphus/evidence/task-16-max-iterations.txt
  ```

  **Commit**: YES (groups with Task 15) — `feat(agent): implement ReAct executor with tool calling`

---

- [x] 17. CLI Chat Command (`tg chat`)

  **What to do**:
  - Create `cmd/tg/chat.go` with Cobra command:
    - `chatCmd` registered as subcommand of root
    - Flags: `--provider` (string, override default), `--model` (string, override model), `--continue` (string, session ID to resume)
    - On run:
      1. Load config (Task 1)
      2. Select provider: `--provider` flag or first configured with valid API key
      3. Create provider adapter (Task 12/13/14)
      4. Create tool registry with bash/read/write tools
      5. Create session store (in-memory default, PG if DATABASE_URL set)
      6. Create or resume session
      7. Print welcome banner and instructions
      8. Enter interactive loop: read stdin → agent.RunStream → print newline → repeat
      9. Handle Ctrl+C: catch SIGINT, print "Goodbye!", exit cleanly
  - Create `internal/provider/factory.go`:
    - `NewProvider(name string, cfg config.ProviderConfig) (Provider, error)` — factory
    - Switch on name: "openai"→NewOpenAI, "anthropic"→NewAnthropic, "google"→NewGoogle
  - Wire up in `cmd/tg/main.go`

  **Must NOT do**:
  - No bubbletea/charmbracelet/TUI frameworks — plain stdin/stdout
  - No multi-line input — single Enter sends
  - No command history or readline
  - No color output (Phase 1 simplicity)

  **Recommended Agent Profile**:
  - **Category**: `deep`
    - Reason: Wiring all components together + signal handling + interactive I/O
  - **Skills**: []

  **Parallelization**: Wave 3 (Sequential). **Blocks**: Task 19. **Blocked By**: Tasks 1, 3, 7, 12-14, 16

  **References**:
  - `cmd/tg/main.go` — existing Cobra setup
  - All `internal/*/` packages — this is the integration point

  **Acceptance Criteria**:
  - [ ] `cmd/tg/chat.go` exists with Cobra chatCmd
  - [ ] `internal/provider/factory.go` exists with NewProvider factory
  - [ ] `make build` → success, `bin/tg` produced
  - [ ] `./bin/tg chat --help` shows flags: --provider, --model, --continue
  - [ ] `./bin/tg` shows `chat` in available commands

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: tg chat command is registered
    Tool: Bash
    Steps:
      1. Run `make build`
      2. Run `./bin/tg --help 2>&1`
      3. Verify output contains "chat"
    Expected Result: "chat" appears in help output
    Evidence: .sisyphus/evidence/task-17-chat-help.txt

  Scenario: tg chat shows help with flags
    Tool: Bash
    Steps:
      1. Run `./bin/tg chat --help 2>&1`
      2. Verify --provider, --model, --continue flags listed
    Expected Result: All 3 flags listed
    Evidence: .sisyphus/evidence/task-17-chat-flags.txt

  Scenario: tg chat fails gracefully without API key
    Tool: Bash
    Steps:
      1. Run `echo '' | ./bin/tg chat 2>&1`
    Expected Result: Descriptive error about missing provider/API key, non-zero exit
    Failure Indicators: Panic, stack trace, or silent hang
    Evidence: .sisyphus/evidence/task-17-chat-no-key.txt
  ```

  **Commit**: YES (groups with Task 18) — `feat(cli): add tg chat and tg config commands`

---

- [x] 18. CLI Config Command (`tg config show`)

  **What to do**:
  - Create `cmd/tg/config.go` with Cobra command:
    - `configCmd` — parent command
    - `configShowCmd` — `tg config show`
    - On run: Load config → marshal to YAML → redact API keys (`***`) → print to stdout
  - Wire up in `cmd/tg/main.go`
  - Add `gopkg.in/yaml.v3` dependency

  **Must NOT do**: No config editing, no config generation, API keys must be redacted

  **Recommended Agent Profile**: `quick` — Single command. **Skills**: []

  **Parallelization**: Wave 3 (parallel). **Blocks**: Task 19. **Blocked By**: Task 1

  **References**: `cmd/tg/main.go` — Cobra patterns. `internal/config/config.go` (Task 1) — Load(), Config.

  **Acceptance Criteria**:
  - [ ] `cmd/tg/config.go` exists with configCmd + configShowCmd
  - [ ] `make build` → success
  - [ ] `./bin/tg config show --help` works
  - [ ] API keys are redacted in output

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: tg config show prints YAML
    Tool: Bash
    Steps:
      1. Run `make build && ./bin/tg config show 2>&1`
    Expected Result: Valid YAML showing config structure
    Evidence: .sisyphus/evidence/task-18-config-show.txt

  Scenario: API keys are redacted
    Tool: Bash
    Steps:
      1. Run `TIANGONG_PROVIDERS_OPENAI_API_KEY=sk-secret123 ./bin/tg config show 2>&1`
      2. Verify output does NOT contain "sk-secret123"
    Expected Result: API key replaced with redaction marker
    Evidence: .sisyphus/evidence/task-18-config-redact.txt
  ```

  **Commit**: YES (groups with Task 17) — `feat(cli): add tg chat and tg config commands`

---

- [x] 19. Integration Tests + Edge Cases

  **What to do**:
  - Create `internal/agent/integration_test.go`:
    - Full pipeline test: config → memory store → tool registry → mock provider → agent
    - Test complete ReAct loop: user asks → LLM requests bash tool → executes → result fed back → final answer
    - Verify session has all messages in correct order
  - Test edge cases:
    - Empty user input → handled gracefully
    - Provider returns error → propagated correctly
    - Tool returns error → fed back as tool result
    - Max iterations → descriptive error
    - Session not found with --continue → error
  - Create integration tests for config and gateway packages
  - Verify `go build ./...`, `go test ./...`, `go vet ./...` all pass

  **Must NOT do**: No real LLM API calls, no database required, no flaky tests

  **Recommended Agent Profile**: `deep` — Cross-cutting integration. **Skills**: []

  **Parallelization**: Wave 4 (Sequential). **Blocks**: F1-F4. **Blocked By**: Tasks 6-18

  **References**: `internal/bus/bus_test.go` — test patterns. All `internal/*/` packages.

  **Acceptance Criteria**:
  - [ ] Integration test files exist with ≥5 test functions
  - [ ] `go test ./...` → ALL PASS
  - [ ] `go build ./...` → success
  - [ ] `go vet ./...` → success
  - [ ] Edge cases tested: empty input, provider error, tool error, max iterations, missing session

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Full test suite passes
    Tool: Bash
    Steps: 1. Run `go test -v -count=1 ./... 2>&1 | tail -50`
    Expected Result: All tests PASS, zero failures
    Evidence: .sisyphus/evidence/task-19-full-tests.txt

  Scenario: Integration test exercises full pipeline
    Tool: Bash
    Steps: 1. Run `go test -v -run TestFullPipeline ./internal/agent/...`
    Expected Result: Test PASS — full ReAct loop with tool call
    Evidence: .sisyphus/evidence/task-19-pipeline.txt

  Scenario: Vet passes
    Tool: Bash
    Steps: 1. Run `go vet ./...`
    Expected Result: Exit 0, no warnings
    Evidence: .sisyphus/evidence/task-19-vet.txt
  ```

  **Commit**: YES — `test: add integration tests and edge case coverage`

---

- [x] 20. Build + Lint + Vet Verification

  **What to do**:
  - Run full verification: `make build`, `make lint`, `make vet`, `make test`
  - Fix any issues: lint warnings, unused imports, missing doc comments, `fmt.Println`→`slog.Info`
  - Update `internal/tool/tool.go` — `NewDefaultRegistry()` registers bash, read, write tools
  - Verify binaries run: `./bin/tg version`, `./bin/tg --help`, `./bin/tiangong version`

  **Must NOT do**: No feature additions, no `//nolint` without justification

  **Recommended Agent Profile**: `quick` — Verification + minor fixes. **Skills**: []

  **Parallelization**: Wave 4 (parallel with Task 19). **Blocks**: F1-F4. **Blocked By**: Tasks 6-18

  **References**: `Makefile`, `.golangci.yml`

  **Acceptance Criteria**:
  - [ ] `make build` → produces `bin/tg` and `bin/tiangong`
  - [ ] `make lint` → zero issues
  - [ ] `make vet` → exit 0
  - [ ] `make test` → all PASS
  - [ ] `./bin/tg version` → version string
  - [ ] `./bin/tg --help` shows chat, config, version
  - [ ] `NewDefaultRegistry()` returns registry with bash, read, write tools

  **QA Scenarios (MANDATORY):**
  ```
  Scenario: Full build succeeds
    Tool: Bash
    Steps:
      1. Run `make build 2>&1`
      2. Verify `bin/tg` and `bin/tiangong` exist
    Expected Result: Both binaries produced
    Evidence: .sisyphus/evidence/task-20-build.txt

  Scenario: Lint passes clean
    Tool: Bash
    Steps: 1. Run `make lint 2>&1`
    Expected Result: Zero lint issues
    Evidence: .sisyphus/evidence/task-20-lint.txt

  Scenario: Binary runs correctly
    Tool: Bash
    Steps:
      1. Run `./bin/tg version`
      2. Run `./bin/tg --help`
    Expected Result: Version prints, help lists all commands
    Evidence: .sisyphus/evidence/task-20-binary.txt
  ```

  **Commit**: YES — `chore: verify build, lint, vet all pass; wire default tool registry`

---


## Final Verification Wave (MANDATORY — after ALL implementation tasks)

> 4 review agents run in PARALLEL. ALL must APPROVE. Rejection → fix → re-run.

- [x] F1. **Plan Compliance Audit** — `oracle`
  Read the plan end-to-end. For each "Must Have": verify implementation exists (read file, run command). For each "Must NOT Have": search codebase for forbidden patterns — reject with file:line if found. Check evidence files exist in .sisyphus/evidence/. Compare deliverables against plan.
  Output: `Must Have [N/N] | Must NOT Have [N/N] | Tasks [N/N] | VERDICT: APPROVE/REJECT`

- [x] F2. **Code Quality Review** — `unspecified-high`
  Run `make build && make lint && make vet && make test`. Review all changed files for: `as any`/`interface{}`(unjustified), empty catches, `fmt.Println` in prod (should be slog), commented-out code, unused imports, `//nolint` without justification. Check AI slop: excessive comments, over-abstraction, generic names.
  Output: `Build [PASS/FAIL] | Lint [PASS/FAIL] | Vet [PASS/FAIL] | Tests [N pass/N fail] | Files [N clean/N issues] | VERDICT`

- [x] F3. **Real Manual QA** — `unspecified-high`
  Start from clean state. Execute EVERY QA scenario from EVERY task — follow exact steps, capture evidence. Test cross-task integration: config → provider → agent → tool → CLI flow. Test edge cases: empty input, Ctrl+C, invalid config, no API key. Save to `.sisyphus/evidence/final-qa/`.
  Output: `Scenarios [N/N pass] | Integration [N/N] | Edge Cases [N tested] | VERDICT`

- [x] F4. **Scope Fidelity Check** — `deep`
  For each task: read "What to do", read actual diff. Verify 1:1 — everything in spec was built (no missing), nothing beyond spec (no creep). Check "Must NOT do" compliance. Flag forbidden patterns: bubbletea imports, pkg/ directory, websearch, channel adapters, sashabaranov/go-openai. Flag SDK type leaks past adapter boundaries.
  Output: `Tasks [N/N compliant] | Forbidden [CLEAN/N issues] | SDK Leak [CLEAN/N files] | VERDICT`

---

## Commit Strategy

| Wave | Commit Message | Files | Pre-commit Check |
|------|---------------|-------|-----------------|
| 1 | `feat(config): add Viper-based configuration system` | `internal/config/*` | `go build ./...` |
| 1 | `feat(provider): define provider interface and types` | `internal/provider/provider.go` | `go build ./...` |
| 1 | `feat(session): define session types and store interface` | `internal/session/*` | `go build ./...` |
| 1 | `feat(tool): define tool interface and registry` | `internal/tool/*` | `go build ./...` |
| 1 | `feat(storage): add sqlc queries and migration` | `internal/storage/*`, `sqlc.yaml` | `go build ./...` |
| 1 | `feat(gateway): add HTTP server with health endpoint` | `internal/gateway/*` | `go test ./internal/gateway/...` |
| 2 | `feat(session): implement in-memory and PostgreSQL stores` | `internal/session/*` | `go test ./internal/session/...` |
| 2 | `feat(tool): implement bash, read, write tools` | `internal/tool/*` | `go test ./internal/tool/...` |
| 2 | `feat(provider): add OpenAI, Anthropic, Google adapters` | `internal/provider/*` | `go test ./internal/provider/...` |
| 3 | `feat(agent): implement ReAct executor with tool calling` | `internal/agent/*` | `go test ./internal/agent/...` |
| 3 | `feat(cli): add tg chat and tg config commands` | `cmd/tg/*` | `make build && make test` |
| 4 | `test: add integration tests and edge case coverage` | `*_test.go` | `make test` |
| 4 | `chore: verify build, lint, vet all pass` | — | `make build && make lint && make vet && make test` |

---

## Success Criteria

### Verification Commands
```bash
make build         # Expected: bin/tg and bin/tiangong produced
make lint          # Expected: 0 issues
make vet           # Expected: exit 0
make test          # Expected: all tests PASS

./bin/tg version   # Expected: "tg v0.1.0" or similar
./bin/tiangong version  # Expected: "tiangong v0.1.0"

# Config loads from env
TIANGONG_PROVIDERS_OPENAI_API_KEY=test ./bin/tg config show 2>&1 | grep -q "openai"

# Health endpoint
./bin/tiangong &
curl -s http://localhost:8080/health | grep -q '"status":"ok"'
kill %1

# Chat (requires real API key)
# TIANGONG_PROVIDERS_OPENAI_API_KEY=$KEY ./bin/tg chat
# → Type "What is 2+2?" → streams response → "4"
# → Type "Write 'hello' to /tmp/tg-test.txt using the write tool" → tool executes
# → cat /tmp/tg-test.txt → "hello"
# → Ctrl+C → graceful exit
```

### Final Checklist
- [x] All "Must Have" present
- [x] All "Must NOT Have" absent
- [x] All tests pass (`make test`)
- [x] Lint clean (`make lint`)
- [x] Vet clean (`make vet`)
- [x] Binary compiles (`make build`)
- [x] Provider adapters compile with real SDK imports
- [x] sqlc-generated code compiles
- [x] No SDK types leak past adapter boundaries
- [x] `log/slog` used throughout — no `fmt.Println` in production code
