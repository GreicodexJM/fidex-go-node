# API Package Structure

This package implements the FideX Node API following clean architecture principles with separation of concerns.

## File Organization

### handlers.go
**Purpose:** Router configuration and setup
- `SetupInternalRouter()` - Configures ERP-facing internal API routes
- `SetupPublicRouter()` - Configures B2B-facing external API routes

### types.go
**Purpose:** Shared data types, validation, and utility functions
- Request/Response types (`TransmitRequest`, `TransmitResponse`, etc.)
- Envelope structures (`FidexEnvelope`, `JmdnEnvelope`, etc.)
- Validation functions (`validateTransmitRequest`, `validateFidexEnvelope`, etc.)
- Utility functions (`respondWithError`)

### internal_handlers.go
**Purpose:** Internal API handlers (ERP-facing)
- `transmitHandler()` - Handles outbound message transmission from local ERP systems

### external_handlers.go
**Purpose:** External API handlers (B2B-facing)
- `healthHandler()` - Health check endpoint
- `inboundHandler()` - Receives FideX envelopes from trading partners
- `receiptHandler()` - Processes message disposition notifications (MDN)
- `jwksHandler()` - Serves JSON Web Key Set for public key discovery

### Other Handler Files
- `auth_handlers.go` - Authentication and session management
- `dashboard_handlers.go` - Dashboard and WebSocket handlers
- `settings_handlers.go` - Settings management endpoints
- `ui_handlers.go` - UI static asset serving
- `discovery_handlers.go` - AS5 discovery and partner registration
- `middleware.go` - Custom middleware (IP allowlist, API key auth)

## Architecture Benefits

### Separation of Concerns
- **Internal handlers**: Secured with IP allowlist + API key, handles local ERP communication
- **External handlers**: Public-facing, handles B2B partner communication
- **Types**: Centralized data structures and validation logic
- **Routers**: Clean configuration without business logic

### Maintainability
- Single Responsibility Principle: Each file has a focused purpose
- Easy to locate and modify specific functionality
- Clear boundaries between internal and external APIs

### Testability
- Handlers can be tested independently
- Validation functions are isolated and testable
- Type definitions are centralized

### Scalability
- Easy to add new internal or external endpoints
- Clear pattern for future handler additions
- Middleware can be composed independently

## API Routes

### Internal API (Port 8080, localhost only)
- `POST /api/v1/transmit` - Send message to trading partner
- `GET /dashboard` - Dashboard UI
- `GET /login` - Login page
- UI static assets

### External API (Port 8443, public)
- `GET /health` - Health check
- `POST /api/v1/inbound` - Receive FideX envelope
- `POST /api/v1/receipt` - Receive message receipt
- `GET /.well-known/jwks.json` - Public key discovery
- `GET /.well-known/as5-configuration` - AS5 discovery
- `POST /as5/onboarding/webhook` - Partner registration webhook
