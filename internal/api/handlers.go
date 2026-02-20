package api

import (
	"net/http"
	"time"

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

	// UI routes (no API key required)
	r.Get("/login", serveLoginHandler)
	r.Get("/dashboard", serveDashboardHandler)
	r.Get("/js/*", serveStaticAssets)
	r.Get("/css/*", serveStaticAssets)
	r.Get("/components/*", serveStaticAssets)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// Protected API routes (require API key)
	r.Route("/api/v1", func(r chi.Router) {
		if apiKey != "" {
			r.Use(APIKeyMiddleware(apiKey))
		}
		r.Post("/transmit", transmitHandler)
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
	r.Get("/health", healthHandler)

	// Public B2B API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/inbound", inboundHandler)
		r.Post("/receipt", receiptHandler)
	})

	// JWKS discovery endpoint
	r.Get("/.well-known/jwks.json", jwksHandler)

	// AS5 Discovery endpoint
	r.Get("/.well-known/as5-configuration", as5ConfigHandler)

	// Partner registration webhook
	r.Post("/as5/onboarding/webhook", webhookRegistrationHandler)

	return r
}
