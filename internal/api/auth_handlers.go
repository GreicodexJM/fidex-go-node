package api

import (
	"encoding/json"
	"log"
	"net/http"

	"fidex-node/internal/auth"
	"fidex-node/internal/db"

	"github.com/go-chi/chi/v5"
)

// LoginRequest represents the login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Success  bool   `json:"success"`
	Username string `json:"username,omitempty"`
	Message  string `json:"message,omitempty"`
}

// loginHandler handles POST /api/auth/login
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithJSON(w, http.StatusBadRequest, LoginResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	// Get user from database
	user, err := auth.GetUserByUsername(req.Username)
	if err != nil {
		log.Printf("Login failed: user not found: %s", req.Username)
		respondWithJSON(w, http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Verify password
	if err := auth.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		log.Printf("Login failed: invalid password for user: %s", req.Username)
		respondWithJSON(w, http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Create session
	session, err := auth.CreateSession(user.ID)
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		respondWithJSON(w, http.StatusInternalServerError, LoginResponse{
			Success: false,
			Message: "Failed to create session",
		})
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    session.SessionID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	log.Printf("User logged in successfully: %s", user.Username)

	respondWithJSON(w, http.StatusOK, LoginResponse{
		Success:  true,
		Username: user.Username,
	})
}

// logoutHandler handles POST /api/auth/logout
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err == nil {
		// Delete session from database
		auth.DeleteSession(cookie.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Logged out successfully",
	})
}

// meHandler handles GET /api/auth/me
func meHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondWithJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"error": "unauthorized",
		})
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"username":   user.Username,
		"created_at": user.CreatedAt,
	})
}

// respondWithJSON is a helper to send JSON responses
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(payload)
}

// InitializeDefaultUser creates the default admin user if no users exist
func InitializeDefaultUser() error {
	// Check if any users exist
	var count int
	err := db.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	// Create default admin user if none exist
	if count == 0 {
		hashedPassword, err := auth.HashPassword("admin123")
		if err != nil {
			return err
		}

		_, err = auth.CreateUser("admin", hashedPassword)
		if err != nil {
			return err
		}

		log.Println("========================================")
		log.Println("IMPORTANT: Default admin user created")
		log.Println("Username: admin")
		log.Println("Password: admin123")
		log.Println("Please change this password immediately!")
		log.Println("========================================")
	}

	return nil
}

// SetupAuthRouter configures the auth API routes
func SetupAuthRouter() chi.Router {
	r := chi.NewRouter()

	r.Post("/login", loginHandler)
	r.Post("/logout", logoutHandler)
	r.Get("/me", meHandler)

	return r
}
