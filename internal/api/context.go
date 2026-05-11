package api

import (
	"context"
	"net/http"

	"fidex-node/internal/logging"

	"github.com/google/uuid"
)

// RequestIDKey and UserIDKey are re-exported from logging so existing api
// callers continue to compile while sharing the single source of truth for
// context-key identity with the logger.
const (
	RequestIDKey = logging.RequestIDKey
	UserIDKey    = logging.UserIDKey
)

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request ID already exists in header
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			// Generate new UUID for request
			requestID = uuid.New().String()
		}

		// Add to response header for client tracking
		w.Header().Set("X-Request-ID", requestID)

		// Add to context using the shared logging key
		ctx := logging.WithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request ID from context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(logging.RequestIDKey).(string); ok {
		return requestID
	}
	return "unknown"
}

// GetUserID extracts the user ID from context
func GetUserID(ctx context.Context) int64 {
	if userID, ok := ctx.Value(logging.UserIDKey).(int64); ok {
		return userID
	}
	return 0
}

// WithUserID adds user ID to context (used by auth middleware)
func WithUserID(ctx context.Context, userID int64) context.Context {
	return logging.WithUserID(ctx, userID)
}
