package logging

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Logger wraps standard logger with context support
type Logger struct {
	prefix string
}

// New creates a new logger instance
func New(prefix string) *Logger {
	return &Logger{prefix: prefix}
}

// Info logs an informational message with context
func (l *Logger) Info(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, "INFO", format, args...)
}

// Error logs an error message with context
func (l *Logger) Error(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, "ERROR", format, args...)
}

// Warn logs a warning message with context
func (l *Logger) Warn(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, "WARN", format, args...)
}

// Debug logs a debug message with context
func (l *Logger) Debug(ctx context.Context, format string, args ...interface{}) {
	l.log(ctx, "DEBUG", format, args...)
}

// log is the internal logging function
func (l *Logger) log(ctx context.Context, level string, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)

	// Extract request ID from context if available
	requestID := extractRequestID(ctx)
	userID := extractUserID(ctx)

	var contextInfo string
	if requestID != "" {
		contextInfo = fmt.Sprintf(" [req_id=%s", requestID)
		if userID != 0 {
			contextInfo += fmt.Sprintf(" user_id=%d", userID)
		}
		contextInfo += "]"
	}

	// Format: timestamp [LEVEL] prefix: message [context]
	logLine := fmt.Sprintf("%s [%s] %s: %s%s", timestamp, level, l.prefix, message, contextInfo)
	log.Println(logLine)
}

// extractRequestID tries to extract request ID from context
func extractRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(ContextKey("request_id")).(string); ok {
		return requestID
	}
	return ""
}

// extractUserID tries to extract user ID from context
func extractUserID(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	if userID, ok := ctx.Value(ContextKey("user_id")).(int64); ok {
		return userID
	}
	return 0
}

// ContextKey is a type for context keys
type ContextKey string

// Default logger instance
var defaultLogger = New("app")

// Info logs an informational message with the default logger
func Info(ctx context.Context, format string, args ...interface{}) {
	defaultLogger.Info(ctx, format, args...)
}

// Error logs an error message with the default logger
func Error(ctx context.Context, format string, args ...interface{}) {
	defaultLogger.Error(ctx, format, args...)
}

// Warn logs a warning message with the default logger
func Warn(ctx context.Context, format string, args ...interface{}) {
	defaultLogger.Warn(ctx, format, args...)
}

// Debug logs a debug message with the default logger
func Debug(ctx context.Context, format string, args ...interface{}) {
	defaultLogger.Debug(ctx, format, args...)
}
