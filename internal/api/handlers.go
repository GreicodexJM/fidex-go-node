package api

import (
	"net/http"
	"time"

	"fidex-node/internal/constants"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// SetupInternalRouter configures the internal API routes (ERP-facing)
// This router should be bound to localhost only and protected with IP allowlist + API key
func SetupInternalRouter(allowedIPs []string, apiKey string, enableIPAllowlist bool) *chi.Mux {
	r := chi.NewRouter()

	// Common middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))

	// Apply IP allowlist globally
	if enableIPAllowlist && len(allowedIPs) > 0 {
		r.Use(IPAllowlistMiddleware(allowedIPs))
	}

	// Root redirect to login
	r.Get(constants.RouteRoot, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, constants.RouteLogin, http.StatusSeeOther)
	})

	// ========================================
	// WebUI - HTML Pages (no auth required on these routes, handled by pages)
	// ========================================
	r.Get(constants.RouteLogin, serveLoginHandler)
	r.Get(constants.RouteDashboard, serveDashboardHandler)

	// Static assets (JS, CSS, components)
	r.Get(constants.RouteJS, serveStaticAssets)
	r.Get(constants.RouteCSS, serveStaticAssets)
	r.Get(constants.RouteComponents, serveStaticAssets)

	// ========================================
	// API Routes - Organized by functionality
	// ========================================

	// Constants API (public access for frontend)
	r.Get("/api/constants", constantsHandler)

	// Note: Auth, Dashboard, and Settings routes are mounted separately in main.go
	// to allow for modular router composition

	// ERP Integration API (protected with API key)
	r.Route(constants.APIV1, func(r chi.Router) {
		if apiKey != "" {
			r.Use(APIKeyMiddleware(apiKey))
		}
		r.Post(constants.RouteTransmitRel, transmitHandler)
	})

	return r
}

// SetupPublicRouter configures the public API routes (B2B-facing)
// This router should be accessible from external networks
func SetupPublicRouter() *chi.Mux {
	r := chi.NewRouter()

	// Common middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check
	r.Get(constants.RouteHealth, healthHandler)

	// Public B2B API routes
	r.Route(constants.APIV1, func(r chi.Router) {
		r.Post(constants.RouteInboundRel, inboundHandler)
		r.Post(constants.RouteReceiptRel, receiptHandler)
		r.Post(constants.RouteRegisterRel, webhookRegistrationHandler)
	})

	// Discovery endpoints
	r.Get(constants.RouteJWKS, jwksHandler)
	r.Get(constants.RouteAS5Configuration, as5ConfigHandler)

	return r
}
