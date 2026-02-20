package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"fidex-node/internal/domain"
)

// SQLiteMessageRepository implements domain.MessageRepository for SQLite
type SQLiteMessageRepository struct {
	db *sql.DB
}

// NewSQLiteMessageRepository creates a new SQLite message repository
func NewSQLiteMessageRepository(db *sql.DB) *SQLiteMessageRepository {
	return &SQLiteMessageRepository{db: db}
}

// Create inserts a new message into the database
func (r *SQLiteMessageRepository) Create(ctx context.Context, msg *domain.Message) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}

	if msg.MessageID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}

	if msg.Payload == "" {
		return fmt.Errorf("payload cannot be empty")
	}

	// Set created_at if not already set
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	// Insert the message
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO messages (message_id, direction, status, payload, retry_count, next_retry_at, last_error, created_at) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.MessageID,
		msg.Direction,
		msg.Status,
		msg.Payload,
		msg.RetryCount,
		msg.NextRetryAt,
		msg.LastError,
		msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	// Get the auto-generated ID
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	msg.ID = id
	return nil
}

// GetByID retrieves a message by its message_id
func (r *SQLiteMessageRepository) GetByID(ctx context.Context, messageID string) (*domain.Message, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message_id cannot be empty")
	}

	var msg domain.Message
	err := r.db.QueryRowContext(ctx,
		`SELECT id, message_id, direction, status, payload, retry_count, next_retry_at, last_error, created_at 
		 FROM messages WHERE message_id = ?`,
		messageID,
	).Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.RetryCount, &msg.NextRetryAt, &msg.LastError, &msg.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("message not found: %s", messageID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	return &msg, nil
}

// UpdateStatus updates the status of a message by its message_id
func (r *SQLiteMessageRepository) UpdateStatus(ctx context.Context, messageID string, status domain.MessageStatus) error {
	if messageID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}

	// Update the message status
	result, err := r.db.ExecContext(ctx,
		`UPDATE messages SET status = ? WHERE message_id = ?`,
		status,
		messageID,
	)
	if err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}

// UpdateRetryInfo updates the retry information for a message
func (r *SQLiteMessageRepository) UpdateRetryInfo(ctx context.Context, messageID string, retryCount int, nextRetryAt *time.Time, lastError string) error {
	if messageID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}

	_, err := r.db.ExecContext(ctx,
		`UPDATE messages SET retry_count = ?, next_retry_at = ?, last_error = ? WHERE message_id = ?`,
		retryCount,
		nextRetryAt,
		lastError,
		messageID,
	)
	if err != nil {
		return fmt.Errorf("failed to update message retry info: %w", err)
	}

	return nil
}

// ListByStatus retrieves all messages with the specified status
func (r *SQLiteMessageRepository) ListByStatus(ctx context.Context, status domain.MessageStatus) ([]*domain.Message, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, message_id, direction, status, payload, retry_count, next_retry_at, last_error, created_at 
		 FROM messages WHERE status = ? ORDER BY created_at ASC`,
		status,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages by status: %w", err)
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.RetryCount, &msg.NextRetryAt, &msg.LastError, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, &msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// Delete removes a message by its message_id
func (r *SQLiteMessageRepository) Delete(ctx context.Context, messageID string) error {
	if messageID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}

	result, err := r.db.ExecContext(ctx, `DELETE FROM messages WHERE message_id = ?`, messageID)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}
