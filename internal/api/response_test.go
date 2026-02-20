package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apierrors "fidex-node/internal/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondJSON(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		data           interface{}
		wantSuccess    bool
		wantStatusCode int
	}{
		{
			name:           "success with 200 OK",
			statusCode:     http.StatusOK,
			data:           map[string]string{"message": "success"},
			wantSuccess:    true,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "success with 201 Created",
			statusCode:     http.StatusCreated,
			data:           map[string]string{"id": "123"},
			wantSuccess:    true,
			wantStatusCode: http.StatusCreated,
		},
		{
			name:           "error with 400 Bad Request",
			statusCode:     http.StatusBadRequest,
			data:           nil,
			wantSuccess:    false,
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:           "error with 500 Internal Server Error",
			statusCode:     http.StatusInternalServerError,
			data:           nil,
			wantSuccess:    false,
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			RespondJSON(w, tt.statusCode, tt.data)

			assert.Equal(t, tt.wantStatusCode, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var response JSONResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.wantSuccess, response.Success)
			if tt.data != nil {
				assert.NotNil(t, response.Data)
			}
		})
	}
}

func TestRespondSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"message": "test"}

	RespondSuccess(w, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response JSONResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.NotNil(t, response.Data)
}

func TestRespondCreated(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"id": "new-resource-123"}

	RespondCreated(w, data)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response JSONResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.NotNil(t, response.Data)
}

func TestRespondNoContent(t *testing.T) {
	w := httptest.NewRecorder()

	RespondNoContent(w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())
}

func TestRespondError(t *testing.T) {
	t.Run("handles AppError", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := apierrors.BadRequest("Invalid input")

		RespondError(w, err)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response JSONResponse
		jsonErr := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, jsonErr)

		assert.False(t, response.Success)
		assert.Nil(t, response.Data)
		assert.NotNil(t, response.Error)
		assert.Equal(t, string(apierrors.ErrCodeBadRequest), response.Error.Code)
		assert.Equal(t, "Invalid input", response.Error.Message)
	})

	t.Run("wraps standard error as internal error", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := errors.New("unexpected error")

		RespondError(w, err)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var response JSONResponse
		jsonErr := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, jsonErr)

		assert.False(t, response.Success)
		assert.NotNil(t, response.Error)
		assert.Equal(t, string(apierrors.ErrCodeInternal), response.Error.Code)
	})
}

func TestRespondAppError(t *testing.T) {
	tests := []struct {
		name           string
		appErr         *apierrors.AppError
		wantStatusCode int
		wantCode       string
		wantRetryable  bool
	}{
		{
			name:           "bad request error",
			appErr:         apierrors.BadRequest("Missing field"),
			wantStatusCode: http.StatusBadRequest,
			wantCode:       string(apierrors.ErrCodeBadRequest),
			wantRetryable:  false,
		},
		{
			name:           "not found error",
			appErr:         apierrors.NotFound("user", "123"),
			wantStatusCode: http.StatusNotFound,
			wantCode:       string(apierrors.ErrCodeNotFound),
			wantRetryable:  false,
		},
		{
			name:           "database error",
			appErr:         apierrors.Database(errors.New("connection failed"), "query"),
			wantStatusCode: http.StatusInternalServerError,
			wantCode:       string(apierrors.ErrCodeDatabase),
			wantRetryable:  false, // Database errors are NOT retryable by default
		},
		{
			name:           "network error",
			appErr:         apierrors.Network(errors.New("timeout"), "API call"),
			wantStatusCode: http.StatusInternalServerError, // Network errors map to 500 in current implementation
			wantCode:       string(apierrors.ErrCodeNetwork),
			wantRetryable:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			RespondAppError(w, tt.appErr)

			assert.Equal(t, tt.wantStatusCode, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var response JSONResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.False(t, response.Success)
			assert.Nil(t, response.Data)
			require.NotNil(t, response.Error)
			assert.Equal(t, tt.wantCode, response.Error.Code)
			assert.NotEmpty(t, response.Error.Message)
			assert.Equal(t, tt.wantRetryable, response.Error.Retryable)
		})
	}
}

func TestRespondAppError_WithDetails(t *testing.T) {
	w := httptest.NewRecorder()
	err := apierrors.Validation("Invalid input")
	err.WithDetail("field", "email")
	err.WithDetail("constraint", "must be valid email")

	RespondAppError(w, err)

	var response JSONResponse
	jsonErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, jsonErr)

	require.NotNil(t, response.Error)
	require.NotNil(t, response.Error.Details)
	assert.Equal(t, "email", response.Error.Details["field"])
	assert.Equal(t, "must be valid email", response.Error.Details["constraint"])
}

func TestRespondBadRequest(t *testing.T) {
	w := httptest.NewRecorder()

	RespondBadRequest(w, "Invalid JSON")

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response JSONResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	require.NotNil(t, response.Error)
	assert.Equal(t, "Invalid JSON", response.Error.Message)
	assert.Equal(t, string(apierrors.ErrCodeBadRequest), response.Error.Code)
}

func TestRespondUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()

	RespondUnauthorized(w, "Invalid token")

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var response JSONResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	require.NotNil(t, response.Error)
	assert.Equal(t, "Invalid token", response.Error.Message)
	assert.Equal(t, string(apierrors.ErrCodeUnauthorized), response.Error.Code)
}

func TestRespondForbidden(t *testing.T) {
	w := httptest.NewRecorder()

	RespondForbidden(w, "Access denied")

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response JSONResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	require.NotNil(t, response.Error)
	assert.Equal(t, "Access denied", response.Error.Message)
	assert.Equal(t, string(apierrors.ErrCodeForbidden), response.Error.Code)
}

func TestRespondNotFound(t *testing.T) {
	w := httptest.NewRecorder()

	RespondNotFound(w, "partner", "abc-123")

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response JSONResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	require.NotNil(t, response.Error)
	assert.Contains(t, response.Error.Message, "partner")
	assert.Contains(t, response.Error.Message, "abc-123")
	assert.Equal(t, string(apierrors.ErrCodeNotFound), response.Error.Code)
}

func TestRespondInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	originalErr := errors.New("database connection failed")

	RespondInternalError(w, originalErr, "Failed to process request")

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response JSONResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.False(t, response.Success)
	require.NotNil(t, response.Error)
	assert.Equal(t, "Failed to process request", response.Error.Message)
	assert.Equal(t, string(apierrors.ErrCodeInternal), response.Error.Code)
	assert.False(t, response.Error.Retryable) // Internal errors are NOT retryable by default
}

func TestJSONResponse_Structure(t *testing.T) {
	t.Run("success response structure", func(t *testing.T) {
		w := httptest.NewRecorder()
		data := map[string]interface{}{
			"id":   "123",
			"name": "Test",
		}

		RespondSuccess(w, data)

		var response JSONResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response.Success)
		assert.NotNil(t, response.Data)
		assert.Nil(t, response.Error)
	})

	t.Run("error response structure", func(t *testing.T) {
		w := httptest.NewRecorder()

		RespondBadRequest(w, "test error")

		var response JSONResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.False(t, response.Success)
		assert.Nil(t, response.Data)
		assert.NotNil(t, response.Error)
		assert.NotEmpty(t, response.Error.Code)
		assert.NotEmpty(t, response.Error.Message)
	})
}

func TestResponseContentType(t *testing.T) {
	tests := []struct {
		name    string
		handler func(w http.ResponseWriter)
	}{
		{
			name:    "RespondSuccess sets JSON content type",
			handler: func(w http.ResponseWriter) { RespondSuccess(w, nil) },
		},
		{
			name:    "RespondCreated sets JSON content type",
			handler: func(w http.ResponseWriter) { RespondCreated(w, nil) },
		},
		{
			name:    "RespondBadRequest sets JSON content type",
			handler: func(w http.ResponseWriter) { RespondBadRequest(w, "error") },
		},
		{
			name:    "RespondError sets JSON content type",
			handler: func(w http.ResponseWriter) { RespondError(w, errors.New("test")) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.handler(w)

			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		})
	}
}

func TestErrorInfo_RetryableField(t *testing.T) {
	tests := []struct {
		name          string
		appErr        *apierrors.AppError
		wantRetryable bool
	}{
		{
			name:          "client errors are not retryable",
			appErr:        apierrors.BadRequest("test"),
			wantRetryable: false,
		},
		{
			name:          "server errors are not retryable by default",
			appErr:        apierrors.Internal("test"),
			wantRetryable: false,
		},
		{
			name:          "database errors are not retryable by default",
			appErr:        apierrors.Database(errors.New("test"), "op"),
			wantRetryable: false,
		},
		{
			name:          "network errors are retryable",
			appErr:        apierrors.Network(errors.New("test"), "op"),
			wantRetryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			RespondAppError(w, tt.appErr)

			var response JSONResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			require.NotNil(t, response.Error)
			assert.Equal(t, tt.wantRetryable, response.Error.Retryable)
		})
	}
}
