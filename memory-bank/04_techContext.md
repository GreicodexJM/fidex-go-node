# Technical Context: FideX AS5 Node

## Technology Stack

### Core Language & Runtime
- **Go 1.24**: Primary programming language
  - Chosen for: Simplicity, performance, excellent concurrency, single binary deployment
  - Standard library provides most networking and crypto needs
  - Static typing reduces runtime errors

### Database
- **SQLite 3** (via `modernc.org/sqlite`)
  - Pure Go implementation (no CGO required)
  - Embedded database, single file
  - WAL mode enabled for better concurrency
  - ACID transactions
  - Foreign keys enabled for referential integrity

### HTTP Framework
- **Chi Router** (`go-chi/chi/v5`)
  - Lightweight, idiomatic Go HTTP router
  - Middleware-based request processing
  - Context-aware routing
  - Sub-router mounting for modular route organization

### Cryptography
- **go-jose** (`go-jose/go-jose/v4`)
  - JSON Web Encryption (JWE) - RSA-OAEP + A256GCM
  - JSON Web Signature (JWS) - RS256
  - JSON Web Key Set (JWKS) support
  - Industry-standard implementations

### WebSocket
- **Gorilla WebSocket** (`gorilla/websocket`)
  - Real-time bi-directional communication
  - Dashboard live updates
  - Hub pattern for broadcasting to multiple clients

### Additional Libraries
- **fsnotify** (`fsnotify/fsnotify`): File system watcher for hot-folder integration
- **uuid** (`google/uuid`): UUID generation for message IDs
- **qrcode** (`skip2/go-qrcode`): QR code generation for security tokens
- **bcrypt** (`golang.org/x/crypto/bcrypt`): Password hashing for user authentication

## Development Setup

### Prerequisites
```bash
# Required
go 1.24+
make (optional, for convenience commands)

# Optional (for development)
air (hot reload)
golangci-lint (linting)
```

### Local Development
```bash
# Install dependencies
go mod download

# Generate RSA keys (first time)
mkdir -p keys
openssl genrsa -out keys/private_key.pem 2048
openssl rsa -in keys/private_key.pem -pubout -out keys/public_key.pem

# Run application
go run ./cmd/fidex-node

# Or with hot reload
air

# Run tests
go test ./...

# Build binary
go build -o fidex-node ./cmd/fidex-node
```

### Environment Configuration

#### Environment Variables
```bash
# Node identity
FIDEX_NODE_ID="urn:gln:my-company:node-123"
FIDEX_ORG_NAME="My Company"
FIDEX_PUBLIC_DOMAIN="node.mycompany.com"

# Ports
FIDEX_INTERNAL_PORT=8080
FIDEX_PUBLIC_PORT=8443

# Security
FIDEX_API_KEY="your-secret-api-key"
FIDEX_ALLOWED_IPS="127.0.0.1,::1"
FIDEX_ENABLE_IP_ALLOWLIST=true

# Paths
FIDEX_PRIVATE_KEY_PATH="./keys/private_key.pem"
FIDEX_PUBLIC_KEY_PATH="./keys/public_key.pem"
FIDEX_DB_PATH="./fidex_local.db"
```

#### Configuration File (config.json)
```json
{
  "node_id": "urn:gln:my-company:node-123",
  "organization_name": "My Company",
  "public_domain": "node.mycompany.com",
  "internal_api_port": 8080,
  "public_api_port": 8443,
  "internal_api_key": "your-secret-api-key",
  "allowed_ip_addresses": ["127.0.0.1", "::1"],
  "enable_ip_allowlist": true,
  "private_key_path": "./keys/private_key.pem",
  "public_key_path": "./keys/public_key.pem",
  "database_path": "./fidex_local.db"
}
```

## Technical Constraints

### Performance
- **Target**: Process 1000 messages/hour on modest hardware
- **Latency**: <2 seconds from API call to database insertion
- **Concurrency**: Support 10+ concurrent inbound/outbound messages

### Security
- **Encryption**: Minimum RSA 2048-bit keys
- **Algorithms**: RS256 (signing), RSA-OAEP (key encryption), A256GCM (content encryption)
- **Sessions**: 24-hour expiry, secure random generation
- **Passwords**: bcrypt hashing with cost factor 12

### Reliability
- **Database**: ACID transactions, WAL mode for crash recovery
- **Message Retry**: Exponential backoff (1m, 5m, 15m, 30m, 1h)
- **Max Retries**: 5 attempts before marking as FAILED
- **Graceful Shutdown**: 30-second timeout for in-flight operations

### Resource Limits
- **Memory**: Target <100MB RSS at idle, <500MB under load
- **Disk**: Database grows ~1KB per message, ~5KB per partner
- **File Descriptors**: Limit concurrent WebSocket connections to 100

## Directory Structure

```
FideXNode/
├── cmd/
│   └── fidex-node/          # Application entry point
│       └── main.go          # Main function
├── internal/                # Internal packages (not importable)
│   ├── api/                 # HTTP handlers and routing
│   │   ├── handlers.go      # Router setup
│   │   ├── middleware.go    # Auth middleware
│   │   ├── *_handlers.go    # Handler implementations
│   │   └── types.go         # Request/response DTOs
│   ├── auth/                # Authentication & sessions
│   │   ├── password.go      # Password hashing
│   │   ├── session.go       # Session management
│   │   └── middleware.go    # Auth middleware
│   ├── config/              # Configuration management
│   │   ├── config.go        # Config loading
│   │   └── config_test.go   # Config tests
│   ├── constants/           # Route constants
│   │   └── routes.go        # Centralized route definitions
│   ├── crypto/              # Encryption/signing
│   │   ├── as5_engine.go    # JWE/JWS operations
│   │   ├── partners.go      # Partner key management
│   │   └── as5_engine_test.go
│   ├── dashboard/           # WebSocket & UI logic
│   │   ├── websocket.go     # WebSocket hub
│   │   └── qrcode.go        # QR code generation
│   ├── db/                  # Database layer
│   │   ├── sqlite.go        # DB initialization & schema
│   │   └── partners.go      # Partner CRUD operations
│   ├── discovery/           # Partner discovery
│   │   ├── service.go       # 4-step handshake logic
│   │   ├── as5_config.go    # AS5 config fetching
│   │   ├── tokens.go        # Security token management
│   │   └── discovery_test.go
│   ├── queue/               # Message queue worker
│   │   └── worker.go        # Retry logic
│   └── watcher/             # File system watcher
│       └── fs_worker.go     # Hot-folder monitoring
├── ui/                      # Web dashboard
│   ├── login.html           # Login page
│   └── dist/                # Built frontend assets
│       ├── index.html       # Dashboard SPA
│       └── js/              # JavaScript modules
├── keys/                    # RSA key pairs (gitignored)
│   ├── private_key.pem
│   └── public_key.pem
├── fidex/                   # Message directories
│   ├── outbox/              # Outgoing messages (hot folder)
│   └── archive/             # Processed messages
├── docs/                    # Documentation
│   ├── openapi.yaml         # API specification
│   └── *.md                 # Additional docs
├── memory-bank/             # Project memory (for Cline)
│   ├── 01_PROJECT_CHARTER.md
│   ├── 02_productContext.md
│   ├── 03_systemPatterns.md
│   ├── 04_techContext.md
│   ├── 05_activeContext.md
│   └── 06_progress.md
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── Makefile                 # Build automation (future)
├── Dockerfile               # Container image (future)
└── README.md                # Project documentation
```

## Dependencies (go.mod)

### Direct Dependencies
```go
require (
    github.com/go-chi/chi/v5 v5.2.5
    github.com/go-jose/go-jose/v4 v4.1.3
    github.com/google/uuid v1.6.0
    github.com/gorilla/websocket v1.5.3
    github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
    modernc.org/sqlite v1.46.1
    golang.org/x/crypto v0.48.0
)
```

### Indirect Dependencies
- Various supporting libraries for SQLite, crypto, etc.

## Tool Usage Patterns

### Building
```bash
# Development build
go build -o fidex-node ./cmd/fidex-node

# Production build (optimized)
go build -ldflags="-s -w" -o fidex-node ./cmd/fidex-node

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o fidex-node-linux ./cmd/fidex-node
```

### Testing
```bash
# Run all tests
go test ./...

# Verbose output
go test -v ./...

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific package
go test -v ./internal/discovery/

# Specific test
go test -v ./internal/discovery/ -run TestCompleteDiscoveryHandshake
```

### Linting
```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# golangci-lint (recommended)
golangci-lint run
```

### Running
```bash
# Default configuration
./fidex-node

# With config file
./fidex-node -config config.json

# With command-line flags
./fidex-node -internal-port 8080 -public-port 8443 -node-id "urn:gln:test:node"

# With environment variables
export FIDEX_NODE_ID="urn:gln:test:node"
export FIDEX_INTERNAL_PORT=8080
./fidex-node
```

## Current Technical Debt

### High Priority
1. **Global Variables**: `DB`, `AppConfig`, `NodeConfig` need removal
2. **Configuration Bug**: `flag.Parse()` called multiple times
3. **No Context Propagation**: Repository methods don't accept `context.Context`
4. **Missing Interfaces**: No abstractions for repositories or services

### Medium Priority
1. **Error Handling**: Inconsistent patterns (log vs return vs both)
2. **No Structured Logging**: Using `log.Printf` directly
3. **Security Issue**: API key logged in plain text
4. **Incomplete Queue Worker**: Mock delivery logic not implemented

### Low Priority
1. **Code Duplication**: Multiple `respondWithJSON` implementations
2. **No Metrics**: No Prometheus/monitoring instrumentation
3. **Limited Tests**: Need more integration and e2e tests

## Future Technical Considerations

### Phase 2 Enhancements
- Add structured logging (zerolog or zap)
- Implement metrics collection (Prometheus)
- Add distributed tracing (OpenTelemetry)
- Health check improvements (liveness/readiness probes)

### Phase 3 Enhancements
- Docker multi-stage builds
- Kubernetes manifests
- CI/CD pipeline (GitHub Actions)
- Performance profiling and optimization
- Comprehensive integration test suite
