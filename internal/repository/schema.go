package repository

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenSQLite opens a SQLite database connection with the production PRAGMAs
// applied (WAL journal, foreign keys, busy timeout). The caller owns the
// returned *sql.DB and must Close it on shutdown.
func OpenSQLite(dbPath string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to apply pragma %q: %w", pragma, err)
		}
	}

	return conn, nil
}

// InitSchema creates the FideX node schema (idempotent — uses IF NOT EXISTS).
// Run once after opening the connection.
func InitSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id TEXT NOT NULL UNIQUE,
			direction TEXT NOT NULL,
			status TEXT NOT NULL,
			payload TEXT NOT NULL,
			retry_count INTEGER DEFAULT 0,
			next_retry_at DATETIME,
			last_error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_message_id ON messages(message_id);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_status ON messages(status);`,
		`CREATE TABLE IF NOT EXISTS trading_partners (
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
		);`,
		`CREATE INDEX IF NOT EXISTS idx_trading_partners_partner_id ON trading_partners(partner_id);`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute schema statement: %w\n%s", err, stmt)
		}
	}

	return nil
}
