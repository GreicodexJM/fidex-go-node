package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"fidex-node/internal/auth"
	"fidex-node/internal/crypto"
	"fidex-node/internal/db"

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
func getConfigHandler(w http.ResponseWriter, r *http.Request) {
	if NodeConfig == nil {
		log.Printf("Node config not initialized")
		respondWithError(w, http.StatusInternalServerError, "Configuration not available", nil)
		return
	}

	// Check if private key file exists
	hasPrivateKey := false
	if _, err := os.Stat(NodeConfig.PrivateKeyPath); err == nil {
		hasPrivateKey = true
	}

	response := ConfigResponse{
		NodeID:             NodeConfig.NodeID,
		OrganizationName:   NodeConfig.OrganizationName,
		PublicDomain:       NodeConfig.PublicDomain,
		InternalAPIPort:    NodeConfig.InternalAPIPort,
		PublicAPIPort:      NodeConfig.PublicAPIPort,
		AllowedIPAddresses: NodeConfig.AllowedIPAddresses,
		EnableIPAllowlist:  NodeConfig.EnableIPAllowlist,
		HasPrivateKey:      hasPrivateKey,
		PublicKeyPath:      NodeConfig.PublicKeyPath,
	}

	respondWithJSON(w, http.StatusOK, response)
}

// rotateKeysHandler handles POST /api/settings/rotate-keys
// Generates new RSA key pair and saves to file system
func rotateKeysHandler(w http.ResponseWriter, r *http.Request) {
	if NodeConfig == nil {
		log.Printf("Node config not initialized")
		respondWithError(w, http.StatusInternalServerError, "Configuration not available", nil)
		return
	}

	log.Println("Generating new RSA key pair...")

	// Generate new key pair
	privateKeyPEM, publicKeyPEM, err := crypto.GenerateKeyPair()
	if err != nil {
		log.Printf("Failed to generate key pair: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to generate keys", err)
		return
	}

	// Save private key to file
	if err := os.WriteFile(NodeConfig.PrivateKeyPath, []byte(privateKeyPEM), 0600); err != nil {
		log.Printf("Failed to save private key: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save private key", err)
		return
	}

	// Save public key to file
	if err := os.WriteFile(NodeConfig.PublicKeyPath, []byte(publicKeyPEM), 0644); err != nil {
		log.Printf("Failed to save public key: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save public key", err)
		return
	}

	log.Println("RSA key pair rotated successfully")

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message":    "Keys rotated successfully",
		"public_key": publicKeyPEM,
	})
}

// listUsersHandler handles GET /api/settings/users
func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(`SELECT id, username, created_at FROM users ORDER BY created_at DESC`)
	if err != nil {
		log.Printf("Failed to query users: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to load users", err)
		return
	}
	defer rows.Close()

	users := []UserResponse{}
	for rows.Next() {
		var user UserResponse
		if err := rows.Scan(&user.ID, &user.Username, &user.CreatedAt); err != nil {
			log.Printf("Failed to scan user: %v", err)
			continue
		}
		users = append(users, user)
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"users": users,
	})
}

// createUserHandler handles POST /api/settings/users
func createUserHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Failed to hash password: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to create user", err)
		return
	}

	// Create user
	user, err := auth.CreateUser(req.Username, hashedPassword)
	if err != nil {
		log.Printf("Failed to create user: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to create user (username may already exist)", err)
		return
	}

	log.Printf("User created successfully: %s", req.Username)

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
func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	// Don't allow deleting yourself
	currentUser, ok := auth.GetUserFromContext(r.Context())
	if ok && currentUser.ID == parseInt64(userID) {
		respondWithError(w, http.StatusBadRequest, "Cannot delete your own account", nil)
		return
	}

	// Delete user
	_, err := db.DB.Exec(`DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		log.Printf("Failed to delete user: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to delete user", err)
		return
	}

	// Delete user's sessions
	_, err = db.DB.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		log.Printf("Failed to delete user sessions: %v", err)
	}

	log.Printf("User deleted: %s", userID)

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "User deleted successfully",
	})
}

// updatePasswordHandler handles PUT /api/settings/password
func updatePasswordHandler(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Get current user
	currentUser, ok := auth.GetUserFromContext(r.Context())
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
		log.Printf("Failed to hash password: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update password", err)
		return
	}

	// Update password
	_, err = db.DB.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, newHash, currentUser.ID)
	if err != nil {
		log.Printf("Failed to update password: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update password", err)
		return
	}

	log.Printf("Password updated for user: %s", currentUser.Username)

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Password updated successfully",
	})
}

// listPartnersDetailedHandler handles GET /api/settings/partners
func listPartnersDetailedHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(`
		SELECT id, partner_id, name, jwks_url, last_key_refresh, created_at
		FROM trading_partners
		ORDER BY created_at DESC
	`)
	if err != nil {
		log.Printf("Failed to query partners: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to load partners", err)
		return
	}
	defer rows.Close()

	partners := []PartnerResponse{}
	for rows.Next() {
		var partner PartnerResponse
		var lastRefresh, createdAt interface{}

		if err := rows.Scan(&partner.ID, &partner.PartnerID, &partner.Name, &partner.JWKSUrl, &lastRefresh, &createdAt); err != nil {
			log.Printf("Failed to scan partner: %v", err)
			continue
		}

		if lastRefresh != nil {
			partner.LastKeyRefresh = lastRefresh.(string)
		}
		if createdAt != nil {
			partner.CreatedAt = createdAt.(string)
		}

		partners = append(partners, partner)
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"partners": partners,
	})
}

// updatePartnerHandler handles PUT /api/settings/partners/:id
func updatePartnerHandler(w http.ResponseWriter, r *http.Request) {
	partnerID := chi.URLParam(r, "id")

	var req UpdatePartnerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Update partner name
	_, err := db.DB.Exec(`UPDATE trading_partners SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, req.Name, partnerID)
	if err != nil {
		log.Printf("Failed to update partner: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update partner", err)
		return
	}

	log.Printf("Partner updated: %s", partnerID)

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Partner updated successfully",
	})
}

// deletePartnerHandler handles DELETE /api/settings/partners/:id
func deletePartnerHandler(w http.ResponseWriter, r *http.Request) {
	partnerID := chi.URLParam(r, "id")

	// Delete partner
	_, err := db.DB.Exec(`DELETE FROM trading_partners WHERE id = ?`, partnerID)
	if err != nil {
		log.Printf("Failed to delete partner: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to delete partner", err)
		return
	}

	log.Printf("Partner deleted: %s", partnerID)

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
func SetupSettingsRouter() chi.Router {
	r := chi.NewRouter()

	// All settings routes require authentication
	// r.Use(auth.RequireAuthAPI) // Uncomment when ready

	// Node configuration (read-only, use config file/env vars to change)
	r.Get("/config", getConfigHandler)
	r.Post("/rotate-keys", rotateKeysHandler)

	// User management
	r.Get("/users", listUsersHandler)
	r.Post("/users", createUserHandler)
	r.Delete("/users/{id}", deleteUserHandler)
	r.Put("/password", updatePasswordHandler)

	// Partner management
	r.Get("/partners", listPartnersDetailedHandler)
	r.Put("/partners/{id}", updatePartnerHandler)
	r.Delete("/partners/{id}", deletePartnerHandler)

	return r
}
