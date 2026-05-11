package auth

import (
	"context"
	"net/http"

	"fidex-node/internal/logging"
)

// logger is the package-level structured logger for auth.
var logger = logging.New("auth")

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
		ctx := r.Context()
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			logger.Debug(ctx, "No session cookie: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, err := s.GetSession(ctx, cookie.Value)
		if err != nil {
			logger.Warn(ctx, "Invalid session: %v", err)
			http.SetCookie(w, &http.Cookie{
				Name:   SessionCookieName,
				Value:  "",
				Path:   "/",
				MaxAge: -1,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		user, err := s.GetUserByID(ctx, session.UserID)
		if err != nil {
			logger.Warn(ctx, "User not found: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Propagate the authenticated user id into context so downstream
		// loggers can attach it automatically.
		ctx = logging.WithUserID(ctx, user.ID)
		ctx = context.WithValue(ctx, UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuthAPI middleware checks for a valid session and returns JSON 401 on failure.
func (s *Service) RequireAuthAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			writeUnauthorized(w, "no valid session")
			return
		}

		session, err := s.GetSession(ctx, cookie.Value)
		if err != nil {
			writeUnauthorized(w, "invalid session")
			return
		}

		user, err := s.GetUserByID(ctx, session.UserID)
		if err != nil {
			writeUnauthorized(w, "user not found")
			return
		}

		ctx = logging.WithUserID(ctx, user.ID)
		ctx = context.WithValue(ctx, UserContextKey, user)
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
