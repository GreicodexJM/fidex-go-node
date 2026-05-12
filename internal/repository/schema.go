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
//
// In addition to the base CREATE statements, InitSchema runs a small set of
// additive migrations that bring older databases (created before a given
// column existed) up to the current shape. Migrations are written to be
// idempotent and safe to re-run on every boot; see migrateMessagesJobType.
func InitSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id TEXT NOT NULL UNIQUE,
			direction TEXT NOT NULL,
			status TEXT NOT NULL,
			payload TEXT NOT NULL,
			job_type VARCHAR(64) NOT NULL DEFAULT '',
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

	// Migration: messages.job_type column (ADR-0002, FID-6).
	if err := migrateMessagesJobType(db); err != nil {
		return fmt.Errorf("failed to migrate messages.job_type: %w", err)
	}

	return nil
}

// migrateMessagesJobType ensures the `messages` table carries the
// first-class `job_type` column required by ADR-0002 / FID-6.
//
// Behavior:
//   - If the column is missing (DB created on a prior schema), ALTER TABLE
//     adds it with DEFAULT '' so the ALTER itself is non-failing.
//   - Any rows whose `job_type` column is still empty are backfilled by
//     reading `job_type` out of the JSON payload. Rows whose payload does
//     not name a job_type fall back to "process_outbound" (the canonical
//     business-document dispatch path that predates the column).
//   - A composite index (status, job_type, created_at) is created so the
//     dominant poll query `WHERE status='QUEUED' AND job_type=? ORDER BY
//     created_at` is index-served.
//
// All statements are idempotent and safe to re-run on every boot.
func migrateMessagesJobType(db *sql.DB) error {
	hasColumn, err := columnExists(db, "messages", "job_type")
	if err != nil {
		return fmt.Errorf("failed to introspect messages columns: %w", err)
	}
	if !hasColumn {
		// SQLite ADD COLUMN with a constant default is O(1) — it does not
		// rewrite existing rows. The default is the empty string so we can
		// distinguish "not yet backfilled" from a deliberately-stamped row.
		if _, err := db.Exec(`ALTER TABLE messages ADD COLUMN job_type VARCHAR(64) NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("ALTER TABLE messages ADD COLUMN job_type failed: %w", err)
		}
	}

	// Backfill: existing rows have job_type=''. Read it out of the JSON
	// payload (the convention pre-ADR was to redundantly stamp job_type in
	// the payload). Anything that doesn't name a job_type — including rows
	// whose payload is not valid JSON at all — is treated as a regular
	// outbound business document. The only pre-existing non-default
	// dispatch path was send_jmdn.
	//
	// We gate the json_extract call with json_valid(payload) so a
	// malformed historical row does not abort the migration: SQLite's
	// json_extract on invalid JSON raises a "malformed JSON" error, but
	// the AND-short-circuit on json_valid() prevents that evaluation.
	if _, err := db.Exec(
		`UPDATE messages
		   SET job_type = json_extract(payload, '$.job_type')
		 WHERE job_type = ''
		   AND json_valid(payload)
		   AND json_extract(payload, '$.job_type') IS NOT NULL`,
	); err != nil {
		return fmt.Errorf("backfill messages.job_type from payload failed: %w", err)
	}
	if _, err := db.Exec(
		`UPDATE messages SET job_type = 'process_outbound' WHERE job_type = ''`,
	); err != nil {
		return fmt.Errorf("backfill messages.job_type default failed: %w", err)
	}

	// Composite index for the dominant poll query. Created last so the
	// backfill above didn't pay index-maintenance cost on a write storm.
	if _, err := db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_messages_status_jobtype_created
		    ON messages(status, job_type, created_at)`,
	); err != nil {
		return fmt.Errorf("create idx_messages_status_jobtype_created failed: %w", err)
	}

	return nil
}

// columnExists reports whether the named column is present on the given
// table. Uses SQLite's PRAGMA table_info, which returns one row per column
// with the column name in position 1 (the 2nd field, "name").
func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("PRAGMA table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("scan PRAGMA table_info row: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterating PRAGMA table_info rows: %w", err)
	}
	return false, nil
}
