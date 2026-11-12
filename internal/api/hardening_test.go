package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The IP allowlist must judge the TCP peer, not client-supplied headers.
func TestIPAllowlistIgnoresForwardedHeaders(t *testing.T) {
	h := IPAllowlistMiddleware([]string{"127.0.0.1"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, hdr := range []string{"X-Forwarded-For", "X-Real-IP"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.7:4444"
		req.Header.Set(hdr, "127.0.0.1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, "spoofed %s must not pass the allowlist", hdr)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRedactTokenInRequestURI(t *testing.T) {
	var logged, seenToken string
	h := RedactTokenInRequestURI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logged, seenToken = r.RequestURI, r.URL.Query().Get("token")
	}))
	req := httptest.NewRequest(http.MethodGet, "/.well-known/as5-configuration?token=s3cret&x=1", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	assert.NotContains(t, logged, "s3cret")
	assert.Contains(t, logged, "token=REDACTED")
	assert.Equal(t, "s3cret", seenToken, "handlers still read the real token from r.URL")
}

func TestWebSocketSameOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://node.local:8080/admin/dashboard/ws", nil)
	assert.True(t, sameOrigin(req), "no Origin (non-browser client)")
	req.Header.Set("Origin", "http://node.local:8080")
	assert.True(t, sameOrigin(req))
	req.Header.Set("Origin", "https://evil.example")
	assert.False(t, sameOrigin(req))
}
