package discovery

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// TokenStore manages single-use security tokens for partner registration
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

// GenerateToken creates a new single-use token that expires after the given duration
func (ts *TokenStore) GenerateToken(expiresIn time.Duration) (string, error) {
	// Generate random token
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	token := base64.URLEncoding.EncodeToString(bytes)

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
