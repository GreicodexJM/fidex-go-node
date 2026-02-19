package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"fidex-node/internal/db"
)

// Session represents a user session
type Session struct {
	SessionID string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
}

// User represents a dashboard user
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// SessionDuration is how long sessions last (24 hours)
const SessionDuration = 24 * time.Hour

// GenerateSessionID creates a cryptographically secure session ID
func GenerateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// CreateSession creates a new session for a user
func CreateSession(userID int64) (*Session, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return nil, err
	}

	session := &Session{
		SessionID: sessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(SessionDuration),
		CreatedAt: time.Now(),
	}

	// Save to database
	_, err = db.DB.Exec(
		`INSERT INTO sessions (session_id, user_id, expires_at, created_at)
		 VALUES (?, ?, ?, ?)`,
		session.SessionID, session.UserID, session.ExpiresAt, session.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetSession retrieves a session by ID
func GetSession(sessionID string) (*Session, error) {
	var session Session
	err := db.DB.QueryRow(
		`SELECT session_id, user_id, expires_at, created_at
		 FROM sessions WHERE session_id = ?`,
		sessionID,
	).Scan(&session.SessionID, &session.UserID, &session.ExpiresAt, &session.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		DeleteSession(sessionID)
		return nil, fmt.Errorf("session expired")
	}

	return &session, nil
}

// DeleteSession removes a session from the database
func DeleteSession(sessionID string) error {
	_, err := db.DB.Exec(`DELETE FROM sessions WHERE session_id = ?`, sessionID)
	return err
}

// CleanupExpiredSessions removes all expired sessions
func CleanupExpiredSessions() error {
	_, err := db.DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now())
	return err
}

// GetUserByID retrieves a user by ID
func GetUserByID(userID int64) (*User, error) {
	var user User
	err := db.DB.QueryRow(
		`SELECT id, username, password_hash, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by username
func GetUserByUsername(username string) (*User, error) {
	var user User
	err := db.DB.QueryRow(
		`SELECT id, username, password_hash, created_at
		 FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

// CreateUser creates a new user (used for initial setup)
func CreateUser(username, passwordHash string) (*User, error) {
	result, err := db.DB.Exec(
		`INSERT INTO users (username, password_hash, created_at)
		 VALUES (?, ?, ?)`,
		username, passwordHash, time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
	}, nil
}
