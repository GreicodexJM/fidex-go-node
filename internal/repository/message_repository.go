package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/domain"
)

// isUniqueConstraint reports whether the given SQL error was raised because
// of a UNIQUE constraint violation. We match on the error text rather than
// on driver-specific types so the check works for both modernc.org/sqlite
// (production) and mattn/go-sqlite3 (tests) — both formats include the
// canonical "UNIQUE constraint failed" string.
func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}

// SQLiteMessageRepository implements domain.MessageRepository for SQLite
type SQLiteMessageRepository struct {
	db *sql.DB
}

// NewSQLiteMessageRepository creates a new SQLite message repository
func NewSQLiteMessageRepository(db *sql.DB) *SQLiteMessageRepository {
	return &SQLiteMessageRepository{db: db}
}

// Create inserts a new message into the database.
//
// Per ADR-0002, `job_type` is the dispatch discriminator and is now a
// first-class column on the queue table. To preserve compatibility with
// callers that have not yet been migrated to set msg.JobType explicitly,
// Create resolves the value with this precedence:
//  1. msg.JobType (caller-supplied, canonical).
//  2. The `job_type` field inside the JSON payload (legacy convention).
//  3. constants.JobTypeProcessOutbound (the default outbound business-
//     document path that predates the column).
// The resolved value is then mirrored back onto msg so the caller sees
// what was actually persisted.
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

	// Resolve job_type. The column is the source of truth for dispatch;
	// the JSON copy stays for cross-system portability per ADR-0002.
	msg.JobType = resolveJobType(msg.JobType, msg.Payload)

	// Insert the message
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO messages (message_id, direction, status, payload, job_type, retry_count, next_retry_at, last_error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.MessageID,
		msg.Direction,
		msg.Status,
		msg.Payload,
		msg.JobType,
		msg.RetryCount,
		msg.NextRetryAt,
		msg.LastError,
		msg.CreatedAt,
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("message_id %q already exists: %w", msg.MessageID, domain.ErrDuplicateMessageID)
		}
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

// resolveJobType applies the ADR-0002 precedence: explicit field beats
// payload-embedded value, payload-embedded value beats the default.
// Exported only via Create; kept private to the repo so the precedence
// rule lives in exactly one place.
func resolveJobType(explicit, payload string) string {
	if explicit != "" {
		return explicit
	}
	if payload != "" {
		var probe struct {
			JobType string `json:"job_type"`
		}
		if err := json.Unmarshal([]byte(payload), &probe); err == nil && probe.JobType != "" {
			return probe.JobType
		}
	}
	return constants.JobTypeProcessOutbound
}

// GetByID retrieves a message by its message_id
func (r *SQLiteMessageRepository) GetByID(ctx context.Context, messageID string) (*domain.Message, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message_id cannot be empty")
	}

	var msg domain.Message
	err := r.db.QueryRowContext(ctx,
		`SELECT id, message_id, direction, status, payload, job_type, retry_count, next_retry_at, last_error, created_at
		 FROM messages WHERE message_id = ?`,
		messageID,
	).Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.JobType, &msg.RetryCount, &msg.NextRetryAt, &msg.LastError, &msg.CreatedAt)

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
		`SELECT id, message_id, direction, status, payload, job_type, retry_count, next_retry_at, last_error, created_at
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
		if err := rows.Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.JobType, &msg.RetryCount, &msg.NextRetryAt, &msg.LastError, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, &msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// ListByStatusAndJobType retrieves messages filtered by both status and
// job_type. Backed by the (status, job_type, created_at) composite index
// introduced in ADR-0002. Empty jobType is rejected — callers that want
// status-only filtering should use ListByStatus.
func (r *SQLiteMessageRepository) ListByStatusAndJobType(
	ctx context.Context,
	status domain.MessageStatus,
	jobType string,
) ([]*domain.Message, error) {
	if jobType == "" {
		return nil, fmt.Errorf("job_type cannot be empty")
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, message_id, direction, status, payload, job_type, retry_count, next_retry_at, last_error, created_at
		 FROM messages WHERE status = ? AND job_type = ? ORDER BY created_at ASC`,
		status,
		jobType,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages by status+job_type: %w", err)
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.JobType, &msg.RetryCount, &msg.NextRetryAt, &msg.LastError, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, &msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// ListPaginated returns messages ordered by created_at DESC.
// If statusFilter is non-nil, only messages with that status are returned.
// total counts all matching rows ignoring limit/offset so the caller can
// build pagination metadata.
func (r *SQLiteMessageRepository) ListPaginated(
	ctx context.Context,
	statusFilter *domain.MessageStatus,
	limit, offset int,
) ([]*domain.Message, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	args := []interface{}{}
	listQuery := `SELECT id, message_id, direction, status, payload, job_type, retry_count, next_retry_at, last_error, created_at FROM messages`
	countQuery := `SELECT COUNT(*) FROM messages`

	if statusFilter != nil {
		listQuery += ` WHERE status = ?`
		countQuery += ` WHERE status = ?`
		args = append(args, *statusFilter)
	}
	listQuery += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count messages: %w", err)
	}

	rowArgs := append(append([]interface{}{}, args...), limit, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, rowArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	messages := []*domain.Message{}
	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.JobType, &msg.RetryCount, &msg.NextRetryAt, &msg.LastError, &msg.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, &msg)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, total, nil
}

// CountByStatusSince returns per-status counts of messages created at or after
// the given instant. Statuses with zero matches are omitted from the result.
func (r *SQLiteMessageRepository) CountByStatusSince(
	ctx context.Context,
	since time.Time,
) (map[domain.MessageStatus]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM messages WHERE created_at >= ? GROUP BY status`,
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query message counts: %w", err)
	}
	defer rows.Close()

	counts := map[domain.MessageStatus]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count row: %w", err)
		}
		counts[domain.MessageStatus(status)] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating counts: %w", err)
	}
	return counts, nil
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
