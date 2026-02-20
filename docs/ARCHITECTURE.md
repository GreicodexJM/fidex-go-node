# FideX AS5 Node - Architecture Documentation

## Overview

The FideX AS5 Node is built using **Hexagonal Architecture** (also known as Ports and Adapters), following **SOLID principles** and **Clean Architecture** patterns. This ensures the application is maintainable, testable, and scalable.

## Architecture Layers

```
┌─────────────────────────────────────────────────────┐
│                   HTTP Layer                         │
│  (Handlers, Middleware, Request/Response)           │
└──────────────────┬──────────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────────┐
│                 Domain Layer                         │
│  (Business Logic, Entities, Interfaces)             │
└──────────────────┬──────────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────────┐
│              Infrastructure Layer                    │
│  (Repositories, External Services, Database)        │
└─────────────────────────────────────────────────────┘
```

## Core Components

### 1. Domain Layer (`internal/domain/`)

**Purpose**: Contains core business logic and defines contracts (interfaces) for external dependencies.

**Key Files**:
- `repositories.go`: Repository interfaces for data persistence
- `services.go`: Service interfaces for business operations

**Domain Types**:
- `Message`: Represents a FideX AS5 message
- `Partner`: Trading partner information
- `User`: Dashboard user
- `Session`: User session data

**Benefits**:
- No external dependencies
- Easy to test in isolation
- Clear business rules

### 2. Repository Layer (`internal/repository/`)

**Purpose**: Implements data persistence following the Repository Pattern.

**Implementation**: SQLite-based repositories with context support

**Files**:
- `message_repository.go`: Message CRUD operations
- `partner_repository.go`: Partner management
- `user_repository.go`: User authentication
- `session_repository.go`: Session tracking

**Features**:
- Context propagation for cancellation
- Proper error handling
- Transaction support (future)
- Easy to swap databases (PostgreSQL, MySQL, etc.)

### 3. Error Handling (`internal/errors/`)

**Purpose**: Standardized error handling across the application.

**Key Features**:
- Type-safe error codes
- HTTP status code mapping
- Retryable vs non-retryable classification
- Error wrapping with context
- Client-safe error messages

**Example**:
```go
err := errors.NotFound("partner", partnerID)
api.RespondError(w, err)
// Returns: {"success": false, "error": {"code": "NOT_FOUND", ...}}
```

### 4. Logging System (`internal/logging/`)

**Purpose**: Context-aware structured logging with request tracing.

**Features**:
- Request ID tracking
- User ID tracking
- Log levels (INFO, WARN, ERROR, DEBUG)
- Clean, parseable format

**Example**:
```go
logging.Info(ctx, "Processing message %s", messageID)
// Output: 2024-01-15 10:30:45 [INFO] app: Processing message msg-123 [req_id=uuid user_id=1]
```

### 5. Dependency Injection Container (`internal/container/`)

**Purpose**: Centralized service composition and lifecycle management.

**Responsibilities**:
- Initialize all services with proper dependencies
- Manage application lifecycle
- Graceful shutdown coordination

**Structure**:
```go
Container {
    Config          *config.Config
    DB              *sql.DB
    MessageRepo     domain.MessageRepository
    PartnerRepo     domain.PartnerRepository
    UserRepo        domain.UserRepository
    SessionRepo     domain.SessionRepository
    CryptoService   *crypto.AS5Engine
    DiscoveryService *discovery.DiscoveryService
    QueueWorker     *queue.Worker
    WebSocketHub    *dashboard.Hub
}
```

### 6. Queue Worker (`internal/queue/`)

**Purpose**: Background message delivery with retry logic.

**Features**:
- HTTP delivery to trading partners
- Exponential backoff: 1m → 5m → 15m → 30m → 1h
- Configurable retry limits
- Error classification
- Status tracking

**Workflow**:
1. Poll database for queued messages
2. Fetch partner endpoint
3. Encrypt and send message
4. Handle success/failure
5. Update message status
6. Schedule retries if needed

### 7. Context Management (`internal/api/`)

**Purpose**: Request tracing and context propagation.

**Features**:
- Request ID generation (UUID v4)
- Request ID header propagation
- User ID context tracking
- Type-safe context keys

## Request Flow

```
1. HTTP Request arrives
        ↓
2. RequestIDMiddleware (adds UUID)
        ↓
3. Router (chi) routes to handler
        ↓
4. Handler extracts context
        ↓
5. Calls Service/Repository with context
        ↓
6. Repository performs DB operation
        ↓
7. Response returned with Request-ID header
```

## Design Patterns Used

### Repository Pattern
- Abstracts data access
- Easy to mock for testing
- Database-agnostic

### Dependency Injection
- Constructor injection
- Interface-based
- Testable and maintainable

### Ports and Adapters (Hexagonal)
- Domain at the center
- Infrastructure at the edges
- Easy to swap implementations

### Factory Pattern
- Service creation
- Repository instantiation

## Error Handling Strategy

**Levels**:
1. **Domain Layer**: Business errors (validation, rules)
2. **Infrastructure Layer**: Technical errors (database, network)
3. **Presentation Layer**: HTTP errors (400, 500 series)

**Flow**:
```
Error occurs
    ↓
Wrapped in AppError
    ↓
Classified (retryable/not)
    ↓
Logged (with context)
    ↓
Returned to client (sanitized)
```

## Testing Strategy

**Unit Tests**:
- Test domain logic in isolation
- Mock repositories
- Fast and deterministic

**Integration Tests**:
- Test with real database
- Test HTTP endpoints
- Test worker processes

**Example**:
```go
// Mock repository for testing
type mockPartnerRepo struct {
    partners map[string]*domain.Partner
}

func (m *mockPartnerRepo) GetByID(ctx context.Context, id string) (*domain.Partner, error) {
    if p, ok := m.partners[id]; ok {
        return p, nil
    }
    return nil, errors.NotFound("partner", id)
}
```

## Configuration

**Loading Order**:
1. Default values
2. Configuration file (config.json)
3. Environment variables
4. Command-line flags

**Environment Variables**:
- `FIDEX_NODE_ID`: Node identifier
- `FIDEX_API_KEY`: Internal API key
- `DATABASE_PATH`: SQLite database path

## Security Considerations

**Implemented**:
- No credential logging
- Error message sanitization
- API key authentication
- IP allowlist support
- Session management
- HTTPS support

**Best Practices**:
- Secrets in environment variables
- Private keys secured (600 permissions)
- Public keys shareable
- Structured error logging

## Performance Considerations

**Optimizations**:
- Context-based cancellation
- Connection pooling (future)
- Batch operations (future)
- Caching layer (future)

**Current Limitations**:
- SQLite (single-writer)
- Synchronous processing
- In-memory queue

**Future Improvements**:
- PostgreSQL for multi-writer
- Message queue (Redis/RabbitMQ)
- Horizontal scaling support

## Deployment

**Requirements**:
- Go 1.21+
- SQLite3
- RSA key pair (auto-generated)

**Environment**:
- Docker support
- Systemd service
- Reverse proxy (nginx)

## Monitoring & Observability

**Current**:
- Structured logging
- Request ID tracing
- Error classification

**Future**:
- Metrics (Prometheus)
- Distributed tracing (Jaeger)
- Health checks
- Performance profiling

## Extensibility

**Easy to Add**:
- New repository implementations (PostgreSQL, MySQL)
- New message types
- New endpoints
- New background workers
- New authentication methods

**Extension Points**:
- Repository interfaces
- Service interfaces
- Middleware chain
- Worker plugins

## Conclusion

This architecture provides:
- ✅ Maintainability through clear separation
- ✅ Testability through dependency injection
- ✅ Scalability through interface-based design
- ✅ Reliability through proper error handling
- ✅ Observability through structured logging

The design follows industry best practices and is production-ready.
