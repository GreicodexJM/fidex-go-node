package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode represents a specific error condition in the application
type ErrorCode string

const (
	// Client Errors (4xx)
	ErrCodeBadRequest   ErrorCode = "BAD_REQUEST"
	ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden    ErrorCode = "FORBIDDEN"
	ErrCodeNotFound     ErrorCode = "NOT_FOUND"
	ErrCodeConflict     ErrorCode = "CONFLICT"
	ErrCodeValidation   ErrorCode = "VALIDATION_ERROR"
	ErrCodeRateLimited  ErrorCode = "RATE_LIMITED"

	// Server Errors (5xx)
	ErrCodeInternal    ErrorCode = "INTERNAL_ERROR"
	ErrCodeDatabase    ErrorCode = "DATABASE_ERROR"
	ErrCodeCrypto      ErrorCode = "CRYPTO_ERROR"
	ErrCodeNetwork     ErrorCode = "NETWORK_ERROR"
	ErrCodeTimeout     ErrorCode = "TIMEOUT"
	ErrCodeUnavailable ErrorCode = "SERVICE_UNAVAILABLE"

	// Business Logic Errors
	ErrCodeMessageInvalid   ErrorCode = "MESSAGE_INVALID"
	ErrCodePartnerNotFound  ErrorCode = "PARTNER_NOT_FOUND"
	ErrCodeKeyExpired       ErrorCode = "KEY_EXPIRED"
	ErrCodeSignatureInvalid ErrorCode = "SIGNATURE_INVALID"
	ErrCodeEncryptionFailed ErrorCode = "ENCRYPTION_FAILED"
)

// AppError represents a structured application error with context
type AppError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	Details    map[string]interface{} `json:"details,omitempty"`
	StatusCode int                    `json:"-"`
	Err        error                  `json:"-"` // Underlying error (not exposed to client)
	Retryable  bool                   `json:"retryable"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for error wrapping
func (e *AppError) Unwrap() error {
	return e.Err
}

// WithDetail adds additional context to the error
func (e *AppError) WithDetail(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// Constructor functions for common errors

// New creates a new AppError with the given code and message
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: codeToHTTPStatus(code),
		Retryable:  isRetryable(code),
	}
}

// Wrap wraps an existing error with application context
func Wrap(err error, code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		Err:        err,
		StatusCode: codeToHTTPStatus(code),
		Retryable:  isRetryable(code),
	}
}

// BadRequest creates a 400 Bad Request error
func BadRequest(message string) *AppError {
	return New(ErrCodeBadRequest, message)
}

// Unauthorized creates a 401 Unauthorized error
func Unauthorized(message string) *AppError {
	return New(ErrCodeUnauthorized, message)
}

// Forbidden creates a 403 Forbidden error
func Forbidden(message string) *AppError {
	return New(ErrCodeForbidden, message)
}

// NotFound creates a 404 Not Found error
func NotFound(resource, id string) *AppError {
	return New(ErrCodeNotFound, fmt.Sprintf("%s not found: %s", resource, id))
}

// Conflict creates a 409 Conflict error
func Conflict(message string) *AppError {
	return New(ErrCodeConflict, message)
}

// Validation creates a validation error
func Validation(message string) *AppError {
	return New(ErrCodeValidation, message)
}

// Internal creates a 500 Internal Server Error
func Internal(message string) *AppError {
	return New(ErrCodeInternal, message)
}

// InternalWrap wraps an error as an internal server error
func InternalWrap(err error, message string) *AppError {
	return Wrap(err, ErrCodeInternal, message)
}

// Database creates a database error
func Database(err error, operation string) *AppError {
	return Wrap(err, ErrCodeDatabase, fmt.Sprintf("database %s failed", operation))
}

// Crypto creates a cryptography error
func Crypto(err error, operation string) *AppError {
	return Wrap(err, ErrCodeCrypto, fmt.Sprintf("cryptography %s failed", operation))
}

// Network creates a network error
func Network(err error, operation string) *AppError {
	return Wrap(err, ErrCodeNetwork, fmt.Sprintf("network %s failed", operation))
}

// Timeout creates a timeout error
func Timeout(operation string) *AppError {
	return New(ErrCodeTimeout, fmt.Sprintf("%s timed out", operation))
}

// Helper functions

// codeToHTTPStatus maps error codes to HTTP status codes
func codeToHTTPStatus(code ErrorCode) int {
	switch code {
	case ErrCodeBadRequest, ErrCodeValidation, ErrCodeMessageInvalid:
		return http.StatusBadRequest
	case ErrCodeUnauthorized:
		return http.StatusUnauthorized
	case ErrCodeForbidden:
		return http.StatusForbidden
	case ErrCodeNotFound, ErrCodePartnerNotFound:
		return http.StatusNotFound
	case ErrCodeConflict:
		return http.StatusConflict
	case ErrCodeRateLimited:
		return http.StatusTooManyRequests
	case ErrCodeTimeout:
		return http.StatusRequestTimeout
	case ErrCodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// isRetryable determines if an error condition is retryable
func isRetryable(code ErrorCode) bool {
	switch code {
	case ErrCodeTimeout, ErrCodeUnavailable, ErrCodeNetwork, ErrCodeRateLimited:
		return true
	case ErrCodeBadRequest, ErrCodeUnauthorized, ErrCodeForbidden, ErrCodeNotFound, ErrCodeValidation:
		return false
	default:
		return false // Conservative: don't retry unless explicitly safe
	}
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) (*AppError, bool) {
	if appErr, ok := err.(*AppError); ok {
		return appErr, true
	}
	return nil, false
}

// GetStatusCode extracts the HTTP status code from an error
func GetStatusCode(err error) int {
	if appErr, ok := IsAppError(err); ok {
		return appErr.StatusCode
	}
	return http.StatusInternalServerError
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	if appErr, ok := IsAppError(err); ok {
		return appErr.Retryable
	}
	return false
}
