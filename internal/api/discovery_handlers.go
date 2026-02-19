package api

import (
	"encoding/json"
	"log"
	"net/http"

	"fidex-node/internal/config"
	"fidex-node/internal/discovery"
)

// NodeConfig is the application config set from main
var NodeConfig *config.Config

// DiscoveryService is the discovery service instance
var DiscoveryService *discovery.DiscoveryService

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

// getBaseURL extracts the base URL from the request
// This will be used to construct absolute URLs in the discovery document
func getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	// Check for X-Forwarded-Proto header (if behind a proxy)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	host := r.Host
	if host == "" {
		host = "localhost:8443"
	}

	return scheme + "://" + host
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
	if err := DiscoveryService.HandleWebhookRegistration(req); err != nil {
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
