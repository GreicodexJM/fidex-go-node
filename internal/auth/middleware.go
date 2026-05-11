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

// RequireAuth middleware checks for a valid session and redirects to /login on failure.
func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			log.Printf("No session cookie: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, err := s.GetSession(r.Context(), cookie.Value)
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

		user, err := s.GetUserByID(r.Context(), session.UserID)
		if err != nil {
			log.Printf("User not found: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuthAPI middleware checks for a valid session and returns JSON 401 on failure.
func (s *Service) RequireAuthAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			writeUnauthorized(w, "no valid session")
			return
		}

		session, err := s.GetSession(r.Context(), cookie.Value)
		if err != nil {
			writeUnauthorized(w, "invalid session")
			return
		}

		user, err := s.GetUserByID(r.Context(), session.UserID)
		if err != nil {
			writeUnauthorized(w, "user not found")
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"unauthorized","message":"` + message + `"}`))
}

// GetUserFromContext retrieves the user from request context
func GetUserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(UserContextKey).(*User)
	return user, ok
}
