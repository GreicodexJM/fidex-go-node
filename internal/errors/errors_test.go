package errors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppError_Error(t *testing.T) {
	err := &AppError{
		Code:    ErrCodeBadRequest,
		Message: "Invalid input",
	}

	assert.Equal(t, "BAD_REQUEST: Invalid input", err.Error())
}

func TestAppError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	appErr := &AppError{
		Code:    ErrCodeInternal,
		Message: "wrapped error",
		Err:     originalErr,
	}

	unwrapped := appErr.Unwrap()
	assert.Equal(t, originalErr, unwrapped)
}

func TestAppError_WithDetail(t *testing.T) {
	err := New(ErrCodeValidation, "Validation failed")
	err.WithDetail("field", "username")
	err.WithDetail("reason", "too short")

	assert.Equal(t, "username", err.Details["field"])
	assert.Equal(t, "too short", err.Details["reason"])
	assert.Len(t, err.Details, 2)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		code       ErrorCode
		message    string
		wantStatus int
		wantRetry  bool
	}{
		{
			name:       "bad request error",
			code:       ErrCodeBadRequest,
			message:    "Invalid input",
			wantStatus: http.StatusBadRequest,
			wantRetry:  false,
		},
		{
			name:       "internal error",
			code:       ErrCodeInternal,
			message:    "Server error",
			wantStatus: http.StatusInternalServerError,
			wantRetry:  false, // Conservative: not retryable by default
		},
		{
			name:       "not found error",
			code:       ErrCodeNotFound,
			message:    "Resource not found",
			wantStatus: http.StatusNotFound,
			wantRetry:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, tt.message)

			assert.Equal(t, tt.code, err.Code)
			assert.Equal(t, tt.message, err.Message)
			assert.Equal(t, tt.wantStatus, err.StatusCode)
			assert.Equal(t, tt.wantRetry, err.Retryable)
			assert.Nil(t, err.Err)
		})
	}
}

func TestWrap(t *testing.T) {
	originalErr := errors.New("database connection failed")
	appErr := Wrap(originalErr, ErrCodeDatabase, "Failed to connect to database")

	assert.Equal(t, ErrCodeDatabase, appErr.Code)
	assert.Equal(t, "Failed to connect to database", appErr.Message)
	assert.Equal(t, originalErr, appErr.Err)
	assert.Equal(t, http.StatusInternalServerError, appErr.StatusCode)
	assert.False(t, appErr.Retryable) // Database errors are NOT retryable by default
}

func TestBadRequest(t *testing.T) {
	err := BadRequest("Missing required field")

	assert.Equal(t, ErrCodeBadRequest, err.Code)
	assert.Equal(t, "Missing required field", err.Message)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.False(t, err.Retryable)
}

func TestUnauthorized(t *testing.T) {
	err := Unauthorized("Invalid credentials")

	assert.Equal(t, ErrCodeUnauthorized, err.Code)
	assert.Equal(t, "Invalid credentials", err.Message)
	assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	assert.False(t, err.Retryable)
}

func TestForbidden(t *testing.T) {
	err := Forbidden("Access denied")

	assert.Equal(t, ErrCodeForbidden, err.Code)
	assert.Equal(t, "Access denied", err.Message)
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.False(t, err.Retryable)
}

func TestNotFound(t *testing.T) {
	err := NotFound("user", "123")

	assert.Equal(t, ErrCodeNotFound, err.Code)
	assert.Equal(t, "user not found: 123", err.Message)
	assert.Equal(t, http.StatusNotFound, err.StatusCode)
	assert.False(t, err.Retryable)
}

func TestConflict(t *testing.T) {
	err := Conflict("Resource already exists")

	assert.Equal(t, ErrCodeConflict, err.Code)
	assert.Equal(t, "Resource already exists", err.Message)
	assert.Equal(t, http.StatusConflict, err.StatusCode)
	assert.False(t, err.Retryable)
}

func TestValidation(t *testing.T) {
	err := Validation("Invalid email format")

	assert.Equal(t, ErrCodeValidation, err.Code)
	assert.Equal(t, "Invalid email format", err.Message)
	assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	assert.False(t, err.Retryable)
}

func TestInternal(t *testing.T) {
	err := Internal("Something went wrong")

	assert.Equal(t, ErrCodeInternal, err.Code)
	assert.Equal(t, "Something went wrong", err.Message)
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.False(t, err.Retryable) // Internal errors are NOT retryable by default
}

func TestInternalWrap(t *testing.T) {
	originalErr := errors.New("panic recovered")
	err := InternalWrap(originalErr, "Unexpected error occurred")

	assert.Equal(t, ErrCodeInternal, err.Code)
	assert.Equal(t, "Unexpected error occurred", err.Message)
	assert.Equal(t, originalErr, err.Err)
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.False(t, err.Retryable) // Internal errors are NOT retryable by default
}

func TestDatabase(t *testing.T) {
	dbErr := errors.New("connection timeout")
	err := Database(dbErr, "insert user")

	assert.Equal(t, ErrCodeDatabase, err.Code)
	assert.Equal(t, "database insert user failed", err.Message)
	assert.Equal(t, dbErr, err.Err)
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.False(t, err.Retryable) // Database errors are NOT retryable by default
}

func TestCrypto(t *testing.T) {
	cryptoErr := errors.New("key expired")
	err := Crypto(cryptoErr, "decrypt message")

	assert.Equal(t, ErrCodeCrypto, err.Code)
	assert.Equal(t, "cryptography decrypt message failed", err.Message)
	assert.Equal(t, cryptoErr, err.Err)
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.False(t, err.Retryable) // Crypto errors are NOT retryable by default
}

func TestNetwork(t *testing.T) {
	netErr := errors.New("connection refused")
	err := Network(netErr, "message delivery")

	assert.Equal(t, ErrCodeNetwork, err.Code)
	assert.Equal(t, "network message delivery failed", err.Message)
	assert.Equal(t, netErr, err.Err)
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.True(t, err.Retryable)
}

func TestTimeout(t *testing.T) {
	err := Timeout("API call")

	assert.Equal(t, ErrCodeTimeout, err.Code)
	assert.Equal(t, "API call timed out", err.Message)
	assert.Equal(t, http.StatusRequestTimeout, err.StatusCode)
	assert.True(t, err.Retryable)
}

func TestCodeToHTTPStatus(t *testing.T) {
	tests := []struct {
		code       ErrorCode
		wantStatus int
	}{
		{ErrCodeBadRequest, http.StatusBadRequest},
		{ErrCodeUnauthorized, http.StatusUnauthorized},
		{ErrCodeForbidden, http.StatusForbidden},
		{ErrCodeNotFound, http.StatusNotFound},
		{ErrCodeConflict, http.StatusConflict},
		{ErrCodeValidation, http.StatusBadRequest},
		{ErrCodeInternal, http.StatusInternalServerError},
		{ErrCodeDatabase, http.StatusInternalServerError},
		{ErrCodeCrypto, http.StatusInternalServerError},
		{ErrCodeNetwork, http.StatusInternalServerError},
		{ErrCodeTimeout, http.StatusRequestTimeout},
		{ErrorCode("UNKNOWN"), http.StatusInternalServerError}, // Unknown code
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			status := codeToHTTPStatus(tt.code)
			assert.Equal(t, tt.wantStatus, status)
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		code        ErrorCode
		wantRetry   bool
		description string
	}{
		{ErrCodeBadRequest, false, "client errors not retryable"},
		{ErrCodeUnauthorized, false, "auth errors not retryable"},
		{ErrCodeForbidden, false, "forbidden not retryable"},
		{ErrCodeNotFound, false, "not found not retryable"},
		{ErrCodeConflict, false, "conflict not retryable"},
		{ErrCodeValidation, false, "validation not retryable"},
		{ErrCodeInternal, false, "internal errors not retryable by default"},
		{ErrCodeDatabase, false, "database errors not retryable by default"},
		{ErrCodeCrypto, false, "crypto errors not retryable by default"},
		{ErrCodeNetwork, true, "network errors retryable"},
		{ErrCodeTimeout, true, "timeout errors retryable"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			retry := isRetryable(tt.code)
			assert.Equal(t, tt.wantRetry, retry)
		})
	}
}

func TestIsAppError(t *testing.T) {
	t.Run("recognizes AppError", func(t *testing.T) {
		appErr := BadRequest("test error")
		result, ok := IsAppError(appErr)

		assert.True(t, ok)
		assert.Equal(t, appErr, result)
	})

	t.Run("recognizes wrapped AppError", func(t *testing.T) {
		appErr := Internal("test error")
		wrappedErr := errors.New("wrapper: " + appErr.Error())

		// AppError should not be recognized when wrapped in standard error
		_, ok := IsAppError(wrappedErr)
		assert.False(t, ok)
	})

	t.Run("returns false for standard error", func(t *testing.T) {
		standardErr := errors.New("standard error")
		_, ok := IsAppError(standardErr)

		assert.False(t, ok)
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		_, ok := IsAppError(nil)
		assert.False(t, ok)
	})
}

func TestGetStatusCode(t *testing.T) {
	t.Run("returns status code for AppError", func(t *testing.T) {
		err := NotFound("user", "123")
		status := GetStatusCode(err)

		assert.Equal(t, http.StatusNotFound, status)
	})

	t.Run("returns 500 for standard error", func(t *testing.T) {
		err := errors.New("standard error")
		status := GetStatusCode(err)

		assert.Equal(t, http.StatusInternalServerError, status)
	})

	t.Run("returns 500 for nil error", func(t *testing.T) {
		status := GetStatusCode(nil)
		assert.Equal(t, http.StatusInternalServerError, status)
	})
}

func TestIsRetryableFunction(t *testing.T) {
	t.Run("returns true for retryable AppError", func(t *testing.T) {
		err := Network(errors.New("network error"), "API call")
		assert.True(t, IsRetryable(err))
	})

	t.Run("returns false for non-retryable AppError", func(t *testing.T) {
		err := BadRequest("invalid input")
		assert.False(t, IsRetryable(err))
	})

	t.Run("returns false for standard error", func(t *testing.T) {
		err := errors.New("standard error")
		assert.False(t, IsRetryable(err))
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		assert.False(t, IsRetryable(nil))
	})
}

func TestErrorDetails(t *testing.T) {
	err := Validation("Invalid input")
	err.WithDetail("field", "email")
	err.WithDetail("value", "invalid@")
	err.WithDetail("constraint", "must be valid email")

	require.Len(t, err.Details, 3)
	assert.Equal(t, "email", err.Details["field"])
	assert.Equal(t, "invalid@", err.Details["value"])
	assert.Equal(t, "must be valid email", err.Details["constraint"])
}

func TestErrorChaining(t *testing.T) {
	// Create chain of errors
	originalErr := errors.New("root cause")
	dbErr := Database(originalErr, "select query")
	wrappedErr := InternalWrap(dbErr, "failed to get user")

	// Verify chain
	assert.Equal(t, dbErr, wrappedErr.Err)
	assert.Equal(t, originalErr, dbErr.Err)

	// Verify unwrapping
	assert.Equal(t, dbErr, wrappedErr.Unwrap())
	assert.Equal(t, originalErr, dbErr.Unwrap())
}
