package api

import (
	"encoding/json"
	"net/http"

	"fidex-node/internal/discovery"
)

// as5ConfigHandler handles GET /.well-known/as5-configuration
// Returns the node's AS5 discovery document
func (h *Handlers) as5ConfigHandler(w http.ResponseWriter, r *http.Request) {
	if h.Config == nil {
		logger.Error(r.Context(), "Node config not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	discConfig := discovery.NodeConfig{
		NodeID:           h.Config.NodeID,
		OrganizationName: h.Config.OrganizationName,
		BaseURL:          getBaseURL(r),
	}

	as5Config := discovery.GenerateAS5Config(discConfig)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(as5Config)

	logger.Info(r.Context(), "AS5 configuration requested from %s", r.RemoteAddr)
}

// webhookRegistrationHandler handles POST /as5/onboarding/webhook
// Accepts registration requests from partners during discovery handshake
func (h *Handlers) webhookRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logger.Info(r.Context(), "Received webhook registration request from %s", r.RemoteAddr)

	var req discovery.RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn(r.Context(), "Failed to decode registration request: %v", err)
		respondWithJSONError(w, http.StatusBadRequest, "Invalid JSON in request body")
		return
	}

	if h.DiscoveryService == nil {
		logger.Error(r.Context(), "Discovery service not initialized")
		respondWithJSONError(w, http.StatusInternalServerError, "Service not available")
		return
	}

	// Process the registration
	if err := h.DiscoveryService.HandleWebhookRegistration(r.Context(), req); err != nil {
		logger.Warn(r.Context(), "Failed to process registration: %v", err)
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

	logger.Info(r.Context(), "Partner registration successful: %s", req.OrganizationName)
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
