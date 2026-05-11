package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"fidex-node/internal/domain"
)

// Re-export domain types for backwards-compatible call sites within this package.
type Session = domain.Session
type User = domain.User

// SessionDuration is how long sessions last (24 hours)
const SessionDuration = 24 * time.Hour

// Service provides session and user authentication operations backed by repositories.
type Service struct {
	sessions domain.SessionRepository
	users    domain.UserRepository
}

// NewService creates a new auth service with the given repositories.
func NewService(sessions domain.SessionRepository, users domain.UserRepository) *Service {
	return &Service{sessions: sessions, users: users}
}

// GenerateSessionID creates a cryptographically secure session ID.
func GenerateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// CreateSession creates a new session for a user.
func (s *Service) CreateSession(ctx context.Context, userID int64) (*Session, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &Session{
		SessionID: sessionID,
		UserID:    userID,
		ExpiresAt: now.Add(SessionDuration),
		CreatedAt: now,
	}

	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetSession retrieves a session by ID, deleting it if expired.
func (s *Service) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	if time.Now().After(session.ExpiresAt) {
		_ = s.sessions.Delete(ctx, sessionID)
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

// DeleteSession removes a session.
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

// CleanupExpiredSessions removes all expired sessions.
func (s *Service) CleanupExpiredSessions(ctx context.Context) error {
	return s.sessions.DeleteExpired(ctx)
}

// GetUserByID retrieves a user by ID.
func (s *Service) GetUserByID(ctx context.Context, userID int64) (*User, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return user, nil
}

// GetUserByUsername retrieves a user by username.
func (s *Service) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return user, nil
}

// CreateUser creates a new user.
func (s *Service) CreateUser(ctx context.Context, username, passwordHash string) (*User, error) {
	user, err := s.users.Create(ctx, username, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return user, nil
}

// CountUsers returns the total number of users in the system.
func (s *Service) CountUsers(ctx context.Context) (int, error) {
	users, err := s.users.List(ctx)
	if err != nil {
		return 0, err
	}
	return len(users), nil
}
