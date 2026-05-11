package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"fidex-node/internal/domain"
)

// SQLiteSessionRepository implements domain.SessionRepository for SQLite
type SQLiteSessionRepository struct {
	db *sql.DB
}

// NewSQLiteSessionRepository creates a new SQLite session repository
func NewSQLiteSessionRepository(db *sql.DB) *SQLiteSessionRepository {
	return &SQLiteSessionRepository{db: db}
}

// Create creates a new session
func (r *SQLiteSessionRepository) Create(ctx context.Context, session *domain.Session) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}

	if session.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (session_id, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		session.SessionID, session.UserID, session.ExpiresAt, session.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// GetByID retrieves a session by session_id
func (r *SQLiteSessionRepository) GetByID(ctx context.Context, sessionID string) (*domain.Session, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	var session domain.Session
	err := r.db.QueryRowContext(ctx,
		`SELECT session_id, user_id, expires_at, created_at FROM sessions WHERE session_id = ?`,
		sessionID,
	).Scan(&session.SessionID, &session.UserID, &session.ExpiresAt, &session.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return &session, nil
}

// Delete removes a session
func (r *SQLiteSessionRepository) Delete(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	result, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return nil
}

// DeleteByUserID removes all sessions belonging to the given user.
// Used when the user account itself is deleted, to invalidate active logins.
func (r *SQLiteSessionRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete sessions by user_id: %w", err)
	}
	return nil
}

// DeleteExpired removes all expired sessions
func (r *SQLiteSessionRepository) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now())
	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}

	return nil
}
