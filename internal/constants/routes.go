package constants

// API Route Constants
// These constants define all API endpoint paths to ensure consistency
// between route configuration and discovery document generation

const (
	// External B2B API Routes (Public-facing)
	// IMPORTANT: these absolute paths MUST match how the public router actually
	// mounts the inbound/receipt/register handlers (see handlers.go, which
	// composes APIV1 + RouteInboundRel etc.). GenerateAS5Config publishes
	// these constants so partner peers will POST here directly.
	RouteHealth           = "/health"
	RouteInbound          = "/api/v1/inbound"
	RouteReceipt          = "/api/v1/receipt"
	RouteRegister         = "/api/v1/register"
	RouteJWKS             = "/.well-known/jwks.json"
	RouteAS5Configuration = "/.well-known/as5-configuration"

	// Internal API Routes (Localhost only)
	RouteRoot      = "/"
	RouteLogin     = "/login"
	RouteDashboard = "/dashboard"

	// Static Assets
	RouteJS         = "/js/*"
	RouteCSS        = "/css/*"
	RouteComponents = "/components/*"

	// API Base Paths
	APIAuth      = "/admin/auth"
	APIDashboard = "/admin/dashboard"
	APISettings  = "/admin/settings"
	APIV1        = "/api/v1"

	// Authentication Routes (relative to /admin/auth)
	RouteAuthLogin  = "/login"
	RouteAuthLogout = "/logout"
	RouteAuthMe     = "/me"

	// Dashboard Routes (relative to /admin/dashboard)
	RouteDashboardWS       = "/ws"
	RouteDashboardQR       = "/qr"
	RouteDashboardMetrics  = "/metrics"
	RouteDashboardMessages = "/messages"
	RouteDashboardPartners = "/partners"
	RouteDiscover          = "/partners/discover"

	// Settings Routes (relative to /admin/settings)
	RouteConfig      = "/config"
	RouteRotateKeys  = "/rotate-keys"
	RouteUsers       = "/users"
	RouteUserByID    = "/users/{id}"
	RoutePassword    = "/password"
	RoutePartners    = "/partners"
	RoutePartnerByID = "/partners/{id}"

	// ERP Integration & B2B Routes (relative to /api/v1)
	RouteTransmitRel = "/transmit"
	RouteInboundRel  = "/inbound"
	RouteReceiptRel  = "/receipt"
	RouteRegisterRel = "/register"
)
