package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"fidex-node/internal/auth"
	"fidex-node/internal/config"
	"fidex-node/internal/discovery"
	"fidex-node/internal/domain"
)

// Transitional package-level dependencies injected from main.go after
// container initialization. These will be folded into an APIHandlers
// struct in Phase 1.4 of the refactor.
var (
	// NodeConfig is the application config set from main
	NodeConfig *config.Config
	// DiscoveryService is the discovery service instance
	DiscoveryService *discovery.DiscoveryService
	// AuthSvc provides session / user authentication operations
	AuthSvc *auth.Service
	// MessageRepo persists FideX messages
	MessageRepo domain.MessageRepository
	// PartnerRepo persists trading partner profiles
	PartnerRepo domain.PartnerRepository
	// UserRepo persists dashboard users
	UserRepo domain.UserRepository
	// SessionRepo persists dashboard sessions
	SessionRepo domain.SessionRepository
	// DB exposes the raw connection for handlers that need ad-hoc queries
	// not yet covered by repository interfaces (dashboard metrics, settings
	// listings). Tracked as deuda técnica for Phase 1.4+.
	DB *sql.DB
)

// as5ConfigHandler handles GET /.well-known/as5-configuration
// Returns the node's AS5 discovery document
func as5ConfigHandler(w http.ResponseWriter, r *http.Request) {
	if NodeConfig == nil {
		log.Printf("Node config not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	discConfig := discovery.NodeConfig{
		NodeID:           NodeConfig.NodeID,
		OrganizationName: NodeConfig.OrganizationName,
		BaseURL:          getBaseURL(r),
	}

	as5Config := discovery.GenerateAS5Config(discConfig)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(as5Config)

	log.Printf("AS5 configuration requested from %s", r.RemoteAddr)
}

// webhookRegistrationHandler handles POST /as5/onboarding/webhook
// Accepts registration requests from partners during discovery handshake
func webhookRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("Received webhook registration request from %s", r.RemoteAddr)

	var req discovery.RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Failed to decode registration request: %v", err)
		respondWithJSONError(w, http.StatusBadRequest, "Invalid JSON in request body")
		return
	}

	if DiscoveryService == nil {
		log.Printf("Discovery service not initialized")
		respondWithJSONError(w, http.StatusInternalServerError, "Service not available")
		return
	}

	// Process the registration
	if err := DiscoveryService.HandleWebhookRegistration(r.Context(), req); err != nil {
		log.Printf("Failed to process registration: %v", err)
		respondWithJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Return success
	response := discovery.RegistrationResponse{
		Success: true,
		Message: "Partner registered successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Partner registration successful: %s", req.OrganizationName)
}

// respondWithJSONError sends a JSON error response
func respondWithJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": message,
	})
}
