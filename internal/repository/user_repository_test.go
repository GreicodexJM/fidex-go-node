package repository

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupUserTestDB creates an in-memory SQLite database for user testing
func setupUserTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create users table schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	return db
}

func TestUserRepository_Create(t *testing.T) {
	db := setupUserTestDB(t)
	defer db.Close()

	repo := NewSQLiteUserRepository(db)
	ctx := context.Background()

	t.Run("creates user successfully", func(t *testing.T) {
		user, err := repo.Create(ctx, "admin", "$2a$10$hashedpassword")

		require.NoError(t, err)
		assert.NotZero(t, user.ID)
		assert.Equal(t, "admin", user.Username)
		assert.Equal(t, "$2a$10$hashedpassword", user.PasswordHash)
		assert.False(t, user.CreatedAt.IsZero())
	})

	t.Run("fails with empty username", func(t *testing.T) {
		_, err := repo.Create(ctx, "", "password")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username cannot be empty")
	})

	t.Run("fails with empty password hash", func(t *testing.T) {
		_, err := repo.Create(ctx, "testuser", "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "password hash cannot be empty")
	})

	t.Run("fails with duplicate username", func(t *testing.T) {
		_, err := repo.Create(ctx, "duplicate", "hash1")
		require.NoError(t, err)

		_, err = repo.Create(ctx, "duplicate", "hash2")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create user")
	})
}

func TestUserRepository_GetByID(t *testing.T) {
	db := setupUserTestDB(t)
	defer db.Close()

	repo := NewSQLiteUserRepository(db)
	ctx := context.Background()

	// Create a test user
	created, err := repo.Create(ctx, "testuser", "$2a$10$hash")
	require.NoError(t, err)

	t.Run("retrieves existing user", func(t *testing.T) {
		user, err := repo.GetByID(ctx, created.ID)

		require.NoError(t, err)
		assert.Equal(t, created.ID, user.ID)
		assert.Equal(t, "testuser", user.Username)
		assert.Equal(t, "$2a$10$hash", user.PasswordHash)
		assert.False(t, user.CreatedAt.IsZero())
	})

	t.Run("returns error for non-existent user", func(t *testing.T) {
		_, err := repo.GetByID(ctx, 99999)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
	})
}

func TestUserRepository_GetByUsername(t *testing.T) {
	db := setupUserTestDB(t)
	defer db.Close()

	repo := NewSQLiteUserRepository(db)
	ctx := context.Background()

	// Create a test user
	_, err := repo.Create(ctx, "findme", "$2a$10$findhash")
	require.NoError(t, err)

	t.Run("retrieves existing user by username", func(t *testing.T) {
		user, err := repo.GetByUsername(ctx, "findme")

		require.NoError(t, err)
		assert.Equal(t, "findme", user.Username)
		assert.Equal(t, "$2a$10$findhash", user.PasswordHash)
	})

	t.Run("returns error for non-existent username", func(t *testing.T) {
		_, err := repo.GetByUsername(ctx, "nonexistent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
	})

	t.Run("fails with empty username", func(t *testing.T) {
		_, err := repo.GetByUsername(ctx, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username cannot be empty")
	})
}

func TestUserRepository_List(t *testing.T) {
	db := setupUserTestDB(t)
	defer db.Close()

	repo := NewSQLiteUserRepository(db)
	ctx := context.Background()

	t.Run("returns empty list when no users exist", func(t *testing.T) {
		users, err := repo.List(ctx)

		require.NoError(t, err)
		assert.Empty(t, users)
	})

	// Create test users
	_, err := repo.Create(ctx, "user1", "hash1")
	require.NoError(t, err)
	_, err = repo.Create(ctx, "user2", "hash2")
	require.NoError(t, err)
	_, err = repo.Create(ctx, "user3", "hash3")
	require.NoError(t, err)

	t.Run("lists all users", func(t *testing.T) {
		users, err := repo.List(ctx)

		require.NoError(t, err)
		assert.Len(t, users, 3)
	})

	t.Run("orders by created_at descending", func(t *testing.T) {
		users, err := repo.List(ctx)

		require.NoError(t, err)
		require.Len(t, users, 3)

		// Newest first
		assert.Equal(t, "user3", users[0].Username)
		assert.Equal(t, "user2", users[1].Username)
		assert.Equal(t, "user1", users[2].Username)
	})
}

func TestUserRepository_Delete(t *testing.T) {
	db := setupUserTestDB(t)
	defer db.Close()

	repo := NewSQLiteUserRepository(db)
	ctx := context.Background()

	// Create a test user
	user, err := repo.Create(ctx, "deleteMe", "hash")
	require.NoError(t, err)

	t.Run("deletes existing user", func(t *testing.T) {
		err := repo.Delete(ctx, user.ID)

		require.NoError(t, err)

		// Verify deletion
		_, err = repo.GetByID(ctx, user.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
	})

	t.Run("fails for non-existent user", func(t *testing.T) {
		err := repo.Delete(ctx, 99999)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
	})
}

func TestUserRepository_UpdatePassword(t *testing.T) {
	db := setupUserTestDB(t)
	defer db.Close()

	repo := NewSQLiteUserRepository(db)
	ctx := context.Background()

	// Create a test user
	user, err := repo.Create(ctx, "changepass", "oldhash")
	require.NoError(t, err)

	t.Run("updates password successfully", func(t *testing.T) {
		err := repo.UpdatePassword(ctx, user.ID, "newhash")

		require.NoError(t, err)

		// Verify the update
		updated, err := repo.GetByID(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "newhash", updated.PasswordHash)
	})

	t.Run("fails with empty password hash", func(t *testing.T) {
		err := repo.UpdatePassword(ctx, user.ID, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "password hash cannot be empty")
	})

	t.Run("fails for non-existent user", func(t *testing.T) {
		err := repo.UpdatePassword(ctx, 99999, "newhash")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
	})
}

func TestUserRepository_CompleteWorkflow(t *testing.T) {
	db := setupUserTestDB(t)
	defer db.Close()

	repo := NewSQLiteUserRepository(db)
	ctx := context.Background()

	// Create
	user, err := repo.Create(ctx, "workflow", "initialhash")
	require.NoError(t, err)
	assert.NotZero(t, user.ID)

	// Get by ID
	retrieved, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "workflow", retrieved.Username)

	// Get by Username
	byName, err := repo.GetByUsername(ctx, "workflow")
	require.NoError(t, err)
	assert.Equal(t, user.ID, byName.ID)

	// Update Password
	err = repo.UpdatePassword(ctx, user.ID, "updatedhash")
	require.NoError(t, err)

	updated, err := repo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "updatedhash", updated.PasswordHash)

	// List
	users, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, users, 1)

	// Delete
	err = repo.Delete(ctx, user.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.GetByID(ctx, user.ID)
	assert.Error(t, err)
}
