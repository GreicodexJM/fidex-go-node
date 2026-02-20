# FideX AS5 Node - Unit Test Coverage Plan

## Current Test Coverage Analysis

### Existing Tests ✅
- `internal/config/config_test.go` - **Comprehensive** (11 tests, all passing)
- `internal/crypto/as5_engine_test.go` - **Exists** (crypto operations)
- `internal/discovery/discovery_test.go` - **Exists** (discovery service)

### Coverage Summary
```
cmd/fidex-node:      0.0% coverage
internal/api:        0.0% coverage
internal/auth:       0.0% coverage
internal/config:     Good coverage ✅
internal/crypto:     Partial coverage ✅
internal/discovery:  Partial coverage ✅
```

---

## Critical Gaps Requiring Unit Tests

### Priority 1: CRITICAL (Must Have)

#### 1.1 Repository Layer (0% coverage)
**Status**: ❌ **NO TESTS**

**Files Needing Tests**:
- `internal/repository/message_repository.go`
- `internal/repository/partner_repository.go`
- `internal/repository/user_repository.go`
- `internal/repository/session_repository.go`

**Why Critical**:
- Core data access layer
- Directly interacts with database
- Used by all business logic
- Recently refactored with new patterns

**Test Requirements**:
```go
message_repository_test.go:
- TestMessageRepository_Create()
- TestMessageRepository_GetByID()
- TestMessageRepository_UpdateStatus()
- TestMessageRepository_UpdateRetryInfo()
- TestMessageRepository_ListByStatus()
- TestMessageRepository_Delete()
- TestMessageRepository_ErrorHandling()
- TestMessageRepository_ContextCancellation()

partner_repository_test.go:
- TestPartnerRepository_Create()
- TestPartnerRepository_GetByID()
- TestPartnerRepository_Update()
- TestPartnerRepository_Delete()
- TestPartnerRepository_List()
- TestPartnerRepository_ErrorHandling()
- TestPartnerRepository_Validation()

user_repository_test.go:
- TestUserRepository_Create()
- TestUserRepository_GetByID()
- TestUserRepository_GetByUsername()
- TestUserRepository_UpdatePassword()
- TestUserRepository_Delete()
- TestUserRepository_List()
- TestUserRepository_DuplicateUsername()

session_repository_test.go:
- TestSessionRepository_Create()
- TestSessionRepository_GetByID()
- TestSessionRepository_Delete()
- TestSessionRepository_DeleteExpired()
- TestSessionRepository_ExpiredSessionHandling()
```

**Test Approach**:
- Use in-memory SQLite database (`:memory:`)
- Test CRUD operations
- Verify error handling
- Test context cancellation
- Validate input constraints
- Test concurrent access (if applicable)

---

#### 1.2 Error Handling System (0% coverage)
**Status**: ❌ **NO TESTS**

**File**: `internal/errors/errors.go`

**Why Critical**:
- Foundation for all error handling
- Used throughout the application
- Determines retry behavior
- Affects API responses

**Test Requirements**:
```go
errors_test.go:
- TestAppError_Creation()
- TestAppError_ErrorCodes()
- TestAppError_HTTPStatusMapping()
- TestAppError_RetryableClassification()
- TestAppError_ErrorWrapping()
- TestAppError_IsAppError()
- TestAppError_IsRetryable()
- TestErrorConstructors() // BadRequest, Unauthorized, NotFound, etc.
- TestErrorDetails()
- TestErrorContext()
```

**Key Test Scenarios**:
- Verify all 15+ error constructors
- Test HTTP status code mapping
- Validate retryable vs non-retryable
- Test error wrapping preserves context
- Verify error details are included
- Test IsAppError type checking

---

#### 1.3 API Response System (0% coverage)
**Status**: ❌ **NO TESTS**

**File**: `internal/api/response.go`

**Why Critical**:
- Standardizes all API responses
- Integrates with error system
- Affects client experience
- Recently created

**Test Requirements**:
```go
response_test.go:
- TestRespondJSON()
- TestRespondSuccess()
- TestRespondCreated()
- TestRespondNoContent()
- TestRespondError()
- TestRespondAppError()
- TestRespondBadRequest()
- TestRespondUnauthorized()
- TestRespondForbidden()
- TestRespondNotFound()
- TestRespondInternalError()
- TestResponseFormat() // Verify JSON structure
- TestErrorLogging() // Verify errors are logged
```

**Test Approach**:
- Use httptest.ResponseRecorder
- Verify HTTP status codes
- Validate JSON response format
- Check response headers
- Verify success/error fields
- Test with various data types

---

### Priority 2: HIGH (Should Have)

#### 2.1 Queue Worker (0% coverage)
**Status**: ❌ **NO TESTS**

**File**: `internal/queue/worker.go`

**Why Important**:
- Production-critical message delivery
- Complex retry logic
- Exponential backoff implementation
- Recently completely rewritten

**Test Requirements**:
```go
worker_test.go:
- TestWorker_DeliverMessage_Success()
- TestWorker_DeliverMessage_Failure()
- TestWorker_RetryLogic()
- TestWorker_ExponentialBackoff()
- TestWorker_MaxRetries()
- TestWorker_ErrorClassification()
- TestWorker_StatusTracking()
- TestWorker_ContextCancellation()
- TestWorker_ConfigurableSettings()
- TestWorkerConfig_Defaults()
```

**Test Approach**:
- Mock HTTP client for partner calls
- Mock repositories for data access
- Test retry scheduling
- Verify exponential backoff timing
- Test failure scenarios
- Validate status updates

---

#### 2.2 Context Management (0% coverage)
**Status**: ❌ **NO TESTS**

**File**: `internal/api/context.go`

**Why Important**:
- Request tracing foundation
- Used in all HTTP handlers
- Security context (user IDs)

**Test Requirements**:
```go
context_test.go:
- TestRequestIDMiddleware()
- TestRequestIDMiddleware_ExistingID()
- TestRequestIDMiddleware_NewID()
- TestGetRequestID()
- TestGetUserID()
- TestWithUserID()
- TestRequestIDHeader()
- TestContextKeyIsolation()
```

**Test Approach**:
- Test middleware integration
- Verify UUID generation
- Test header propagation
- Validate context values
- Test type safety

---

#### 2.3 Structured Logging (0% coverage)
**Status**: ❌ **NO TESTS**

**File**: `internal/logging/logger.go`

**Why Important**:
- Production observability
- Request correlation
- Debugging capabilities

**Test Requirements**:
```go
logger_test.go:
- TestLogger_Info()
- TestLogger_Error()
- TestLogger_Warn()
- TestLogger_Debug()
- TestLogger_WithContext()
- TestLogger_RequestIDExtraction()
- TestLogger_UserIDExtraction()
- TestLogger_Format()
- TestDefaultLogger()
```

**Test Approach**:
- Capture log output
- Verify format
- Test context extraction
- Validate log levels
- Test with/without context

---

### Priority 3: MEDIUM (Nice to Have)

#### 3.1 Dependency Injection Container
**File**: `internal/container/container.go`

**Test Requirements**:
- TestContainer_Creation()
- TestContainer_InitializationOrder()
- TestContainer_GracefulShutdown()
- TestContainer_ErrorHandling()

#### 3.2 API Handlers
**Files**: `internal/api/*_handlers.go`

**Test Requirements**:
- TestTransmitHandler()
- TestInboundHandler()
- TestAuthHandlers()
- TestDashboardHandlers()
- TestSettingsHandlers()

**Note**: These would be integration tests requiring HTTP test server

---

## Test Implementation Strategy

### Phase 1: Foundation (Week 1)
1. ✅ Error handling tests (`errors_test.go`)
2. ✅ API response tests (`response_test.go`)
3. ✅ Context management tests (`context_test.go`)

### Phase 2: Data Layer (Week 2)
4. ✅ Message repository tests
5. ✅ Partner repository tests
6. ✅ User repository tests
7. ✅ Session repository tests

### Phase 3: Business Logic (Week 3)
8. ✅ Queue worker tests
9. ✅ Logging tests
10. ✅ Container tests

### Phase 4: Integration (Week 4)
11. ✅ API handler tests
12. ✅ End-to-end tests
13. ✅ Performance tests

---

## Test Standards and Best Practices

### Test File Naming
```
<package>_test.go       // Same package tests
<feature>_test.go       // Feature-specific tests
```

### Test Function Naming
```go
func TestFunction_Scenario_ExpectedBehavior(t *testing.T)

Examples:
- TestMessageRepository_Create_ValidMessage()
- TestMessageRepository_Create_DuplicateID()
- TestMessageRepository_GetByID_NotFound()
```

### Test Structure (AAA Pattern)
```go
func TestExample(t *testing.T) {
    // Arrange
    setup := createTestSetup()
    
    // Act
    result, err := functionUnderTest()
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Required Test Helpers
```go
// Test database setup
func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    require.NoError(t, err)
    // Run migrations
    return db
}

// Mock HTTP responses
func mockHTTPClient(status int, body string) *http.Client {
    // Return mock client
}

// Test context creation
func testContext() context.Context {
    ctx := context.Background()
    // Add test values
    return ctx
}
```

### Testing Libraries
```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/mock"
)
```

---

## Coverage Goals

### Target Coverage by Component

| Component | Current | Target | Priority |
|-----------|---------|--------|----------|
| Repositories | 0% | 80%+ | P1 |
| Errors | 0% | 90%+ | P1 |
| API Responses | 0% | 85%+ | P1 |
| Queue Worker | 0% | 75%+ | P2 |
| Context | 0% | 85%+ | P2 |
| Logging | 0% | 70%+ | P2 |
| Container | 0% | 60%+ | P3 |
| Handlers | 0% | 70%+ | P3 |

### Overall Project Goal
- **Current**: ~40% (estimated with existing tests)
- **Target**: **75%+** (industry standard for production code)
- **Stretch Goal**: 85%+

---

## Test Execution

### Running Tests
```bash
# All tests
go test ./...

# With coverage
go test ./... -cover

# Detailed coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Specific package
go test ./internal/repository/...

# Verbose output
go test ./... -v

# Short mode (skip slow tests)
go test ./... -short
```

### CI/CD Integration
```bash
# In CI pipeline
go test ./... -race -coverprofile=coverage.txt -covermode=atomic
go tool cover -func=coverage.txt
```

---

## Dependencies Needed

### Testing Libraries
```bash
go get github.com/stretchr/testify
go get github.com/DATA-DOG/go-sqlmock  # For database mocking
```

### In go.mod
```
require (
    github.com/stretchr/testify v1.8.4
    github.com/DATA-DOG/go-sqlmock v1.5.0
)
```

---

## Test File Templates

### Repository Test Template
```go
package repository_test

import (
    "context"
    "database/sql"
    "testing"
    
    "fidex-node/internal/domain"
    "fidex-node/internal/repository"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    _ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    require.NoError(t, err)
    
    // Run schema migrations
    _, err = db.Exec(`CREATE TABLE IF NOT EXISTS messages (...)`)
    require.NoError(t, err)
    
    return db
}

func TestMessageRepository_Create(t *testing.T) {
    // Arrange
    db := setupTestDB(t)
    defer db.Close()
    repo := repository.NewSQLiteMessageRepository(db)
    
    msg := &domain.Message{
        MessageID: "test-123",
        // ... other fields
    }
    
    // Act
    err := repo.Create(context.Background(), msg)
    
    // Assert
    assert.NoError(t, err)
}
```

---

## Success Metrics

### Code Quality
- ✅ All tests passing
- ✅ 75%+ code coverage
- ✅ No race conditions detected
- ✅ Fast test execution (<30s for full suite)

### Test Quality
- ✅ Tests are independent
- ✅ Tests are deterministic
- ✅ Clear test names
- ✅ Good error messages
- ✅ Proper cleanup

### Documentation
- ✅ Test purpose documented
- ✅ Complex scenarios explained
- ✅ Edge cases covered
- ✅ Test data documented

---

## Conclusion

This test coverage plan prioritizes the most critical components created during the recent refactoring. The repository layer, error handling, and API responses are the foundation that everything else builds upon, making them Priority 1.

By following this plan, we will achieve production-grade test coverage and ensure the reliability of the FideX AS5 Node.

**Next Steps**:
1. Install testing dependencies
2. Create test files for Priority 1 components
3. Implement repository tests with in-memory database
4. Add error handling tests
5. Implement API response tests
6. Measure and track coverage improvements
