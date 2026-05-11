package api

import (
	"context"
	"encoding/json"
	"net/http"

	"fidex-node/internal/errors"
)

// JSONResponse represents a standardized API response
type JSONResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo represents error information in API responses
type ErrorInfo struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Retryable bool                   `json:"retryable"`
}

// RespondJSON writes a JSON response with the given status code
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := JSONResponse{
		Success: statusCode >= 200 && statusCode < 300,
		Data:    data,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error(context.Background(), "Failed to encode JSON response: %v", err)
	}
}

// RespondSuccess writes a successful JSON response (200 OK)
func RespondSuccess(w http.ResponseWriter, data interface{}) {
	RespondJSON(w, http.StatusOK, data)
}

// RespondCreated writes a 201 Created response
func RespondCreated(w http.ResponseWriter, data interface{}) {
	RespondJSON(w, http.StatusCreated, data)
}

// RespondNoContent writes a 204 No Content response
func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// RespondError writes an error response using AppError
func RespondError(w http.ResponseWriter, err error) {
	// Check if it's an AppError
	if appErr, ok := errors.IsAppError(err); ok {
		RespondAppError(w, appErr)
		return
	}

	// For unknown errors, wrap as internal error
	appErr := errors.InternalWrap(err, "An unexpected error occurred")
	RespondAppError(w, appErr)
}

// RespondAppError writes an AppError as a JSON response
func RespondAppError(w http.ResponseWriter, appErr *errors.AppError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.StatusCode)

	response := JSONResponse{
		Success: false,
		Error: &ErrorInfo{
			Code:      string(appErr.Code),
			Message:   appErr.Message,
			Details:   appErr.Details,
			Retryable: appErr.Retryable,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error(context.Background(), "Failed to encode error response: %v", err)
	}

	// Log the underlying error (not exposed to client)
	if appErr.Err != nil {
		logger.Error(context.Background(), "[%s]: %s (underlying: %v)", appErr.Code, appErr.Message, appErr.Err)
	} else {
		logger.Error(context.Background(), "[%s]: %s", appErr.Code, appErr.Message)
	}
}

// RespondBadRequest is a convenience function for 400 errors
func RespondBadRequest(w http.ResponseWriter, message string) {
	RespondAppError(w, errors.BadRequest(message))
}

// RespondUnauthorized is a convenience function for 401 errors
func RespondUnauthorized(w http.ResponseWriter, message string) {
	RespondAppError(w, errors.Unauthorized(message))
}

// RespondForbidden is a convenience function for 403 errors
func RespondForbidden(w http.ResponseWriter, message string) {
	RespondAppError(w, errors.Forbidden(message))
}

// RespondNotFound is a convenience function for 404 errors
func RespondNotFound(w http.ResponseWriter, resource, id string) {
	RespondAppError(w, errors.NotFound(resource, id))
}

// RespondInternalError is a convenience function for 500 errors
func RespondInternalError(w http.ResponseWriter, err error, message string) {
	RespondAppError(w, errors.InternalWrap(err, message))
}
