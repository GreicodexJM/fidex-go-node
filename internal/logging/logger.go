package logging

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ContextKey is the type used for context keys defined by this package.
// Using a custom type avoids collisions with keys defined in other packages.
type ContextKey string

const (
	// RequestIDKey carries a per-request correlation ID across goroutines.
	RequestIDKey ContextKey = "request_id"

	// UserIDKey carries the authenticated user's numeric ID.
	UserIDKey ContextKey = "user_id"
)

// WithRequestID returns a copy of ctx carrying the given request ID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// WithUserID returns a copy of ctx carrying the given user ID.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// Logger wraps the standard logger with context-aware structured output.
type Logger struct {
	prefix string
}

// New creates a new logger instance with the given prefix.
func New(prefix string) *Logger {
	return &Logger{prefix: prefix}
}

// Info logs an informational message with context.
func (l *Logger) Info(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, "INFO", format, args...)
}

// Error logs an error message with context.
func (l *Logger) Error(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, "ERROR", format, args...)
}

// Warn logs a warning message with context.
func (l *Logger) Warn(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, "WARN", format, args...)
}

// Debug logs a debug message with context.
func (l *Logger) Debug(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, "DEBUG", format, args...)
}

// log is the internal logging function.
func (l *Logger) log(ctx context.Context, level string, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)

	requestID := extractRequestID(ctx)
	userID := extractUserID(ctx)

	var contextInfo string
	if requestID != "" || userID != 0 {
		contextInfo = " ["
		if requestID != "" {
			contextInfo += fmt.Sprintf("req_id=%s", requestID)
			if userID != 0 {
				contextInfo += " "
			}
		}
		if userID != 0 {
			contextInfo += fmt.Sprintf("user_id=%d", userID)
		}
		contextInfo += "]"
	}

	// Format: timestamp [LEVEL] prefix: message [context]
	logLine := fmt.Sprintf("%s [%s] %s: %s%s", timestamp, level, l.prefix, message, contextInfo)
	log.Println(logLine)
}

// extractRequestID reads the request ID from context (empty string if absent).
func extractRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// extractUserID reads the user ID from context (0 if absent).
func extractUserID(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	if userID, ok := ctx.Value(UserIDKey).(int64); ok {
		return userID
	}
	return 0
}

// Default logger instance (used by the package-level helpers below).
var defaultLogger = New("app")

// Info logs an informational message with the default logger.
func Info(ctx context.Context, format string, args ...interface{}) {
	defaultLogger.Info(ctx, format, args...)
}

// Error logs an error message with the default logger.
func Error(ctx context.Context, format string, args ...interface{}) {
	defaultLogger.Error(ctx, format, args...)
}

// Warn logs a warning message with the default logger.
func Warn(ctx context.Context, format string, args ...interface{}) {
	defaultLogger.Warn(ctx, format, args...)
}

// Debug logs a debug message with the default logger.
func Debug(ctx context.Context, format string, args ...interface{}) {
	defaultLogger.Debug(ctx, format, args...)
}
