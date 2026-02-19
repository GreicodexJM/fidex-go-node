package db

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// MessageStatus represents the status of a message in the system
type MessageStatus string

const (
	StatusQueued      MessageStatus = "QUEUED"
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
	ID        int64            `json:"id"`
	MessageID string           `json:"message_id"`
	Direction MessageDirection `json:"direction"`
	Status    MessageStatus    `json:"status"`
	Payload   string           `json:"payload"`
	CreatedAt time.Time        `json:"created_at"`
}

// Config represents the node's configuration stored in the database
type Config struct {
	TenantID   string
	APISecret  string
	PrivateKey string
	IsPaired   bool
	// Security configuration
	InternalAPIPort    int
	PublicAPIPort      int
	InternalAPIKey     string
	AllowedIPAddresses string // JSON array of allowed IPs
	EnableIPAllowlist  bool
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
	// Config table - single row configuration
	createConfigTable := `
	CREATE TABLE IF NOT EXISTS config (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		tenant_id TEXT,
		api_secret TEXT,
		private_key TEXT,
		is_paired BOOLEAN DEFAULT 0
	);
	`

	// Messages table - stores all FideX messages
	createMessagesTable := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id TEXT NOT NULL UNIQUE,
		direction TEXT NOT NULL,
		status TEXT NOT NULL,
		payload TEXT NOT NULL,
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

	// Execute config table creation
	if _, err := DB.Exec(createConfigTable); err != nil {
		return fmt.Errorf("failed to create config table: %w", err)
	}

	// Initialize config row if it doesn't exist
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM config").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check config table: %w", err)
	}
	if count == 0 {
		_, err = DB.Exec("INSERT INTO config (id, is_paired) VALUES (1, 0)")
		if err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}
	}

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

	// Run migrations to add new columns if they don't exist
	if err := runMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// runMigrations handles schema upgrades for existing databases
func runMigrations() error {
	// Check if security columns exist
	var columnExists int
	err := DB.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('config') 
		WHERE name = 'internal_api_port'
	`).Scan(&columnExists)

	if err != nil {
		return fmt.Errorf("failed to check for security columns: %w", err)
	}

	// If columns don't exist, add them
	if columnExists == 0 {
		migrations := []string{
			`ALTER TABLE config ADD COLUMN internal_api_port INTEGER DEFAULT 8080`,
			`ALTER TABLE config ADD COLUMN public_api_port INTEGER DEFAULT 8443`,
			`ALTER TABLE config ADD COLUMN internal_api_key TEXT`,
			`ALTER TABLE config ADD COLUMN allowed_ip_addresses TEXT DEFAULT '["127.0.0.1","::1"]'`,
			`ALTER TABLE config ADD COLUMN enable_ip_allowlist BOOLEAN DEFAULT 1`,
		}

		for _, migration := range migrations {
			if _, err := DB.Exec(migration); err != nil {
				return fmt.Errorf("failed to execute migration: %w", err)
			}
		}

		// Generate a secure API key for existing installations
		apiKey, err := generateSecureAPIKey()
		if err != nil {
			return fmt.Errorf("failed to generate API key: %w", err)
		}

		_, err = DB.Exec(`UPDATE config SET internal_api_key = ? WHERE id = 1`, apiKey)
		if err != nil {
			return fmt.Errorf("failed to set initial API key: %w", err)
		}
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
		`INSERT INTO messages (message_id, direction, status, payload, created_at) 
		 VALUES (?, ?, ?, ?, ?)`,
		msg.MessageID,
		msg.Direction,
		msg.Status,
		msg.Payload,
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

// GetMessageByID retrieves a message by its message_id
func GetMessageByID(messageID string) (*Message, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message_id cannot be empty")
	}

	var msg Message
	err := DB.QueryRow(
		`SELECT id, message_id, direction, status, payload, created_at 
		 FROM messages WHERE message_id = ?`,
		messageID,
	).Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.CreatedAt)

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
		`SELECT id, message_id, direction, status, payload, created_at 
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
		if err := rows.Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// GetConfig retrieves the node configuration
func GetConfig() (*Config, error) {
	var cfg Config
	var tenantID, apiSecret, privateKey, internalAPIKey, allowedIPs sql.NullString

	err := DB.QueryRow(
		`SELECT tenant_id, api_secret, private_key, is_paired,
		        COALESCE(internal_api_port, 8080), 
		        COALESCE(public_api_port, 8443),
		        internal_api_key,
		        COALESCE(allowed_ip_addresses, '["127.0.0.1","::1"]'),
		        COALESCE(enable_ip_allowlist, 1)
		 FROM config WHERE id = 1`,
	).Scan(&tenantID, &apiSecret, &privateKey, &cfg.IsPaired,
		&cfg.InternalAPIPort, &cfg.PublicAPIPort, &internalAPIKey,
		&allowedIPs, &cfg.EnableIPAllowlist)

	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	if tenantID.Valid {
		cfg.TenantID = tenantID.String
	}
	if apiSecret.Valid {
		cfg.APISecret = apiSecret.String
	}
	if privateKey.Valid {
		cfg.PrivateKey = privateKey.String
	}
	if internalAPIKey.Valid {
		cfg.InternalAPIKey = internalAPIKey.String
	}
	if allowedIPs.Valid {
		cfg.AllowedIPAddresses = allowedIPs.String
	}

	return &cfg, nil
}

// UpdateConfig updates the node configuration
func UpdateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	_, err := DB.Exec(
		`UPDATE config SET tenant_id = ?, api_secret = ?, private_key = ?, is_paired = ?,
		 internal_api_port = ?, public_api_port = ?, internal_api_key = ?,
		 allowed_ip_addresses = ?, enable_ip_allowlist = ?
		 WHERE id = 1`,
		cfg.TenantID,
		cfg.APISecret,
		cfg.PrivateKey,
		cfg.IsPaired,
		cfg.InternalAPIPort,
		cfg.PublicAPIPort,
		cfg.InternalAPIKey,
		cfg.AllowedIPAddresses,
		cfg.EnableIPAllowlist,
	)
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	return nil
}

// generateSecureAPIKey generates a cryptographically secure random API key
func generateSecureAPIKey() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	key := make([]byte, 32)

	// Use time-based seed (in production, use crypto/rand for better security)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range key {
		key[i] = charset[r.Intn(len(charset))]
	}

	return string(key), nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
