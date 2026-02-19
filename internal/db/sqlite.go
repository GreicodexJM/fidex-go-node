package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// MessageStatus represents the status of a message in the system
type MessageStatus string

const (
	StatusQueued      MessageStatus = "QUEUED"
	StatusSent        MessageStatus = "SENT"
	StatusDelivered   MessageStatus = "DELIVERED"
	StatusFailed      MessageStatus = "FAILED"
	StatusQuarantined MessageStatus = "QUARANTINED"
)

// MessageDirection represents whether a message is inbound or outbound
type MessageDirection string

const (
	DirectionInbound  MessageDirection = "INBOUND"
	DirectionOutbound MessageDirection = "OUTBOUND"
)

// Message represents a FideX AS5 message
type Message struct {
	ID          int64            `json:"id"`
	MessageID   string           `json:"message_id"`
	Direction   MessageDirection `json:"direction"`
	Status      MessageStatus    `json:"status"`
	Payload     string           `json:"payload"`
	RetryCount  int              `json:"retry_count"`
	NextRetryAt *time.Time       `json:"next_retry_at"`
	LastError   string           `json:"last_error"`
	CreatedAt   time.Time        `json:"created_at"`
}

// InitDB initializes the SQLite database connection and creates the schema
func InitDB(dbPath string) error {
	var err error

	// Open the database connection
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := DB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := DB.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set busy timeout to handle concurrent access
	if _, err := DB.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Create the schema
	return createSchema()
}

// createSchema creates all necessary tables if they don't exist
func createSchema() error {
	// Messages table - stores all FideX messages
	createMessagesTable := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id TEXT NOT NULL UNIQUE,
		direction TEXT NOT NULL,
		status TEXT NOT NULL,
		payload TEXT NOT NULL,
		retry_count INTEGER DEFAULT 0,
		next_retry_at DATETIME,
		last_error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	// Create index on message_id for faster lookups
	createMessageIDIndex := `
	CREATE INDEX IF NOT EXISTS idx_messages_message_id ON messages(message_id);
	`

	// Create index on status for queue queries
	createStatusIndex := `
	CREATE INDEX IF NOT EXISTS idx_messages_status ON messages(status);
	`

	// Trading Partners table - stores partner connection profiles and public keys
	createTradingPartnersTable := `
	CREATE TABLE IF NOT EXISTS trading_partners (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		partner_id TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		jwks_url TEXT NOT NULL,
		message_endpoint TEXT,
		mdn_receipt_endpoint TEXT,
		public_key_jwks TEXT,
		last_key_refresh DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	// Create index on partner_id for faster lookups
	createPartnerIDIndex := `
	CREATE INDEX IF NOT EXISTS idx_trading_partners_partner_id ON trading_partners(partner_id);
	`

	// Execute messages table creation
	if _, err := DB.Exec(createMessagesTable); err != nil {
		return fmt.Errorf("failed to create messages table: %w", err)
	}

	// Create indexes
	if _, err := DB.Exec(createMessageIDIndex); err != nil {
		return fmt.Errorf("failed to create message_id index: %w", err)
	}

	if _, err := DB.Exec(createStatusIndex); err != nil {
		return fmt.Errorf("failed to create status index: %w", err)
	}

	// Execute trading partners table creation
	if _, err := DB.Exec(createTradingPartnersTable); err != nil {
		return fmt.Errorf("failed to create trading_partners table: %w", err)
	}

	// Create partner_id index
	if _, err := DB.Exec(createPartnerIDIndex); err != nil {
		return fmt.Errorf("failed to create partner_id index: %w", err)
	}

	// Users table - for dashboard authentication
	createUsersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`

	// Sessions table - for dashboard session management
	createSessionsTable := `
	CREATE TABLE IF NOT EXISTS sessions (
		session_id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	`

	// Create index on expires_at for efficient cleanup
	createSessionsExpiresIndex := `
	CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
	`

	// Execute users table creation
	if _, err := DB.Exec(createUsersTable); err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}

	// Execute sessions table creation
	if _, err := DB.Exec(createSessionsTable); err != nil {
		return fmt.Errorf("failed to create sessions table: %w", err)
	}

	// Create sessions expires index
	if _, err := DB.Exec(createSessionsExpiresIndex); err != nil {
		return fmt.Errorf("failed to create sessions expires index: %w", err)
	}

	return nil
}

// InsertMessage inserts a new message into the database
func InsertMessage(msg *Message) error {
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
	result, err := DB.Exec(
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

// UpdateMessageStatus updates the status of a message by its message_id
func UpdateMessageStatus(messageID string, status string) error {
	if messageID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}

	if status == "" {
		return fmt.Errorf("status cannot be empty")
	}

	// Validate the status is one of the allowed values
	validStatuses := map[string]bool{
		string(StatusQueued):      true,
		string(StatusDelivered):   true,
		string(StatusFailed):      true,
		string(StatusQuarantined): true,
	}

	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}

	// Update the message status
	result, err := DB.Exec(
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

// UpdateMessageRetry updates the retry information for a message
func UpdateMessageRetry(messageID string, retryCount int, nextRetryAt *time.Time, lastError string) error {
	if messageID == "" {
		return fmt.Errorf("message_id cannot be empty")
	}

	_, err := DB.Exec(
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

// GetMessageByID retrieves a message by its message_id
func GetMessageByID(messageID string) (*Message, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message_id cannot be empty")
	}

	var msg Message
	err := DB.QueryRow(
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

// GetQueuedMessages retrieves all messages with QUEUED status
func GetQueuedMessages() ([]Message, error) {
	rows, err := DB.Query(
		`SELECT id, message_id, direction, status, payload, retry_count, next_retry_at, last_error, created_at 
		 FROM messages WHERE status = ? ORDER BY created_at ASC`,
		StatusQueued,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query queued messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.RetryCount, &msg.NextRetryAt, &msg.LastError, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
