package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"fidex-node/internal/domain"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates an in-memory SQLite database for testing.
// The schema mirrors the production messages table including the
// `job_type` column introduced by ADR-0002 / FID-6.
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create messages table schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id TEXT NOT NULL UNIQUE,
			direction TEXT NOT NULL,
			status TEXT NOT NULL,
			payload TEXT NOT NULL,
			job_type VARCHAR(64) NOT NULL DEFAULT '',
			retry_count INTEGER DEFAULT 0,
			next_retry_at DATETIME,
			last_error TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	return db
}

// setupLegacyTestDB creates an in-memory SQLite DB on the pre-ADR-0002
// schema (no job_type column). Used by the migration test to verify the
// boot-time backfill brings older DBs forward without data loss.
func setupLegacyTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id TEXT NOT NULL UNIQUE,
			direction TEXT NOT NULL,
			status TEXT NOT NULL,
			payload TEXT NOT NULL,
			retry_count INTEGER DEFAULT 0,
			next_retry_at DATETIME,
			last_error TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)
	return db
}

func TestMessageRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	t.Run("creates message successfully", func(t *testing.T) {
		msg := &domain.Message{
			MessageID: "msg-123",
			Direction: domain.DirectionOutbound,
			Status:    domain.StatusQueued,
			Payload:   "test payload",
		}

		err := repo.Create(ctx, msg)

		require.NoError(t, err)
		assert.NotZero(t, msg.ID, "ID should be set after creation")
		assert.False(t, msg.CreatedAt.IsZero(), "CreatedAt should be set")
	})

	t.Run("fails with nil message", func(t *testing.T) {
		err := repo.Create(ctx, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("fails with empty message_id", func(t *testing.T) {
		msg := &domain.Message{
			MessageID: "",
			Payload:   "test",
		}

		err := repo.Create(ctx, msg)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message_id cannot be empty")
	})

	t.Run("fails with empty payload", func(t *testing.T) {
		msg := &domain.Message{
			MessageID: "msg-empty-payload",
			Payload:   "",
		}

		err := repo.Create(ctx, msg)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "payload cannot be empty")
	})

	t.Run("fails with duplicate message_id", func(t *testing.T) {
		msg1 := &domain.Message{
			MessageID: "msg-duplicate",
			Direction: domain.DirectionOutbound,
			Status:    domain.StatusQueued,
			Payload:   "payload 1",
		}

		err := repo.Create(ctx, msg1)
		require.NoError(t, err)

		// Try to create another message with the same ID
		msg2 := &domain.Message{
			MessageID: "msg-duplicate",
			Direction: domain.DirectionOutbound,
			Status:    domain.StatusQueued,
			Payload:   "payload 2",
		}

		err = repo.Create(ctx, msg2)
		assert.Error(t, err)
		// Spec §9.3 — replay protection. Repository must surface a typed
		// sentinel error so the API layer can map it to HTTP 409 instead
		// of letting the raw UNIQUE constraint bubble up as a 500.
		assert.ErrorIs(t, err, domain.ErrDuplicateMessageID)
	})

	t.Run("sets CreatedAt if not provided", func(t *testing.T) {
		before := time.Now()
		msg := &domain.Message{
			MessageID: "msg-auto-timestamp",
			Direction: domain.DirectionOutbound,
			Status:    domain.StatusQueued,
			Payload:   "test",
		}

		err := repo.Create(ctx, msg)

		require.NoError(t, err)
		assert.False(t, msg.CreatedAt.IsZero())
		assert.True(t, msg.CreatedAt.After(before) || msg.CreatedAt.Equal(before))
	})

	t.Run("respects custom CreatedAt", func(t *testing.T) {
		customTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		msg := &domain.Message{
			MessageID: "msg-custom-time",
			Direction: domain.DirectionOutbound,
			Status:    domain.StatusQueued,
			Payload:   "test",
			CreatedAt: customTime,
		}

		err := repo.Create(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, customTime.Unix(), msg.CreatedAt.Unix())
	})
}

func TestMessageRepository_GetByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	// Create a test message
	msg := &domain.Message{
		MessageID:  "msg-get-test",
		Direction:  domain.DirectionInbound,
		Status:     domain.StatusDelivered,
		Payload:    "test payload",
		RetryCount: 2,
		LastError:  "some error",
	}
	require.NoError(t, repo.Create(ctx, msg))

	t.Run("retrieves existing message", func(t *testing.T) {
		retrieved, err := repo.GetByID(ctx, "msg-get-test")

		require.NoError(t, err)
		assert.Equal(t, msg.ID, retrieved.ID)
		assert.Equal(t, "msg-get-test", retrieved.MessageID)
		assert.Equal(t, domain.DirectionInbound, retrieved.Direction)
		assert.Equal(t, domain.StatusDelivered, retrieved.Status)
		assert.Equal(t, "test payload", retrieved.Payload)
		assert.Equal(t, 2, retrieved.RetryCount)
		assert.Equal(t, "some error", retrieved.LastError)
	})

	t.Run("returns error for non-existent message", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "non-existent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message not found")
	})

	t.Run("returns error for empty message_id", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message_id cannot be empty")
	})
}

func TestMessageRepository_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	// Create a test message
	msg := &domain.Message{
		MessageID: "msg-update-status",
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   "test",
	}
	require.NoError(t, repo.Create(ctx, msg))

	t.Run("updates status successfully", func(t *testing.T) {
		err := repo.UpdateStatus(ctx, "msg-update-status", domain.StatusSent)

		require.NoError(t, err)

		// Verify the update
		updated, err := repo.GetByID(ctx, "msg-update-status")
		require.NoError(t, err)
		assert.Equal(t, domain.StatusSent, updated.Status)
	})

	t.Run("fails for non-existent message", func(t *testing.T) {
		err := repo.UpdateStatus(ctx, "non-existent", domain.StatusFailed)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message not found")
	})

	t.Run("fails with empty message_id", func(t *testing.T) {
		err := repo.UpdateStatus(ctx, "", domain.StatusFailed)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message_id cannot be empty")
	})
}

func TestMessageRepository_UpdateRetryInfo(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	// Create a test message
	msg := &domain.Message{
		MessageID: "msg-retry-info",
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   "test",
	}
	require.NoError(t, repo.Create(ctx, msg))

	t.Run("updates retry info successfully", func(t *testing.T) {
		nextRetry := time.Now().Add(5 * time.Minute)
		err := repo.UpdateRetryInfo(ctx, "msg-retry-info", 3, &nextRetry, "connection timeout")

		require.NoError(t, err)

		// Verify the update
		updated, err := repo.GetByID(ctx, "msg-retry-info")
		require.NoError(t, err)
		assert.Equal(t, 3, updated.RetryCount)
		assert.NotNil(t, updated.NextRetryAt)
		assert.Equal(t, "connection timeout", updated.LastError)
	})

	t.Run("updates with nil next_retry_at", func(t *testing.T) {
		err := repo.UpdateRetryInfo(ctx, "msg-retry-info", 0, nil, "")

		require.NoError(t, err)

		updated, err := repo.GetByID(ctx, "msg-retry-info")
		require.NoError(t, err)
		assert.Equal(t, 0, updated.RetryCount)
		assert.Nil(t, updated.NextRetryAt)
		assert.Equal(t, "", updated.LastError)
	})

	t.Run("fails with empty message_id", func(t *testing.T) {
		err := repo.UpdateRetryInfo(ctx, "", 1, nil, "error")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message_id cannot be empty")
	})
}

func TestMessageRepository_ListByStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	// Create test messages with different statuses
	messages := []*domain.Message{
		{MessageID: "msg-queued-1", Status: domain.StatusQueued, Direction: domain.DirectionOutbound, Payload: "test1"},
		{MessageID: "msg-queued-2", Status: domain.StatusQueued, Direction: domain.DirectionOutbound, Payload: "test2"},
		{MessageID: "msg-sent-1", Status: domain.StatusSent, Direction: domain.DirectionOutbound, Payload: "test3"},
		{MessageID: "msg-failed-1", Status: domain.StatusFailed, Direction: domain.DirectionOutbound, Payload: "test4"},
	}

	for _, msg := range messages {
		require.NoError(t, repo.Create(ctx, msg))
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	t.Run("lists messages by status", func(t *testing.T) {
		queued, err := repo.ListByStatus(ctx, domain.StatusQueued)

		require.NoError(t, err)
		assert.Len(t, queued, 2)
		assert.Equal(t, "msg-queued-1", queued[0].MessageID)
		assert.Equal(t, "msg-queued-2", queued[1].MessageID)
	})

	t.Run("returns empty list for status with no messages", func(t *testing.T) {
		delivered, err := repo.ListByStatus(ctx, domain.StatusDelivered)

		require.NoError(t, err)
		assert.Empty(t, delivered)
	})

	t.Run("orders by created_at ascending", func(t *testing.T) {
		queued, err := repo.ListByStatus(ctx, domain.StatusQueued)

		require.NoError(t, err)
		require.Len(t, queued, 2)
		assert.True(t, queued[0].CreatedAt.Before(queued[1].CreatedAt) ||
			queued[0].CreatedAt.Equal(queued[1].CreatedAt))
	})
}

func TestMessageRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	// Create a test message
	msg := &domain.Message{
		MessageID: "msg-delete-test",
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   "test",
	}
	require.NoError(t, repo.Create(ctx, msg))

	t.Run("deletes existing message", func(t *testing.T) {
		err := repo.Delete(ctx, "msg-delete-test")

		require.NoError(t, err)

		// Verify deletion
		_, err = repo.GetByID(ctx, "msg-delete-test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message not found")
	})

	t.Run("fails for non-existent message", func(t *testing.T) {
		err := repo.Delete(ctx, "non-existent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message not found")
	})

	t.Run("fails with empty message_id", func(t *testing.T) {
		err := repo.Delete(ctx, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "message_id cannot be empty")
	})
}

func TestMessageRepository_ListPaginated(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()

	base := time.Now()
	// Seed: 3 queued (newest first by created_at), 2 delivered
	seeded := []*domain.Message{
		{MessageID: "q1", Direction: domain.DirectionOutbound, Status: domain.StatusQueued, Payload: "p", CreatedAt: base.Add(-5 * time.Minute)},
		{MessageID: "q2", Direction: domain.DirectionOutbound, Status: domain.StatusQueued, Payload: "p", CreatedAt: base.Add(-3 * time.Minute)},
		{MessageID: "q3", Direction: domain.DirectionOutbound, Status: domain.StatusQueued, Payload: "p", CreatedAt: base.Add(-1 * time.Minute)},
		{MessageID: "d1", Direction: domain.DirectionInbound, Status: domain.StatusDelivered, Payload: "p", CreatedAt: base.Add(-4 * time.Minute)},
		{MessageID: "d2", Direction: domain.DirectionInbound, Status: domain.StatusDelivered, Payload: "p", CreatedAt: base.Add(-2 * time.Minute)},
	}
	for _, m := range seeded {
		require.NoError(t, repo.Create(ctx, m))
	}

	t.Run("no filter returns all ordered by created_at DESC", func(t *testing.T) {
		msgs, total, err := repo.ListPaginated(ctx, nil, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		require.Len(t, msgs, 5)
		assert.Equal(t, "q3", msgs[0].MessageID)
		assert.Equal(t, "d2", msgs[1].MessageID)
		assert.Equal(t, "q2", msgs[2].MessageID)
	})

	t.Run("status filter restricts results", func(t *testing.T) {
		queued := domain.StatusQueued
		msgs, total, err := repo.ListPaginated(ctx, &queued, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
		require.Len(t, msgs, 3)
		for _, m := range msgs {
			assert.Equal(t, domain.StatusQueued, m.Status)
		}
	})

	t.Run("limit + offset paginate", func(t *testing.T) {
		msgs, total, err := repo.ListPaginated(ctx, nil, 2, 2)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		require.Len(t, msgs, 2)
		assert.Equal(t, "q2", msgs[0].MessageID)
		assert.Equal(t, "d1", msgs[1].MessageID)
	})

	t.Run("empty result returns []*Message, not nil", func(t *testing.T) {
		failed := domain.StatusFailed
		msgs, total, err := repo.ListPaginated(ctx, &failed, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.NotNil(t, msgs)
		assert.Len(t, msgs, 0)
	})
}

func TestMessageRepository_CountByStatusSince(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)
	ctx := context.Background()
	now := time.Now()

	seeded := []*domain.Message{
		{MessageID: "old-q", Status: domain.StatusQueued, Direction: domain.DirectionOutbound, Payload: "p", CreatedAt: now.Add(-48 * time.Hour)},
		{MessageID: "recent-q1", Status: domain.StatusQueued, Direction: domain.DirectionOutbound, Payload: "p", CreatedAt: now.Add(-1 * time.Hour)},
		{MessageID: "recent-q2", Status: domain.StatusQueued, Direction: domain.DirectionOutbound, Payload: "p", CreatedAt: now.Add(-30 * time.Minute)},
		{MessageID: "recent-d", Status: domain.StatusDelivered, Direction: domain.DirectionInbound, Payload: "p", CreatedAt: now.Add(-2 * time.Hour)},
		{MessageID: "recent-f", Status: domain.StatusFailed, Direction: domain.DirectionOutbound, Payload: "p", CreatedAt: now.Add(-15 * time.Minute)},
	}
	for _, m := range seeded {
		require.NoError(t, repo.Create(ctx, m))
	}

	counts, err := repo.CountByStatusSince(ctx, now.Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 2, counts[domain.StatusQueued])
	assert.Equal(t, 1, counts[domain.StatusDelivered])
	assert.Equal(t, 1, counts[domain.StatusFailed])
	_, hasOld := counts[domain.StatusQueued]
	assert.True(t, hasOld) // present
}

func TestMessageRepository_ContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSQLiteMessageRepository(db)

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		msg := &domain.Message{
			MessageID: "msg-cancelled",
			Direction: domain.DirectionOutbound,
			Status:    domain.StatusQueued,
			Payload:   "test",
		}

		err := repo.Create(ctx, msg)

		// Should fail due to cancelled context
		assert.Error(t, err)
	})
}
