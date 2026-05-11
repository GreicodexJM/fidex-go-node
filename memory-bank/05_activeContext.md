# Active Context: FideX AS5 Node

## Current Work Focus
**Refactor Phase 1.6 — Structured logging across all packages** (2026-05-11). Phase 1.4 merged to master. Every `log.Printf`/`log.Println` call outside of `main.go` `must*` helpers now flows through `internal/logging.Logger` with per-package prefixes (`api`, `auth`, `container`, `dashboard`, `discovery`, `main`, `queue`, `watcher`). Bug fix: `RequestIDKey` / `UserIDKey` unified in `internal/logging` (typed `ContextKey`); `internal/api/context.go` re-exports them — request_ids and user_ids now actually propagate from middleware to log lines. `auth.RequireAuth` / `auth.RequireAuthAPI` stamp the authenticated user id into context via `logging.WithUserID` so every downstream log line carries it automatically. Branch: `feature/phase-1.6-structured-logging`.

## Recent Changes (2026-05-11) — Phase 1.6

### What changed
- ✅ **Context-key bug fix in `internal/logging`**: `extractRequestID` and `extractUserID` used string keys `"request_id"` / `"user_id"`; `internal/api/context.go` was setting them via typed `ContextKey("request_id")`. Different keys in Go, so log lines never carried the request id. Unified by moving `RequestIDKey`, `UserIDKey` (typed `logging.ContextKey`) into `internal/logging` plus helpers `logging.WithRequestID` / `logging.WithUserID`. `internal/api/context.go` now re-exports them.
- ✅ **`logging.Logger` honoured everywhere**: every `log.Printf`/`log.Println` call across `internal/api/*`, `internal/auth/middleware.go`, `internal/container/container.go`, `internal/dashboard/websocket.go`, `internal/discovery/service.go`, `internal/queue/worker.go`, `internal/watcher/fs_worker.go`, and `cmd/fidex-node/main.go` rewritten as `pkgLogger.Info/Warn/Error/Debug(ctx, …)`. Per-package logger instances named after the package (`api`, `auth`, `container`, `dashboard`, `discovery`, `main`, `queue`, `watcher`).
- ✅ **`auth` middleware seeds user id into context**: both `RequireAuth` and `RequireAuthAPI` now call `logging.WithUserID(ctx, user.ID)` so every downstream log line in the request lifecycle carries `[user_id=N]` automatically.
- ✅ **Level discipline**: failed logins, IP allowlist denials, invalid sessions, retry warnings → `Warn`. Repository / I/O failures → `Error`. Request lifecycle events → `Info`. Debug-only details (IP allowlist check, websocket client messages) → `Debug`.
- ✅ **`log.Fatalf` retained only in `cmd/fidex-node/main.go` `must*` helpers**, per the project convention "only main can panic". All other `log.*` calls eradicated outside `internal/logging/` itself.
- ✅ **Test fix-up**: `internal/logging/logger_test.go` updated to use the typed `RequestIDKey` / `UserIDKey` constants instead of string literals. All 25+ logging tests still green.

### Verification (Phase 1.6)
- `go build ./...` clean
- `go test ./...` green (api / config / container / crypto / discovery / errors / logging / queue / repository all pass)
- Binary smoke test: every boot log line now formatted as `2026-05-11 HH:MM:SS [LEVEL] pkg: message` with per-package prefix visible (`main`, `container`, `queue`, `watcher`, `api`). SIGTERM still produces clean shutdown sequence.

### Outstanding
- Repository interface gap: dashboard / settings handlers still use `h.DB` for raw SQL (paginated listing, date-range counts, partner upsert). Track for a future Phase 1.x.
- Future log levels via env (LEVEL=DEBUG/INFO/WARN/ERROR) — `internal/logging` currently emits all levels unconditionally.

## Recent Changes (2026-05-11) — Phase 1.4

## Recent Changes (2026-05-11) — Phase 1.4

### What changed
- ✅ **New `internal/api/api_handlers.go`** exposing `Handlers` struct that owns `Config`, `DB`, all repos, `AuthService`, `DiscoveryService`, and `WebSocketHub`.
- ✅ **All handlers converted to methods on `*Handlers`** across `auth_handlers.go`, `dashboard_handlers.go`, `discovery_handlers.go`, `external_handlers.go`, `internal_handlers.go`, `settings_handlers.go`. Pure handlers without state (`healthHandler`, `jwksHandler`, `constantsHandler`, `serveLoginHandler`, `serveDashboardHandler`, `serveStaticAssets`) remain free functions.
- ✅ **`SetupInternalRouter`, `SetupPublicRouter`, `SetupAuthRouter`, `SetupDashboardRouter`, `SetupSettingsRouter` are now methods on `*Handlers`**. Internal router now mounts auth/dashboard/settings routers itself; main.go no longer needs separate `Mount` calls.
- ✅ **`InitializeDefaultUser` is now a method on `*Handlers`**.
- ✅ **Removed `wsHub` global + `InitializeWebSocketHub` / `GetWebSocketHub`** from `dashboard_handlers.go`. `Handlers.WebSocketHub` is wired from `container.WebSocketHub` (which the container already runs).
- ✅ **`main.go` refactored** into 8 helpers — `mustLoadConfig`, `ensureKeysExist`, `logAPIKeySecurityWarning`, `mustInitContainer`, `buildHandlers`, `mustStartFileWatcher`, `startSessionCleanup`, `mustSetupServers`, `runWithGracefulShutdown`. `main()` is now ~30 lines of orchestration.
- ✅ **`.gitignore`** added (was missing — compiled `fidex-node` binary was tracked and committed in error). Binary + local SQLite + cookie/log artifacts now ignored.

### Verification (Phase 1.4)
- `go build ./...` clean
- `go test ./...` green (9/9 test packages pass)
- Binary smoke test: boots cleanly through container init → DB ready → workers running → file watcher monitoring → both HTTP servers bound. Graceful shutdown via SIGTERM works.

### Outstanding (next phase)
- Phase 1.6: Replace ~223 `log.Printf` calls with the `internal/logging` structured logger (already exists, just needs to flow through DI to handlers).
- Repository interface gap: `MessageRepository` and `PartnerRepository` still don't cover paginated listing, date-range counts, or partner upsert — dashboard/settings handlers continue using `h.DB` raw SQL. Track as deuda técnica.

## Recent Changes (2026-05-11) — Phase 1.3

### What changed
- ✅ **Deleted `internal/db/`** entirely (`sqlite.go`, `partners.go`). Schema + connection opening moved to `internal/repository/schema.go` as `OpenSQLite()` + `InitSchema()`.
- ✅ **Eliminated 3 globals**: `db.DB`, `main.AppConfig`, `api.NodeConfig` (the last one was kept as a transitional package-level var — documented).
- ✅ **`auth` package** converted to `auth.Service` struct holding `domain.SessionRepository` + `domain.UserRepository`. Middleware now methods on `*Service`.
- ✅ **`discovery.DiscoveryService`** now receives `domain.PartnerRepository` in constructor; methods take `context.Context`.
- ✅ **`watcher.FileWatcher`** now receives `domain.MessageRepository` in constructor.
- ✅ **`internal/crypto/partners.go`** deleted (was dead code — no non-test callers).
- ✅ **API handlers** wired via transitional package-level vars (`api.MessageRepo`, `api.PartnerRepo`, `api.AuthSvc`, `api.DB`, etc.) set from `main.go` after container init. `api.DB` covers the dashboard/settings raw-SQL queries until repository interfaces grow to cover them.
- ✅ **Container** now opens DB via `repository.OpenSQLite` (no global), adds `AuthService` field, wires `DiscoveryService` with the partner repo.
- ✅ **Discovery tests** rewritten to use `repository.NewSQLitePartnerRepository` + `OpenSQLite` (no globals).

### Outstanding (next phase)
- Phase 1.4: Fold the `api.*` transitional package-level vars into an `APIHandlers` struct passed to `SetupXxxRouter` factories. Extract `mustLoadConfig` / `mustInit` helpers from `main.go`.
- Phase 1.6: Replace 223 `log.Printf` calls with the `internal/logging` structured logger flowing through DI.
- Repository interface gap: `MessageRepository` does not yet cover paginated listing or date-range counts (dashboard handlers still use raw SQL via `api.DB`).

## Pre-Phase 1.3 history (2026-02-23 spec work) — kept for reference

### Phase 1 (Critical Fixes) — COMPLETE
- ✅ `openapi.yaml`: Complete rewrite with unified field names, all endpoints, full schemas
- ✅ `fidex-protocol-specification.md`: Document hierarchy preamble, receipt_webhook REQUIRED, complete J-MDN spec (7 sub-sections), conformance profiles (Core/Enhanced/Edge), interoperability test vectors
- ✅ `fidex-annotated-specification.md` (renamed from `fidex.as5-draft-specification.md`): INFORMATIVE preamble added, relationship to normative spec clarified
- ✅ Timestamp format standardized: `YYYY-MM-DDTHH:mm:ss.SSSZ`

## Older spec phases (2026-02-23)

### Phase 1 (Critical Fixes) — COMPLETE
- ✅ `openapi.yaml`: Complete rewrite with unified field names, all endpoints, full schemas
- ✅ `fidex-protocol-specification.md`: Document hierarchy preamble, receipt_webhook REQUIRED, complete J-MDN spec (7 sub-sections), conformance profiles (Core/Enhanced/Edge), interoperability test vectors
- ✅ `fidex-annotated-specification.md` (renamed from `fidex.as5-draft-specification.md`): INFORMATIVE preamble added, relationship to normative spec clarified
- ✅ Timestamp format standardized: `YYYY-MM-DDTHH:mm:ss.SSSZ`

### Phase 2 (Enhancements) — COMPLETE
- ✅ Document type registry (Section 3.3): 13 standard types + custom type naming convention
- ✅ `payload_digest` optional field added to routing header (SHA-256 integrity without decryption)
- ✅ Version negotiation protocol (Section 6.2.1) with `supported_versions` array in AS5 config
- ✅ Partner de-registration protocol (Section 6.5) with ACTIVE/SUSPENDED/INACTIVE states
- ✅ AS5 config expanded: `supported_versions`, `conformance_profile`, `supported_document_types`, `receive_receipt` endpoint
- ✅ `fidex-quickstart.md`: 5-minute quick start guide created

### Phase 3 (Machine Validation & Operations) — COMPLETE
- ✅ JSON Schema definitions (Appendix E): 5 schemas for routing header, envelope, J-MDN, error response, AS5 config
- ✅ Security guide restructured: INFORMATIVE preamble, 15-threat control matrix with spec references and defense layers diagram
- ✅ Implementation guide expanded: New Section 8 with error classification table, decision tree, Go/JS error handlers, J-MDN retry implementation, sender-side retry, security leak prevention rules
- ✅ All informative documents now have consistent document hierarchy preamble
- ✅ Duplicate footers removed, section numbering fixed across all guides

## Next Immediate Steps

### 1. Create Domain Interfaces (Next)
**Status**: About to start
**Files**: `internal/domain/repositories.go`, `internal/domain/services.go`
**Goal**: Define clean interfaces for all core abstractions

### 2. Implement Dependency Injection Container
**Status**: Pending
**Files**: `internal/container/container.go`
**Goal**: Centralize service composition and eliminate global variables

### 3. Refactor Database Layer
**Status**: Pending
**Files**: Refactor `internal/db/*.go`
**Goal**: Remove global `DB` variable, add context support

## Active Decisions & Considerations

### Architecture Decisions
1. **Repository Pattern**: Using interface-based repositories for all data access
2. **Container Pattern**: Single container owns all service lifetimes
3. **Context Propagation**: All operations accept `context.Context` for cancellation
4. **No Global State**: Everything injected through constructors

### Implementation Approach
- **Incremental Refactoring**: Make changes in small, testable chunks
- **Backward Compatibility**: Keep existing HTTP API unchanged
- **Test After Each Step**: Ensure `go test ./...` passes after each major change
- **Compile-Driven**: Let compiler errors guide refactoring progress

### Risk Mitigation
- **Frequent Testing**: Run tests after each file modification
- **Git Commits**: Commit after each successful sub-step
- **Rollback Plan**: Can revert to working state at any point
- **Preserve Functionality**: All existing features must continue working

## Important Patterns & Preferences

### Code Style
- **Explicit over Implicit**: Clear dependency injection over magic
- **Interfaces in Domain**: Define interfaces where they're used, not where implemented
- **Error Wrapping**: Always use `fmt.Errorf("context: %w", err)` for wrapping
- **No Panic in Libraries**: Only `main.go` can panic (with `must*` functions)

### Testing Strategy
- **Unit Tests**: Test business logic in isolation with mocks
- **Integration Tests**: Test with real SQLite database
- **No Test Database**: Use in-memory SQLite (`:memory:`)
- **Table-Driven Tests**: Use table-driven approach for multiple scenarios

### Logging Preferences
- **Structured Logging**: Will implement in Phase 1.6
- **Log Levels**: DEBUG, INFO, WARN, ERROR
- **No Sensitive Data**: Never log API keys, passwords, or encryption keys
- **Request IDs**: Include request ID in all log entries

### Naming Conventions
```go
// Interfaces: Describe capability
type MessageRepository interface {...}
type CryptoService interface {...}

// Implementations: Include technology/strategy
type SQLiteMessageRepository struct {...}
type JWECryptoService struct {...}

// Constructors: New* prefix
func NewMessageRepository(db *sql.DB) *SQLiteMessageRepository {...}

// Must* functions: For initialization that should never fail
func mustLoadConfig() *config.Config {...}
```

## Learnings & Project Insights

### What's Working Well
1. **Hexagonal Architecture Foundation**: Core structure is solid, just needs cleanup
2. **Chi Router**: Middleware composition is clean and extensible
3. **Test Coverage**: Discovery and crypto packages have excellent tests
4. **API Design**: REST endpoints are well-designed and RESTful

### What Needs Improvement
1. **Dependency Management**: Too many global variables coupling components
2. **Testability**: Hard to test handlers due to global dependencies
3. **Configuration Loading**: Flag parsing bug needs immediate fix
4. **Error Handling**: Inconsistent patterns across packages

### Discovered Issues
1. **Flag Parse Bug**: `flag.Parse()` called twice in `config.Load()`
   - **Impact**: Can cause panic if flags already parsed
   - **Fix**: Parse once, apply flag values without re-parsing

2. **Security Issue**: API key logged in plain text in `main.go`
   - **Impact**: Sensitive data in logs
   - **Fix**: Remove log statement, add warning instead

3. **Session Cleanup**: No error handling in goroutine
   - **Impact**: Silent failures, potential resource leaks
   - **Fix**: Add error logging with proper context

4. **Queue Worker**: Mock implementation in production code
   - **Impact**: Messages not actually delivered
   - **Fix**: Implement real delivery logic (Phase 2)

### Patterns to Replicate
- **Discovery Package**: Excellent example of clean service layer with tests
- **Crypto Package**: Good separation of concerns, well-tested
- **Chi Router Mounting**: Clean separation of auth, dashboard, settings routes

### Patterns to Avoid
- **Global Package Variables**: `var DB *sql.DB` couples everything
- **Direct Log.Printf**: Need structured logging with levels
- **Missing Context**: Functions should accept `context.Context`
- **Commented Code**: Remove commented mock code (queue worker)

## Current Blockers & Challenges

### Technical Challenges
1. **Circular Dependencies**: Need to carefully order package refactoring
2. **Handler Signatures**: Changing handler signatures requires API router updates
3. **Testing Existing Code**: Need to add tests before refactoring some areas

### Solutions
1. **Introduce Interfaces First**: Define interfaces before changing implementations
2. **Use Closures**: Handlers can close over Container for dependencies
3. **Add Tests Incrementally**: Write tests for critical paths as we go

## Context for Next Session

### When I Return, I Should Know:
1. **Phase 1 Goal**: Eliminate global state, introduce DI container
2. **Current Step**: Creating domain interfaces
3. **Compiler is Friend**: Use compiler errors to find all usages during refactoring
4. **Test Frequently**: Run `go test ./...` after each file change
5. **Memory Bank**: Update `progress.md` after completing each major step

### Quick Start After Memory Reset:
```bash
# 1. Check current phase
cat memory-bank/05_activeContext.md

# 2. Review progress
cat memory-bank/06_progress.md

# 3. Verify codebase compiles
go build ./cmd/fidex-node

# 4. Run tests
go test ./...

# 5. Continue from last checkpoint
```

### Files to Review on Return:
- `memory-bank/06_progress.md` - Current status
- `internal/domain/` - Check if interfaces created
- `internal/container/` - Check if container implemented
- `cmd/fidex-node/main.go` - Check if refactored

## Notes for Cline (My Future Self)

🎯 **Mission**: Clean up technical debt while preserving all functionality

🛠️ **Approach**: Small, incremental changes with frequent testing

✅ **Success Criteria**: 
- Zero global mutable variables
- All components testable in isolation
- `go test ./...` passes
- Application runs and serves requests
- All HTTP endpoints work as before

⚠️ **Remember**:
- Update `progress.md` after each major milestone
- Commit to git after each successful step
- If something breaks, revert and try smaller change
- Keep activeContext.md updated with new learnings
