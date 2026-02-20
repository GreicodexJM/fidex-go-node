# Active Context: FideX AS5 Node Refactoring

## Current Work Focus
**Phase 1: Foundation Improvements** - Eliminating technical debt and establishing clean architecture patterns

## Recent Changes (Just Completed)
- ✅ Created comprehensive memory bank structure (6 files)
- ✅ Documented project charter, product context, and system patterns
- ✅ Identified 10 major issues requiring refactoring
- ✅ Planned 3-phase refactoring strategy

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
