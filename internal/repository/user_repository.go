package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"fidex-node/internal/domain"
)

// SQLiteUserRepository implements domain.UserRepository for SQLite
type SQLiteUserRepository struct {
	db *sql.DB
}

// NewSQLiteUserRepository creates a new SQLite user repository
func NewSQLiteUserRepository(db *sql.DB) *SQLiteUserRepository {
	return &SQLiteUserRepository{db: db}
}

// Create creates a new user
func (r *SQLiteUserRepository) Create(ctx context.Context, username, passwordHash string) (*domain.User, error) {
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}

	if passwordHash == "" {
		return nil, fmt.Errorf("password hash cannot be empty")
	}

	now := time.Now()
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username, passwordHash, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return &domain.User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    now,
	}, nil
}

// GetByID retrieves a user by ID
func (r *SQLiteUserRepository) GetByID(ctx context.Context, userID int64) (*domain.User, error) {
	var user domain.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at FROM users WHERE id = ?`,
		userID,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %d", userID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetByUsername retrieves a user by username
func (r *SQLiteUserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}

	var user domain.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// List retrieves all users
func (r *SQLiteUserRepository) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, username, password_hash, created_at FROM users ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	return users, nil
}

// Delete removes a user
func (r *SQLiteUserRepository) Delete(ctx context.Context, userID int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found: %d", userID)
	}

	return nil
}

// UpdatePassword updates a user's password hash
func (r *SQLiteUserRepository) UpdatePassword(ctx context.Context, userID int64, newPasswordHash string) error {
	if newPasswordHash == "" {
		return fmt.Errorf("password hash cannot be empty")
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE id = ?`,
		newPasswordHash, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found: %d", userID)
	}

	return nil
}
