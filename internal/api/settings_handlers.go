package api

import (
	"encoding/json"
	"net/http"
	"os"

	"fidex-node/internal/auth"
	"fidex-node/internal/crypto"

	"github.com/go-chi/chi/v5"
)

// ConfigResponse represents the node configuration (read-only from externalized config)
type ConfigResponse struct {
	NodeID             string   `json:"node_id"`
	OrganizationName   string   `json:"organization_name"`
	PublicDomain       string   `json:"public_domain"`
	InternalAPIPort    int      `json:"internal_api_port"`
	PublicAPIPort      int      `json:"public_api_port"`
	AllowedIPAddresses []string `json:"allowed_ip_addresses"`
	EnableIPAllowlist  bool     `json:"enable_ip_allowlist"`
	HasPrivateKey      bool     `json:"has_private_key"`
	PublicKeyPath      string   `json:"public_key_path"`
}

// UserResponse represents a user
type UserResponse struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

// CreateUserRequest represents user creation request
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateUserPasswordRequest represents password update request
type UpdateUserPasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// PartnerResponse represents a trading partner
type PartnerResponse struct {
	ID             int64  `json:"id"`
	PartnerID      string `json:"partner_id"`
	Name           string `json:"name"`
	JWKSUrl        string `json:"jwks_url"`
	LastKeyRefresh string `json:"last_key_refresh,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// UpdatePartnerRequest represents partner update request
type UpdatePartnerRequest struct {
	Name string `json:"name"`
}

// getConfigHandler handles GET /api/settings/config
// Returns the current node configuration (read-only, externalized from DB)
func (h *Handlers) getConfigHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.Config == nil {
		logger.Error(ctx, "Node config not initialized")
		respondWithError(w, http.StatusInternalServerError, "Configuration not available", nil)
		return
	}

	// Check if private key file exists
	hasPrivateKey := false
	if _, err := os.Stat(h.Config.PrivateKeyPath); err == nil {
		hasPrivateKey = true
	}

	response := ConfigResponse{
		NodeID:             h.Config.NodeID,
		OrganizationName:   h.Config.OrganizationName,
		PublicDomain:       h.Config.PublicDomain,
		InternalAPIPort:    h.Config.InternalAPIPort,
		PublicAPIPort:      h.Config.PublicAPIPort,
		AllowedIPAddresses: h.Config.AllowedIPAddresses,
		EnableIPAllowlist:  h.Config.EnableIPAllowlist,
		HasPrivateKey:      hasPrivateKey,
		PublicKeyPath:      h.Config.PublicKeyPath,
	}

	respondWithJSON(w, http.StatusOK, response)
}

// rotateKeysHandler handles POST /api/settings/rotate-keys
// Generates new RSA key pair and saves to file system
func (h *Handlers) rotateKeysHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.Config == nil {
		logger.Error(ctx, "Node config not initialized")
		respondWithError(w, http.StatusInternalServerError, "Configuration not available", nil)
		return
	}

	logger.Info(ctx, "Generating new RSA key pair...")

	// Generate new key pair
	privateKeyPEM, publicKeyPEM, err := crypto.GenerateKeyPair()
	if err != nil {
		logger.Error(ctx, "Failed to generate key pair: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to generate keys", err)
		return
	}

	// Save private key to file
	if err := os.WriteFile(h.Config.PrivateKeyPath, []byte(privateKeyPEM), 0600); err != nil {
		logger.Error(ctx, "Failed to save private key: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save private key", err)
		return
	}

	// Save public key to file
	if err := os.WriteFile(h.Config.PublicKeyPath, []byte(publicKeyPEM), 0644); err != nil {
		logger.Error(ctx, "Failed to save public key: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save public key", err)
		return
	}

	logger.Info(ctx, "RSA key pair rotated successfully")

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message":    "Keys rotated successfully",
		"public_key": publicKeyPEM,
	})
}

// listUsersHandler handles GET /api/settings/users
func (h *Handlers) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.UserRepo.List(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to load users: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to load users", err)
		return
	}

	users := make([]UserResponse, 0, len(rows))
	for _, u := range rows {
		users = append(users, UserResponse{
			ID:        u.ID,
			Username:  u.Username,
			CreatedAt: u.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"users": users,
	})
}

// createUserHandler handles POST /api/settings/users
func (h *Handlers) createUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if req.Username == "" || req.Password == "" {
		respondWithError(w, http.StatusBadRequest, "Username and password are required", nil)
		return
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		logger.Error(ctx, "Failed to hash password: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to create user", err)
		return
	}

	// Create user
	user, err := h.AuthService.CreateUser(ctx, req.Username, hashedPassword)
	if err != nil {
		logger.Error(ctx, "Failed to create user: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to create user (username may already exist)", err)
		return
	}

	logger.Info(ctx, "User created successfully: %s", req.Username)

	respondWithJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"user": UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		},
	})
}

// deleteUserHandler handles DELETE /api/settings/users/:id
func (h *Handlers) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userIDParam := chi.URLParam(r, "id")
	userID := parseInt64(userIDParam)

	// Don't allow deleting yourself
	if currentUser, ok := auth.GetUserFromContext(ctx); ok && currentUser.ID == userID {
		respondWithError(w, http.StatusBadRequest, "Cannot delete your own account", nil)
		return
	}

	if err := h.UserRepo.Delete(ctx, userID); err != nil {
		logger.Error(ctx, "Failed to delete user: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to delete user", err)
		return
	}

	// Invalidate any active sessions for the deleted user. Best-effort.
	if err := h.SessionRepo.DeleteByUserID(ctx, userID); err != nil {
		logger.Warn(ctx, "Failed to delete user sessions: %v", err)
	}

	logger.Info(ctx, "User deleted: %d", userID)

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "User deleted successfully",
	})
}

// updatePasswordHandler handles PUT /api/settings/password
func (h *Handlers) updatePasswordHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req UpdateUserPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Get current user
	currentUser, ok := auth.GetUserFromContext(ctx)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	// Verify current password
	if err := auth.VerifyPassword(currentUser.PasswordHash, req.CurrentPassword); err != nil {
		respondWithError(w, http.StatusUnauthorized, "Current password is incorrect", nil)
		return
	}

	// Hash new password
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		logger.Error(ctx, "Failed to hash password: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update password", err)
		return
	}

	// Update password via repository.
	if err := h.UserRepo.UpdatePassword(ctx, currentUser.ID, newHash); err != nil {
		logger.Error(ctx, "Failed to update password: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update password", err)
		return
	}

	logger.Info(ctx, "Password updated for user: %s", currentUser.Username)

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Password updated successfully",
	})
}

// listPartnersDetailedHandler handles GET /api/settings/partners
func (h *Handlers) listPartnersDetailedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.PartnerRepo.List(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to load partners: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to load partners", err)
		return
	}

	partners := make([]PartnerResponse, 0, len(rows))
	for _, p := range rows {
		resp := PartnerResponse{
			ID:        p.ID,
			PartnerID: p.PartnerID,
			Name:      p.Name,
			JWKSUrl:   p.JWKSUrl,
			CreatedAt: p.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if p.LastKeyRefresh != nil {
			resp.LastKeyRefresh = p.LastKeyRefresh.Format("2006-01-02 15:04:05")
		}
		partners = append(partners, resp)
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"partners": partners,
	})
}

// updatePartnerHandler handles PUT /api/settings/partners/:id
// The URL :id is the trading_partners.id (database numeric key) returned by
// listPartnersDetailedHandler, not the partner_id string.
func (h *Handlers) updatePartnerHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbID := parseInt64(chi.URLParam(r, "id"))

	var req UpdatePartnerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if err := h.PartnerRepo.UpdateNameByDBID(ctx, dbID, req.Name); err != nil {
		logger.Error(ctx, "Failed to update partner: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update partner", err)
		return
	}

	logger.Info(ctx, "Partner updated: id=%d", dbID)

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Partner updated successfully",
	})
}

// deletePartnerHandler handles DELETE /api/settings/partners/:id
// The URL :id is the trading_partners.id (database numeric key).
func (h *Handlers) deletePartnerHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbID := parseInt64(chi.URLParam(r, "id"))

	if err := h.PartnerRepo.DeleteByDBID(ctx, dbID); err != nil {
		logger.Error(ctx, "Failed to delete partner: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to delete partner", err)
		return
	}

	logger.Info(ctx, "Partner deleted: id=%d", dbID)

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Partner deleted successfully",
	})
}

// Helper function to parse int64
func parseInt64(s string) int64 {
	var result int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int64(c-'0')
		}
	}
	return result
}

// SetupSettingsRouter configures the settings API routes
func (h *Handlers) SetupSettingsRouter() chi.Router {
	r := chi.NewRouter()

	// All settings routes require authentication
	// r.Use(auth.RequireAuthAPI) // Uncomment when ready

	// Node configuration (read-only, use config file/env vars to change)
	r.Get("/config", h.getConfigHandler)
	r.Post("/rotate-keys", h.rotateKeysHandler)

	// User management
	r.Get("/users", h.listUsersHandler)
	r.Post("/users", h.createUserHandler)
	r.Delete("/users/{id}", h.deleteUserHandler)
	r.Put("/password", h.updatePasswordHandler)

	// Partner management
	r.Get("/partners", h.listPartnersDetailedHandler)
	r.Put("/partners/{id}", h.updatePartnerHandler)
	r.Delete("/partners/{id}", h.deletePartnerHandler)

	return r
}
