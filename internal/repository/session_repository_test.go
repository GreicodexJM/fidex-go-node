package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"fidex-node/internal/domain"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSessionTestDB creates an in-memory SQLite database for session testing
func setupSessionTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create sessions table schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	return db
}

func TestSessionRepository_Create(t *testing.T) {
	db := setupSessionTestDB(t)
	defer db.Close()

	repo := NewSQLiteSessionRepository(db)
	ctx := context.Background()

	t.Run("creates session successfully", func(t *testing.T) {
		session := &domain.Session{
			SessionID: "session-123",
			UserID:    1,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		err := repo.Create(ctx, session)

		require.NoError(t, err)
		assert.False(t, session.CreatedAt.IsZero())
	})

	t.Run("fails with nil session", func(t *testing.T) {
		err := repo.Create(ctx, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("fails with empty session_id", func(t *testing.T) {
		session := &domain.Session{
			SessionID: "",
			UserID:    1,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		err := repo.Create(ctx, session)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session_id cannot be empty")
	})

	t.Run("sets created_at automatically", func(t *testing.T) {
		before := time.Now()
		session := &domain.Session{
			SessionID: "session-auto-time",
			UserID:    1,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		err := repo.Create(ctx, session)

		require.NoError(t, err)
		assert.True(t, session.CreatedAt.After(before) || session.CreatedAt.Equal(before))
	})

	t.Run("respects custom created_at", func(t *testing.T) {
		customTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		session := &domain.Session{
			SessionID: "session-custom-time",
			UserID:    1,
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: customTime,
		}

		err := repo.Create(ctx, session)

		require.NoError(t, err)
		assert.Equal(t, customTime.Unix(), session.CreatedAt.Unix())
	})

	t.Run("fails with duplicate session_id", func(t *testing.T) {
		session1 := &domain.Session{
			SessionID: "duplicate-session",
			UserID:    1,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		err := repo.Create(ctx, session1)
		require.NoError(t, err)

		session2 := &domain.Session{
			SessionID: "duplicate-session",
			UserID:    2,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		err = repo.Create(ctx, session2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create session")
	})
}

func TestSessionRepository_GetByID(t *testing.T) {
	db := setupSessionTestDB(t)
	defer db.Close()

	repo := NewSQLiteSessionRepository(db)
	ctx := context.Background()

	// Create a test session
	expiresAt := time.Now().Add(24 * time.Hour)
	session := &domain.Session{
		SessionID: "get-test-session",
		UserID:    42,
		ExpiresAt: expiresAt,
	}
	require.NoError(t, repo.Create(ctx, session))

	t.Run("retrieves existing session", func(t *testing.T) {
		retrieved, err := repo.GetByID(ctx, "get-test-session")

		require.NoError(t, err)
		assert.Equal(t, "get-test-session", retrieved.SessionID)
		assert.Equal(t, int64(42), retrieved.UserID)
		assert.WithinDuration(t, expiresAt, retrieved.ExpiresAt, time.Second)
		assert.False(t, retrieved.CreatedAt.IsZero())
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "non-existent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")
	})

	t.Run("fails with empty session_id", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session_id cannot be empty")
	})
}

func TestSessionRepository_Delete(t *testing.T) {
	db := setupSessionTestDB(t)
	defer db.Close()

	repo := NewSQLiteSessionRepository(db)
	ctx := context.Background()

	// Create a test session
	session := &domain.Session{
		SessionID: "delete-test-session",
		UserID:    1,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, repo.Create(ctx, session))

	t.Run("deletes existing session", func(t *testing.T) {
		err := repo.Delete(ctx, "delete-test-session")

		require.NoError(t, err)

		// Verify deletion
		_, err = repo.GetByID(ctx, "delete-test-session")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")
	})

	t.Run("fails for non-existent session", func(t *testing.T) {
		err := repo.Delete(ctx, "non-existent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")
	})

	t.Run("fails with empty session_id", func(t *testing.T) {
		err := repo.Delete(ctx, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session_id cannot be empty")
	})
}

func TestSessionRepository_DeleteExpired(t *testing.T) {
	db := setupSessionTestDB(t)
	defer db.Close()

	repo := NewSQLiteSessionRepository(db)
	ctx := context.Background()

	t.Run("deletes only expired sessions", func(t *testing.T) {
		// Create an expired session
		expiredSession := &domain.Session{
			SessionID: "expired-session",
			UserID:    1,
			ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		}
		require.NoError(t, repo.Create(ctx, expiredSession))

		// Create a valid session
		validSession := &domain.Session{
			SessionID: "valid-session",
			UserID:    2,
			ExpiresAt: time.Now().Add(24 * time.Hour), // Expires in 24 hours
		}
		require.NoError(t, repo.Create(ctx, validSession))

		// Delete expired sessions
		err := repo.DeleteExpired(ctx)
		require.NoError(t, err)

		// Verify expired session is deleted
		_, err = repo.GetByID(ctx, "expired-session")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")

		// Verify valid session still exists
		retrieved, err := repo.GetByID(ctx, "valid-session")
		require.NoError(t, err)
		assert.Equal(t, "valid-session", retrieved.SessionID)
	})

	t.Run("succeeds when no expired sessions exist", func(t *testing.T) {
		err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
	})

	t.Run("deletes multiple expired sessions", func(t *testing.T) {
		// Create multiple expired sessions
		for i := 1; i <= 3; i++ {
			session := &domain.Session{
				SessionID: fmt.Sprintf("expired-%d", i),
				UserID:    int64(i),
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			}
			require.NoError(t, repo.Create(ctx, session))
		}

		// Delete all expired
		err := repo.DeleteExpired(ctx)
		require.NoError(t, err)

		// Verify all are deleted
		for i := 1; i <= 3; i++ {
			_, err := repo.GetByID(ctx, fmt.Sprintf("expired-%d", i))
			assert.Error(t, err)
		}
	})
}

func TestSessionRepository_CompleteWorkflow(t *testing.T) {
	db := setupSessionTestDB(t)
	defer db.Close()

	repo := NewSQLiteSessionRepository(db)
	ctx := context.Background()

	// Create
	session := &domain.Session{
		SessionID: "workflow-session",
		UserID:    100,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err := repo.Create(ctx, session)
	require.NoError(t, err)

	// Read
	retrieved, err := repo.GetByID(ctx, "workflow-session")
	require.NoError(t, err)
	assert.Equal(t, int64(100), retrieved.UserID)

	// Delete
	err = repo.Delete(ctx, "workflow-session")
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.GetByID(ctx, "workflow-session")
	assert.Error(t, err)
}

func TestSessionRepository_ExpirationEdgeCases(t *testing.T) {
	db := setupSessionTestDB(t)
	defer db.Close()

	repo := NewSQLiteSessionRepository(db)
	ctx := context.Background()

	t.Run("session expiring exactly now", func(t *testing.T) {
		session := &domain.Session{
			SessionID: "expire-now",
			UserID:    1,
			ExpiresAt: time.Now(), // Expires right now
		}
		require.NoError(t, repo.Create(ctx, session))

		// Sleep briefly to ensure time has passed
		time.Sleep(10 * time.Millisecond)

		err := repo.DeleteExpired(ctx)
		require.NoError(t, err)

		// Should be deleted
		_, err = repo.GetByID(ctx, "expire-now")
		assert.Error(t, err)
	})

	t.Run("session expiring in far future", func(t *testing.T) {
		session := &domain.Session{
			SessionID: "future-session",
			UserID:    1,
			ExpiresAt: time.Now().Add(365 * 24 * time.Hour), // 1 year from now
		}
		require.NoError(t, repo.Create(ctx, session))

		err := repo.DeleteExpired(ctx)
		require.NoError(t, err)

		// Should still exist
		retrieved, err := repo.GetByID(ctx, "future-session")
		require.NoError(t, err)
		assert.Equal(t, "future-session", retrieved.SessionID)
	})
}
