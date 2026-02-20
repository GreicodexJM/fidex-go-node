# API Package Structure

This package implements the FideX Node API following clean architecture principles with separation of concerns.

## File Organization

### Core Router Configuration

#### handlers.go
**Purpose:** Main router configuration and setup
- `SetupInternalRouter()` - Configures ERP-facing internal API routes (port 8080)
- `SetupPublicRouter()` - Configures B2B-facing external API routes (port 8443)

### Shared Types and Utilities

#### types.go
**Purpose:** Shared data types, validation, and utility functions
- Request/Response types (`TransmitRequest`, `TransmitResponse`, etc.)
- Envelope structures (`FidexEnvelope`, `JmdnEnvelope`, etc.)
- Validation functions (`validateTransmitRequest`, `validateFidexEnvelope`, etc.)
- Utility functions (`respondWithError`)

### Handler Groups

#### internal_handlers.go
**Purpose:** ERP Integration API handlers
- `transmitHandler()` - POST /api/v1/transmit - Outbound message transmission

#### external_handlers.go
**Purpose:** B2B API handlers (Trading Partners)
- `healthHandler()` - GET /health - Health check
- `inboundHandler()` - POST /api/v1/inbound - Receives FideX envelopes
- `receiptHandler()` - POST /api/v1/receipt - Processes MDN receipts
- `jwksHandler()` - GET /.well-known/jwks.json - Public key discovery

#### webui_handlers.go
**Purpose:** WebUI HTML pages and static assets
- `serveLoginHandler()` - GET /login - Login page
- `serveDashboardHandler()` - GET /dashboard - Dashboard page
- `serveStaticAssets()` - GET /js/*, /css/*, /components/* - Static files
- `getBaseURL()` - Utility for extracting base URL from requests

#### auth_handlers.go
**Purpose:** Authentication and session management
- `loginHandler()` - POST /api/auth/login - User login
- `logoutHandler()` - POST /api/auth/logout - User logout
- `meHandler()` - GET /api/auth/me - Current user info
- `InitializeDefaultUser()` - Creates default admin user

#### dashboard_handlers.go
**Purpose:** Dashboard data and real-time updates
- `qrCodeHandler()` - GET /api/dashboard/qr - QR code for node discovery
- `metricsHandler()` - GET /api/dashboard/metrics - System metrics
- `messagesHandler()` - GET /api/dashboard/messages - Message list
- `partnersHandler()` - GET /api/dashboard/partners - Partner list
- `discoverPartnerHandler()` - POST /api/dashboard/partners/discover - Auto-discover partner
- `websocketHandler()` - GET /api/dashboard/ws - WebSocket connection
- `InitializeWebSocketHub()` - WebSocket hub initialization

#### settings_handlers.go
**Purpose:** Node configuration and management
- `getConfigHandler()` - GET /api/settings/config - Node configuration
- `rotateKeysHandler()` - POST /api/settings/rotate-keys - Key rotation
- `listUsersHandler()` - GET /api/settings/users - User list
- `createUserHandler()` - POST /api/settings/users - Create user
- `deleteUserHandler()` - DELETE /api/settings/users/:id - Delete user
- `updatePasswordHandler()` - PUT /api/settings/password - Change password
- `listPartnersDetailedHandler()` - GET /api/settings/partners - Detailed partner list
- `updatePartnerHandler()` - PUT /api/settings/partners/:id - Update partner
- `deletePartnerHandler()` - DELETE /api/settings/partners/:id - Delete partner

#### discovery_handlers.go
**Purpose:** AS5 discovery protocol
- `as5ConfigHandler()` - GET /.well-known/as5-configuration - AS5 discovery document
- `webhookRegistrationHandler()` - POST /api/v1/register - Partner registration

#### middleware.go
**Purpose:** Custom middleware
- `IPAllowlistMiddleware()` - IP address filtering
- `APIKeyMiddleware()` - API key authentication

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

#### WebUI Pages
- `GET /` - Redirect to /login
- `GET /login` - Login page
- `GET /dashboard` - Dashboard UI
- `GET /js/*` - JavaScript assets
- `GET /css/*` - CSS stylesheets
- `GET /components/*` - HTML components

#### Authentication API
- `POST /api/auth/login` - User login
- `POST /api/auth/logout` - User logout
- `GET /api/auth/me` - Get current user

#### Dashboard API (Real-time data)
- `GET /api/dashboard/ws` - WebSocket connection
- `GET /api/dashboard/qr` - QR code for discovery
- `GET /api/dashboard/metrics` - System metrics
- `GET /api/dashboard/messages` - Message list (paginated)
- `GET /api/dashboard/partners` - Partner list
- `POST /api/dashboard/partners/discover` - Auto-discover partner

#### Settings API (Configuration & Management)
- `GET /api/settings/config` - Node configuration
- `POST /api/settings/rotate-keys` - Rotate cryptographic keys
- `GET /api/settings/users` - List users
- `POST /api/settings/users` - Create user
- `DELETE /api/settings/users/:id` - Delete user
- `PUT /api/settings/password` - Change password
- `GET /api/settings/partners` - List partners (detailed)
- `PUT /api/settings/partners/:id` - Update partner
- `DELETE /api/settings/partners/:id` - Delete partner

#### ERP Integration API (API Key Protected)
- `POST /api/v1/transmit` - Send message to trading partner

### External API (Port 8443, public-facing)

#### Health & Monitoring
- `GET /health` - Health check

#### B2B Messaging
- `POST /api/v1/inbound` - Receive FideX envelope from partner
- `POST /api/v1/receipt` - Receive message receipt (MDN)
- `POST /api/v1/register` - Partner registration (discovery handshake)

#### Discovery & Keys
- `GET /.well-known/jwks.json` - Public key discovery (JWKS)
- `GET /.well-known/as5-configuration` - AS5 discovery document
