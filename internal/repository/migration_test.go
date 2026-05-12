package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/domain"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrateMessagesJobType_AddsColumn asserts that migrateMessagesJobType
// promotes a pre-ADR-0002 messages table (no job_type column) to the
// current shape: column present, default '' for new rows, composite index
// in place. This is the "old DB on disk, new binary" scenario.
func TestMigrateMessagesJobType_AddsColumn(t *testing.T) {
	db := setupLegacyTestDB(t)
	defer db.Close()

	// Sanity check: starting state has no job_type column.
	has, err := columnExists(db, "messages", "job_type")
	require.NoError(t, err)
	require.False(t, has, "legacy schema must not have job_type column before migration")

	// Run the migration.
	require.NoError(t, migrateMessagesJobType(db))

	// Column must now exist.
	has, err = columnExists(db, "messages", "job_type")
	require.NoError(t, err)
	assert.True(t, has, "job_type column must exist after migration")

	// The composite index must exist.
	var idxName string
	err = db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name=?`,
		"idx_messages_status_jobtype_created",
	).Scan(&idxName)
	require.NoError(t, err)
	assert.Equal(t, "idx_messages_status_jobtype_created", idxName)
}

// TestMigrateMessagesJobType_BackfillFromPayload asserts that existing
// rows get their job_type column backfilled from the JSON payload's
// embedded job_type field, with the canonical process_outbound default
// applied to rows whose payload does not name one.
func TestMigrateMessagesJobType_BackfillFromPayload(t *testing.T) {
	db := setupLegacyTestDB(t)
	defer db.Close()

	// Seed three rows on the legacy schema: a J-MDN job (payload names
	// send_jmdn), a business document (no job_type in payload), and a
	// malformed payload (not even valid JSON — must still fall back).
	seedRows := []struct {
		messageID string
		payload   string
	}{
		{"msg-jmdn", `{"job_type":"send_jmdn","original_message_id":"orig-1"}`},
		{"msg-biz", `{"destination_partner_id":"urn:partner:1","document_type":"PO","payload":{"hi":"there"}}`},
		{"msg-junk", `{not even json`},
	}
	for _, r := range seedRows {
		_, err := db.Exec(
			`INSERT INTO messages (message_id, direction, status, payload, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			r.messageID, "OUTBOUND", "QUEUED", r.payload, time.Now(),
		)
		require.NoError(t, err)
	}

	require.NoError(t, migrateMessagesJobType(db))

	// Verify backfilled values.
	got := map[string]string{}
	rows, err := db.Query(`SELECT message_id, job_type FROM messages`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var mid, jt string
		require.NoError(t, rows.Scan(&mid, &jt))
		got[mid] = jt
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, constants.JobTypeSendJMDN, got["msg-jmdn"],
		"row with send_jmdn in payload must be backfilled to send_jmdn")
	assert.Equal(t, constants.JobTypeProcessOutbound, got["msg-biz"],
		"business-document row must default to process_outbound")
	assert.Equal(t, constants.JobTypeProcessOutbound, got["msg-junk"],
		"malformed-JSON payload row must still get the process_outbound default — no data loss")

	// No row may be left with an empty job_type after migration: that
	// would mean a queued row is unroutable.
	var empties int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM messages WHERE job_type = ''`).Scan(&empties))
	assert.Equal(t, 0, empties, "no rows must remain with empty job_type after backfill")
}

// TestMigrateMessagesJobType_Idempotent asserts the migration can run
// repeatedly (which is what InitSchema actually does on every boot)
// without disturbing already-backfilled rows.
func TestMigrateMessagesJobType_Idempotent(t *testing.T) {
	db := setupLegacyTestDB(t)
	defer db.Close()

	_, err := db.Exec(
		`INSERT INTO messages (message_id, direction, status, payload, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"msg-1", "OUTBOUND", "QUEUED", `{"job_type":"send_jmdn"}`, time.Now(),
	)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		require.NoError(t, migrateMessagesJobType(db), "migration must be safe to re-run (pass %d)", i+1)
	}

	var jt string
	require.NoError(t, db.QueryRow(`SELECT job_type FROM messages WHERE message_id=?`, "msg-1").Scan(&jt))
	assert.Equal(t, constants.JobTypeSendJMDN, jt)

	// Subsequent inserts with an explicit job_type must NOT be clobbered
	// by re-running the migration.
	_, err = db.Exec(
		`INSERT INTO messages (message_id, direction, status, payload, job_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"msg-2", "OUTBOUND", "QUEUED", `{}`, "custom_future_type", time.Now(),
	)
	require.NoError(t, err)
	require.NoError(t, migrateMessagesJobType(db))
	require.NoError(t, db.QueryRow(`SELECT job_type FROM messages WHERE message_id=?`, "msg-2").Scan(&jt))
	assert.Equal(t, "custom_future_type", jt,
		"explicitly-stamped job_type values must survive re-running the migration")
}

// TestMessageRepository_Create_StampsJobType_FromField asserts that the
// repository persists msg.JobType verbatim to the column when set
// explicitly.
func TestMessageRepository_Create_StampsJobType_FromField(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	msg := &domain.Message{
		MessageID: "msg-explicit-jt",
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   `{"hello":"world"}`,
		JobType:   constants.JobTypeSendJMDN,
	}
	require.NoError(t, repo.Create(ctx, msg))

	var jt string
	require.NoError(t, db.QueryRow(`SELECT job_type FROM messages WHERE message_id=?`, "msg-explicit-jt").Scan(&jt))
	assert.Equal(t, constants.JobTypeSendJMDN, jt)
}

// TestMessageRepository_Create_StampsJobType_FromPayload asserts that
// when msg.JobType is empty, the repository extracts the value from the
// JSON payload — keeping legacy callers (like the api package's J-MDN
// enqueue) working without code changes there.
func TestMessageRepository_Create_StampsJobType_FromPayload(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	msg := &domain.Message{
		MessageID: "msg-payload-jt",
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   `{"job_type":"send_jmdn","original_message_id":"orig-x"}`,
		// JobType field intentionally empty
	}
	require.NoError(t, repo.Create(ctx, msg))

	// The repository must mirror the resolved value back onto the struct.
	assert.Equal(t, constants.JobTypeSendJMDN, msg.JobType)

	var jt string
	require.NoError(t, db.QueryRow(`SELECT job_type FROM messages WHERE message_id=?`, "msg-payload-jt").Scan(&jt))
	assert.Equal(t, constants.JobTypeSendJMDN, jt)
}

// TestMessageRepository_Create_StampsJobType_Default asserts that rows
// with neither an explicit field nor a payload-embedded job_type default
// to process_outbound — the canonical business-document path.
func TestMessageRepository_Create_StampsJobType_Default(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	msg := &domain.Message{
		MessageID: "msg-default-jt",
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   `{"destination_partner_id":"urn:p:1","document_type":"PO","payload":{}}`,
	}
	require.NoError(t, repo.Create(ctx, msg))

	var jt string
	require.NoError(t, db.QueryRow(`SELECT job_type FROM messages WHERE message_id=?`, "msg-default-jt").Scan(&jt))
	assert.Equal(t, constants.JobTypeProcessOutbound, jt)
	assert.Equal(t, constants.JobTypeProcessOutbound, msg.JobType,
		"resolved value must be mirrored back onto the struct")
}

// TestMessageRepository_ListByStatusAndJobType asserts the new method
// returns only the intersection of status AND job_type, and that an
// empty job_type is rejected at the API surface.
func TestMessageRepository_ListByStatusAndJobType(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	seed := []*domain.Message{
		{MessageID: "a", Direction: domain.DirectionOutbound, Status: domain.StatusQueued, Payload: `{}`, JobType: constants.JobTypeSendJMDN},
		{MessageID: "b", Direction: domain.DirectionOutbound, Status: domain.StatusQueued, Payload: `{}`, JobType: constants.JobTypeSendJMDN},
		{MessageID: "c", Direction: domain.DirectionOutbound, Status: domain.StatusQueued, Payload: `{}`, JobType: constants.JobTypeProcessOutbound},
		{MessageID: "d", Direction: domain.DirectionOutbound, Status: domain.StatusDelivered, Payload: `{}`, JobType: constants.JobTypeSendJMDN},
	}
	for _, m := range seed {
		require.NoError(t, repo.Create(ctx, m))
	}

	jmdnQueued, err := repo.ListByStatusAndJobType(ctx, domain.StatusQueued, constants.JobTypeSendJMDN)
	require.NoError(t, err)
	assert.Len(t, jmdnQueued, 2, "exactly 2 rows match status=QUEUED AND job_type=send_jmdn")

	bizQueued, err := repo.ListByStatusAndJobType(ctx, domain.StatusQueued, constants.JobTypeProcessOutbound)
	require.NoError(t, err)
	assert.Len(t, bizQueued, 1)
	assert.Equal(t, "c", bizQueued[0].MessageID)

	_, err = repo.ListByStatusAndJobType(ctx, domain.StatusQueued, "")
	assert.Error(t, err, "empty job_type must be rejected — use ListByStatus for status-only queries")
}

// TestInitSchema_FreshDB asserts that running InitSchema on a fresh
// in-memory DB produces a schema that already includes job_type +
// composite index — no migration noise for new installations.
func TestInitSchema_FreshDB(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, InitSchema(db))

	has, err := columnExists(db, "messages", "job_type")
	require.NoError(t, err)
	assert.True(t, has, "fresh DB must have job_type column after InitSchema")

	var idxCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`,
		"idx_messages_status_jobtype_created",
	).Scan(&idxCount))
	assert.Equal(t, 1, idxCount, "composite index must exist on a fresh DB")
}
