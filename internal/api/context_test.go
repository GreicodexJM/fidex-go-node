package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestIDMiddleware_GeneratesNewID(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		assert.NotEmpty(t, reqID)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

func TestRequestIDMiddleware_PreservesExistingID(t *testing.T) {
	existingID := "test-request-id-123"

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		assert.Equal(t, existingID, reqID)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", existingID)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, existingID, w.Header().Get("X-Request-ID"))
}

func TestRequestIDMiddleware_SetsResponseHeader(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	assert.NotEmpty(t, requestID)
	assert.Len(t, requestID, 36) // UUID length
}

func TestGetRequestID_WithValidContext(t *testing.T) {
	expectedID := "test-request-123"
	ctx := context.WithValue(context.Background(), RequestIDKey, expectedID)

	actualID := GetRequestID(ctx)

	assert.Equal(t, expectedID, actualID)
}

func TestGetRequestID_WithEmptyContext(t *testing.T) {
	ctx := context.Background()

	reqID := GetRequestID(ctx)

	assert.Equal(t, "unknown", reqID) // GetRequestID returns "unknown" for empty context
}

func TestGetRequestID_WithWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), RequestIDKey, 12345)

	reqID := GetRequestID(ctx)

	assert.Equal(t, "unknown", reqID) // GetRequestID returns "unknown" for wrong type
}

func TestGetUserID_WithValidContext(t *testing.T) {
	expectedUserID := int64(42)
	ctx := WithUserID(context.Background(), expectedUserID)

	actualUserID := GetUserID(ctx)

	assert.Equal(t, expectedUserID, actualUserID)
}

func TestGetUserID_WithEmptyContext(t *testing.T) {
	ctx := context.Background()

	userID := GetUserID(ctx)

	assert.Equal(t, int64(0), userID)
}

func TestGetUserID_WithWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserIDKey, "not-an-int")

	userID := GetUserID(ctx)

	assert.Equal(t, int64(0), userID)
}

func TestWithUserID(t *testing.T) {
	userID := int64(123)
	ctx := WithUserID(context.Background(), userID)

	retrievedUserID := GetUserID(ctx)

	assert.Equal(t, userID, retrievedUserID)
}

func TestContextKeyIsolation(t *testing.T) {
	// Ensure request ID and user ID don't interfere with each other
	requestID := "req-123"
	userID := int64(456)

	ctx := context.WithValue(context.Background(), RequestIDKey, requestID)
	ctx = WithUserID(ctx, userID)

	assert.Equal(t, requestID, GetRequestID(ctx))
	assert.Equal(t, userID, GetUserID(ctx))
}

func TestRequestIDMiddleware_PropagatesContext(t *testing.T) {
	var capturedUserID int64

	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Request ID should be in context
		reqID := GetRequestID(r.Context())
		assert.NotEmpty(t, reqID)

		// Add user ID to context
		ctx := WithUserID(r.Context(), 789)
		capturedUserID = GetUserID(ctx)

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, int64(789), capturedUserID)
}

func TestMultipleRequestIDs(t *testing.T) {
	// Ensure each request gets a unique ID
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/test1", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/test2", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	id1 := w1.Header().Get("X-Request-ID")
	id2 := w2.Header().Get("X-Request-ID")

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "Each request should have a unique ID")
}
