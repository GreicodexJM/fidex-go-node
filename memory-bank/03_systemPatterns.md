# System Patterns: FideX AS5 Node

## Architectural Style: Hexagonal Architecture (Ports & Adapters)

### Core Principle
**Business logic is independent of external systems**. The domain core contains pure business rules and is isolated from infrastructure concerns like HTTP, databases, or file systems.

### Architecture Layers

```
┌─────────────────────────────────────────────────────────┐
│                     Adapters (Input)                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ REST API     │  │  Web UI      │  │ File Watcher │ │
│  │ Handlers     │  │  Dashboard   │  │              │ │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘ │
└─────────┼──────────────────┼──────────────────┼─────────┘
          │                  │                  │
          └──────────────────┼──────────────────┘
                             │
┌────────────────────────────┼─────────────────────────────┐
│                    Ports (Interfaces)                    │
│  • MessageService    • PartnerService                   │
│  • CryptoService     • QueueService                     │
└────────────────────────────┼─────────────────────────────┘
                             │
┌────────────────────────────┼─────────────────────────────┐
│                     Domain Core                          │
│  • Business Logic    • Domain Models                    │
│  • Validation        • Domain Events                    │
└────────────────────────────┼─────────────────────────────┘
                             │
┌────────────────────────────┼─────────────────────────────┐
│                    Ports (Interfaces)                    │
│  • MessageRepository  • PartnerRepository               │
│  • KeyStore          • Logger                           │
└────────────────────────────┼─────────────────────────────┘
          ┌──────────────────┼──────────────────┐
          │                  │                  │
┌─────────┼──────────────────┼──────────────────┼─────────┐
│                    Adapters (Output)                     │
│  ┌──────┴───────┐  ┌──────┴───────┐  ┌──────┴───────┐ │
│  │   SQLite     │  │  File System │  │  HTTP Client │ │
│  │  Repository  │  │   (Keys)     │  │  (B2B Comms) │ │
│  └──────────────┘  └──────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────┘
```

## SOLID Principles Applied

### 1. Single Responsibility Principle (SRP)
**Each component has one reason to change.**

**Examples:**
- `MessageRepository`: Only handles message persistence
- `CryptoService`: Only handles encryption/decryption
- `QueueWorker`: Only handles message delivery retry logic
- `DiscoveryService`: Only handles partner onboarding workflow

**Anti-pattern (Being Fixed):**
- ❌ `main.go` currently does too much (config, DB, keys, servers)
- ✅ Refactoring into: `mustLoadConfig()`, `mustInitializeServices()`, etc.

### 2. Open/Closed Principle (OCP)
**Open for extension, closed for modification.**

**Examples:**
- Repository interfaces allow swapping SQLite for PostgreSQL without changing business logic
- Middleware stack is extensible (add new auth methods without modifying existing)
- Crypto service can support new algorithms by extending, not modifying

**Implementation:**
```go
// Open for extension via interfaces
type MessageRepository interface {
    Insert(ctx context.Context, msg *Message) error
    // New methods can be added
}

// Closed for modification - clients depend on interface
func NewMessageService(repo MessageRepository) *MessageService {
    return &MessageService{repo: repo}
}
```

### 3. Liskov Substitution Principle (LSP)
**Subtypes must be substitutable for their base types.**

**Examples:**
- Any `MessageRepository` implementation (SQLite, Postgres, Memory) can be used interchangeably
- All middleware functions follow `func(http.Handler) http.Handler` contract
- All repository methods accept `context.Context` for cancellation

**Testing:**
```go
// Test with mock repository
mockRepo := &MockMessageRepository{...}
service := NewMessageService(mockRepo)
// Service works identically with mock or real implementation
```

### 4. Interface Segregation Principle (ISP)
**Clients shouldn't depend on interfaces they don't use.**

**Examples:**
- Split large repository into focused interfaces:
  - `MessageReader` (read-only operations)
  - `MessageWriter` (write operations)
  - `MessageRepository` (combines both for full access)
  
**Implementation:**
```go
// Focused interface for read-only use cases
type MessageReader interface {
    GetByID(ctx context.Context, id string) (*Message, error)
    GetQueued(ctx context.Context) ([]Message, error)
}

// Dashboard only needs to read, not write
type Dashboard struct {
    reader MessageReader  // Not full MessageRepository
}
```

### 5. Dependency Inversion Principle (DIP)
**Depend on abstractions, not concretions.**

**Before (Anti-pattern):**
```go
// ❌ Direct dependency on concrete implementation
func transmitHandler(w http.ResponseWriter, r *http.Request) {
    db.DB.Exec(...)  // Depends on global concrete DB
}
```

**After (Proper DIP):**
```go
// ✅ Depends on abstraction
type TransmitHandler struct {
    messageRepo domain.MessageRepository  // Interface
    cryptoSvc   domain.CryptoService      // Interface
}

func (h *TransmitHandler) Handle(w http.ResponseWriter, r *http.Request) {
    h.messageRepo.Insert(...)  // Works with any implementation
}
```

## Key Design Patterns

### 1. Repository Pattern
**Purpose**: Abstract data access logic

```go
type MessageRepository interface {
    Insert(ctx context.Context, msg *Message) error
    GetByID(ctx context.Context, id string) (*Message, error)
    GetQueued(ctx context.Context) ([]Message, error)
    UpdateStatus(ctx context.Context, id string, status MessageStatus) error
}

type SQLiteMessageRepository struct {
    db *sql.DB
}

func (r *SQLiteMessageRepository) Insert(ctx context.Context, msg *Message) error {
    // SQLite-specific implementation
}
```

### 2. Dependency Injection Container
**Purpose**: Centralized service composition

```go
type Container struct {
    Config           *config.Config
    MessageRepo      domain.MessageRepository
    PartnerRepo      domain.PartnerRepository
    CryptoService    domain.CryptoService
    QueueWorker      *queue.Worker
    DiscoveryService *discovery.DiscoveryService
}

func NewContainer(cfg *config.Config) (*Container, error) {
    // Wire all dependencies here
    db := initDB(cfg.DatabasePath)
    messageRepo := db.NewMessageRepository(db)
    cryptoSvc := crypto.NewService(cfg.PrivateKeyPath)
    
    return &Container{
        Config:      cfg,
        MessageRepo: messageRepo,
        CryptoService: cryptoSvc,
        // ... etc
    }, nil
}
```

### 3. Middleware Chain Pattern
**Purpose**: Composable request processing

```go
// Middleware functions are composable
r.Use(middleware.Logger)
r.Use(middleware.Recoverer)
r.Use(IPAllowlistMiddleware(allowedIPs))
r.Use(APIKeyMiddleware(apiKey))

// Order matters - executed in sequence
```

### 4. Strategy Pattern (Implicit)
**Purpose**: Swappable algorithms

```go
// Different crypto strategies
type CryptoService interface {
    Encrypt(data []byte, key *rsa.PublicKey) (string, error)
}

type JWECryptoService struct{...}  // JWE strategy
type GPGCryptoService struct{...}  // Alternative strategy

// Client code doesn't care which strategy is used
```

### 5. Observer Pattern (WebSocket Hub)
**Purpose**: Real-time event broadcasting

```go
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
}

// Broadcasts to all connected dashboard clients
hub.Broadcast(messageEvent)
```

## Component Relationships

### Critical Implementation Paths

#### Path 1: Outbound Message Flow
```
ERP/File → Handler → MessageService → CryptoService → MessageRepository
                                           ↓
                                      QueueWorker → HTTP Client → Partner
```

#### Path 2: Inbound Message Flow
```
Partner → Handler → CryptoService → MessageRepository → ERP Webhook
                         ↓
                   Create J-MDN Receipt → Queue → Partner
```

#### Path 3: Partner Discovery Flow
```
Dashboard → Handler → DiscoveryService → HTTP Client (fetch config/JWKS)
                           ↓
                      PartnerRepository (save partner)
```

## Architectural Decisions

### ADR-001: Use SQLite for Persistence
**Context**: Need simple, reliable persistence without external dependencies
**Decision**: Use SQLite with WAL mode
**Consequences**: 
- ✅ Zero configuration, single file
- ✅ ACID guarantees
- ❌ Not suitable for multi-node clustering (acceptable for edge node)

### ADR-002: Synchronous HTTP for B2B Communication
**Context**: Need reliable delivery between trading partners
**Decision**: Use synchronous HTTP POST with retry logic
**Consequences**:
- ✅ Simple to implement and debug
- ✅ Compatible with firewalls and load balancers
- ❌ No built-in message queuing (mitigated with local queue)

### ADR-003: Chi Router for HTTP Routing
**Context**: Need lightweight, composable router
**Decision**: Use go-chi/chi for HTTP routing
**Consequences**:
- ✅ Idiomatic Go, middleware-based
- ✅ Context-aware, good performance
- ✅ Clear route grouping and mounting

### ADR-004: JWE/JWS for Message Security
**Context**: Need industry-standard encryption without proprietary formats
**Decision**: Use JSON Web Encryption (JWE) and JSON Web Signature (JWS)
**Consequences**:
- ✅ Standard cryptographic primitives
- ✅ Interoperable with other systems
- ✅ Good library support (go-jose)

### ADR-005: Hexagonal Architecture
**Context**: Need testable, maintainable codebase
**Decision**: Implement hexagonal architecture with dependency injection
**Consequences**:
- ✅ Business logic independent of infrastructure
- ✅ Easy to mock dependencies for testing
- ✅ Clear boundaries between layers
- ❌ More upfront design effort (acceptable tradeoff)

## Refactoring Goals (Phase 1)

### Eliminate Global State
- ❌ Remove `var DB *sql.DB` from db package
- ❌ Remove `var AppConfig *config.Config` from main
- ❌ Remove `var NodeConfig *config.Config` from api package
- ✅ Pass dependencies through constructors

### Introduce Interfaces
- ✅ Define repository interfaces in domain package
- ✅ Define service interfaces in domain package
- ✅ Implementations live in infrastructure packages

### Dependency Injection
- ✅ Create Container to wire all dependencies
- ✅ Pass Container to HTTP handlers (via closure or struct)
- ✅ No direct package-level function calls between layers

### Context Propagation
- ✅ All repository methods accept `context.Context`
- ✅ Pass request context through layers
- ✅ Enable timeout and cancellation propagation
