package api

import (
	"fidex-node/internal/auth"
	"fidex-node/internal/config"
	"fidex-node/internal/crypto"
	"fidex-node/internal/dashboard"
	"fidex-node/internal/discovery"
	"fidex-node/internal/domain"
	"fidex-node/internal/logging"
)

// logger is the package-level structured logger used by all api handlers,
// middleware, and routers. It honours request_id / user_id carried in
// request contexts (see internal/api/context.go and internal/logging).
var logger = logging.New("api")

// Handlers carries all dependencies required by HTTP handlers in this package.
// All persistence flows through the repository interfaces — there is no raw
// *sql.DB exposed here. Construct one Handlers from the service container in
// main, then call its Setup*Router methods to obtain wired chi routers.
type Handlers struct {
	Config           *config.Config
	MessageRepo      domain.MessageRepository
	PartnerRepo      domain.PartnerRepository
	UserRepo         domain.UserRepository
	SessionRepo      domain.SessionRepository
	AuthService      *auth.Service
	CryptoService    *crypto.AS5Engine
	DiscoveryService *discovery.DiscoveryService
	WebSocketHub     *dashboard.Hub
}
