package auth

import (
	"context"
	"log"
	"net/http"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// UserContextKey is the key for storing user info in request context
	UserContextKey contextKey = "user"
	// SessionCookieName is the name of the session cookie
	SessionCookieName = "fidex_session"
)

// RequireAuth middleware checks for a valid session
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get session cookie
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			log.Printf("No session cookie: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Validate session
		session, err := GetSession(cookie.Value)
		if err != nil {
			log.Printf("Invalid session: %v", err)
			http.SetCookie(w, &http.Cookie{
				Name:   SessionCookieName,
				Value:  "",
				Path:   "/",
				MaxAge: -1,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Get user
		user, err := GetUserByID(session.UserID)
		if err != nil {
			log.Printf("User not found: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserFromContext retrieves the user from request context
func GetUserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(UserContextKey).(*User)
	return user, ok
}

// RequireAuthAPI middleware for API endpoints (returns JSON error instead of redirect)
func RequireAuthAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get session cookie
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","message":"no valid session"}`))
			return
		}

		// Validate session
		session, err := GetSession(cookie.Value)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","message":"invalid session"}`))
			return
		}

		// Get user
		user, err := GetUserByID(session.UserID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","message":"user not found"}`))
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
