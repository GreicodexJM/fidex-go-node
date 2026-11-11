package discovery

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// TokenStore manages the single-use security tokens a RESPONDER mints for
// partner registration (spec §6.2 / §6.4). The responder hands a token out
// inside its AS5 configuration URL (e.g. via the dashboard QR); the initiator
// echoes it as security_token in its signed registration request, and the
// responder spends it with ValidateAndConsume before fetching anything the
// initiator names.
type TokenStore struct {
	tokens map[string]*tokenData
	mu     sync.RWMutex
}

type tokenData struct {
	createdAt time.Time
	expiresAt time.Time
	used      bool
}

// NewTokenStore creates a new token store
func NewTokenStore() *TokenStore {
	store := &TokenStore{
		tokens: make(map[string]*tokenData),
	}

	// Start cleanup goroutine
	go store.cleanupExpiredTokens()

	return store
}

// GenerateToken creates a new single-use token that expires after the given
// duration. 256 bits from crypto/rand (spec §6.4 asks for at least 128),
// encoded base64url WITHOUT padding so it can sit in a URL query unescaped.
func (ts *TokenStore) GenerateToken(expiresIn time.Duration) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(bytes)

	now := time.Now()
	ts.mu.Lock()
	ts.tokens[token] = &tokenData{
		createdAt: now,
		expiresAt: now.Add(expiresIn),
		used:      false,
	}
	ts.mu.Unlock()

	return token, nil
}

// ValidateAndConsume validates a token and marks it as used
// Returns true if the token is valid and hasn't been used
func (ts *TokenStore) ValidateAndConsume(token string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	data, exists := ts.tokens[token]
	if !exists {
		return false
	}

	// Check if expired
	if time.Now().After(data.expiresAt) {
		delete(ts.tokens, token)
		return false
	}

	// Check if already used
	if data.used {
		return false
	}

	// Mark as used
	data.used = true

	return true
}

// IsOpen reports whether token exists, has not expired and has not been used,
// WITHOUT consuming it. Used to gate GET of the AS5 configuration when the
// caller presents a token; only registration spends one.
func (ts *TokenStore) IsOpen(token string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	data, exists := ts.tokens[token]
	if !exists {
		return false
	}
	return !data.used && time.Now().Before(data.expiresAt)
}

// cleanupExpiredTokens periodically removes expired tokens
func (ts *TokenStore) cleanupExpiredTokens() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ts.mu.Lock()
		now := time.Now()
		for token, data := range ts.tokens {
			if now.After(data.expiresAt) || data.used {
				delete(ts.tokens, token)
			}
		}
		ts.mu.Unlock()
	}
}
