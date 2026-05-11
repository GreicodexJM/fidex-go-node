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

// setupPartnerTestDB creates an in-memory SQLite database for partner testing
func setupPartnerTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Create trading_partners table schema
	_, err = db.Exec(`
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
		)
	`)
	require.NoError(t, err)

	return db
}

func TestPartnerRepository_Create(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	t.Run("creates partner successfully", func(t *testing.T) {
		partner := &domain.Partner{
			PartnerID:          "partner-123",
			Name:               "Test Partner",
			JWKSUrl:            "https://partner.example.com/.well-known/jwks.json",
			MessageEndpoint:    "https://partner.example.com/messages",
			MDNReceiptEndpoint: "https://partner.example.com/mdn",
			PublicKeyJWKS:      `{"keys":[]}`,
		}

		err := repo.Create(ctx, partner)

		require.NoError(t, err)
		assert.NotZero(t, partner.ID, "ID should be set after creation")
		assert.False(t, partner.CreatedAt.IsZero(), "CreatedAt should be set")
		assert.False(t, partner.UpdatedAt.IsZero(), "UpdatedAt should be set")
	})

	t.Run("fails with nil partner", func(t *testing.T) {
		err := repo.Create(ctx, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("fails with empty partner_id", func(t *testing.T) {
		partner := &domain.Partner{
			Name:    "Test",
			JWKSUrl: "https://example.com/jwks",
		}

		err := repo.Create(ctx, partner)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partner_id cannot be empty")
	})

	t.Run("fails with empty name", func(t *testing.T) {
		partner := &domain.Partner{
			PartnerID: "partner-no-name",
			JWKSUrl:   "https://example.com/jwks",
		}

		err := repo.Create(ctx, partner)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name cannot be empty")
	})

	t.Run("fails with empty jwks_url", func(t *testing.T) {
		partner := &domain.Partner{
			PartnerID: "partner-no-jwks",
			Name:      "Test Partner",
		}

		err := repo.Create(ctx, partner)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "jwks_url cannot be empty")
	})

	t.Run("fails with duplicate partner_id", func(t *testing.T) {
		partner1 := &domain.Partner{
			PartnerID: "partner-duplicate",
			Name:      "Partner 1",
			JWKSUrl:   "https://example.com/jwks",
		}

		err := repo.Create(ctx, partner1)
		require.NoError(t, err)

		// Try to create another partner with the same ID
		partner2 := &domain.Partner{
			PartnerID: "partner-duplicate",
			Name:      "Partner 2",
			JWKSUrl:   "https://example2.com/jwks",
		}

		err = repo.Create(ctx, partner2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to insert partner")
	})

	t.Run("sets timestamps automatically", func(t *testing.T) {
		before := time.Now()
		partner := &domain.Partner{
			PartnerID: "partner-auto-time",
			Name:      "Test Partner",
			JWKSUrl:   "https://example.com/jwks",
		}

		err := repo.Create(ctx, partner)

		require.NoError(t, err)
		assert.True(t, partner.CreatedAt.After(before) || partner.CreatedAt.Equal(before))
		assert.True(t, partner.UpdatedAt.After(before) || partner.UpdatedAt.Equal(before))
	})

	t.Run("respects custom timestamps", func(t *testing.T) {
		customTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		partner := &domain.Partner{
			PartnerID: "partner-custom-time",
			Name:      "Test Partner",
			JWKSUrl:   "https://example.com/jwks",
			CreatedAt: customTime,
			UpdatedAt: customTime,
		}

		err := repo.Create(ctx, partner)

		require.NoError(t, err)
		assert.Equal(t, customTime.Unix(), partner.CreatedAt.Unix())
		assert.Equal(t, customTime.Unix(), partner.UpdatedAt.Unix())
	})

	t.Run("creates partner with optional fields", func(t *testing.T) {
		lastRefresh := time.Now().Add(-1 * time.Hour)
		partner := &domain.Partner{
			PartnerID:          "partner-full",
			Name:               "Full Partner",
			JWKSUrl:            "https://example.com/jwks",
			MessageEndpoint:    "https://example.com/messages",
			MDNReceiptEndpoint: "https://example.com/mdn",
			PublicKeyJWKS:      `{"keys":[{"kty":"RSA"}]}`,
			LastKeyRefresh:     &lastRefresh,
		}

		err := repo.Create(ctx, partner)

		require.NoError(t, err)
		assert.NotZero(t, partner.ID)
	})
}

func TestPartnerRepository_GetByID(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	// Create a test partner
	lastRefresh := time.Now().Add(-2 * time.Hour)
	partner := &domain.Partner{
		PartnerID:          "partner-get-test",
		Name:               "Get Test Partner",
		JWKSUrl:            "https://example.com/jwks",
		MessageEndpoint:    "https://example.com/messages",
		MDNReceiptEndpoint: "https://example.com/mdn",
		PublicKeyJWKS:      `{"keys":[]}`,
		LastKeyRefresh:     &lastRefresh,
	}
	require.NoError(t, repo.Create(ctx, partner))

	t.Run("retrieves existing partner", func(t *testing.T) {
		retrieved, err := repo.GetByID(ctx, "partner-get-test")

		require.NoError(t, err)
		assert.Equal(t, partner.ID, retrieved.ID)
		assert.Equal(t, "partner-get-test", retrieved.PartnerID)
		assert.Equal(t, "Get Test Partner", retrieved.Name)
		assert.Equal(t, "https://example.com/jwks", retrieved.JWKSUrl)
		assert.Equal(t, "https://example.com/messages", retrieved.MessageEndpoint)
		assert.Equal(t, "https://example.com/mdn", retrieved.MDNReceiptEndpoint)
		assert.Equal(t, `{"keys":[]}`, retrieved.PublicKeyJWKS)
		assert.NotNil(t, retrieved.LastKeyRefresh)
	})

	t.Run("returns error for non-existent partner", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "non-existent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partner not found")
	})

	t.Run("returns error for empty partner_id", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partner_id cannot be empty")
	})
}

func TestPartnerRepository_Update(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	// Create a test partner
	partner := &domain.Partner{
		PartnerID: "partner-update-test",
		Name:      "Original Name",
		JWKSUrl:   "https://example.com/jwks",
	}
	require.NoError(t, repo.Create(ctx, partner))

	t.Run("updates partner successfully", func(t *testing.T) {
		partner.Name = "Updated Name"
		partner.MessageEndpoint = "https://new-endpoint.com/messages"
		partner.PublicKeyJWKS = `{"keys":[{"kty":"RSA"}]}`

		beforeUpdate := time.Now()
		err := repo.Update(ctx, partner)

		require.NoError(t, err)
		assert.True(t, partner.UpdatedAt.After(beforeUpdate) || partner.UpdatedAt.Equal(beforeUpdate))

		// Verify the update
		updated, err := repo.GetByID(ctx, "partner-update-test")
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", updated.Name)
		assert.Equal(t, "https://new-endpoint.com/messages", updated.MessageEndpoint)
		assert.Equal(t, `{"keys":[{"kty":"RSA"}]}`, updated.PublicKeyJWKS)
	})

	t.Run("fails with nil partner", func(t *testing.T) {
		err := repo.Update(ctx, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("fails with empty partner_id", func(t *testing.T) {
		partner := &domain.Partner{
			Name:    "Test",
			JWKSUrl: "https://example.com/jwks",
		}

		err := repo.Update(ctx, partner)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partner_id cannot be empty")
	})

	t.Run("fails for non-existent partner", func(t *testing.T) {
		nonExistent := &domain.Partner{
			PartnerID: "non-existent",
			Name:      "Test",
			JWKSUrl:   "https://example.com/jwks",
		}

		err := repo.Update(ctx, nonExistent)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partner not found")
	})

	t.Run("updates last_key_refresh", func(t *testing.T) {
		newRefresh := time.Now()
		partner.LastKeyRefresh = &newRefresh

		err := repo.Update(ctx, partner)

		require.NoError(t, err)

		updated, err := repo.GetByID(ctx, "partner-update-test")
		require.NoError(t, err)
		assert.NotNil(t, updated.LastKeyRefresh)
	})
}

func TestPartnerRepository_Delete(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	// Create a test partner
	partner := &domain.Partner{
		PartnerID: "partner-delete-test",
		Name:      "Delete Test",
		JWKSUrl:   "https://example.com/jwks",
	}
	require.NoError(t, repo.Create(ctx, partner))

	t.Run("deletes existing partner", func(t *testing.T) {
		err := repo.Delete(ctx, "partner-delete-test")

		require.NoError(t, err)

		// Verify deletion
		_, err = repo.GetByID(ctx, "partner-delete-test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partner not found")
	})

	t.Run("fails for non-existent partner", func(t *testing.T) {
		err := repo.Delete(ctx, "non-existent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partner not found")
	})

	t.Run("fails with empty partner_id", func(t *testing.T) {
		err := repo.Delete(ctx, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partner_id cannot be empty")
	})
}

func TestPartnerRepository_List(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	t.Run("returns empty list when no partners exist", func(t *testing.T) {
		partners, err := repo.List(ctx)

		require.NoError(t, err)
		assert.Empty(t, partners)
	})

	// Create test partners
	partners := []*domain.Partner{
		{
			PartnerID: "partner-1",
			Name:      "Partner One",
			JWKSUrl:   "https://partner1.com/jwks",
		},
		{
			PartnerID: "partner-2",
			Name:      "Partner Two",
			JWKSUrl:   "https://partner2.com/jwks",
		},
		{
			PartnerID: "partner-3",
			Name:      "Partner Three",
			JWKSUrl:   "https://partner3.com/jwks",
		},
	}

	for _, p := range partners {
		require.NoError(t, repo.Create(ctx, p))
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	t.Run("lists all partners", func(t *testing.T) {
		list, err := repo.List(ctx)

		require.NoError(t, err)
		assert.Len(t, list, 3)
	})

	t.Run("orders by created_at descending", func(t *testing.T) {
		list, err := repo.List(ctx)

		require.NoError(t, err)
		require.Len(t, list, 3)

		// Should be in reverse order (newest first)
		assert.Equal(t, "partner-3", list[0].PartnerID)
		assert.Equal(t, "partner-2", list[1].PartnerID)
		assert.Equal(t, "partner-1", list[2].PartnerID)
	})

	t.Run("includes all partner fields", func(t *testing.T) {
		list, err := repo.List(ctx)

		require.NoError(t, err)
		require.NotEmpty(t, list)

		firstPartner := list[0]
		assert.NotZero(t, firstPartner.ID)
		assert.NotEmpty(t, firstPartner.PartnerID)
		assert.NotEmpty(t, firstPartner.Name)
		assert.NotEmpty(t, firstPartner.JWKSUrl)
		assert.False(t, firstPartner.CreatedAt.IsZero())
		assert.False(t, firstPartner.UpdatedAt.IsZero())
	})
}

func TestPartnerRepository_ContextCancellation(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)

	t.Run("respects context cancellation on create", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		partner := &domain.Partner{
			PartnerID: "partner-cancelled",
			Name:      "Test",
			JWKSUrl:   "https://example.com/jwks",
		}

		err := repo.Create(ctx, partner)

		// Should fail due to cancelled context
		assert.Error(t, err)
	})
}

func TestPartnerRepository_CompleteWorkflow(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	// Create
	partner := &domain.Partner{
		PartnerID:       "workflow-partner",
		Name:            "Workflow Test",
		JWKSUrl:         "https://example.com/jwks",
		MessageEndpoint: "https://example.com/messages",
	}

	err := repo.Create(ctx, partner)
	require.NoError(t, err)
	originalID := partner.ID

	// Read
	retrieved, err := repo.GetByID(ctx, "workflow-partner")
	require.NoError(t, err)
	assert.Equal(t, originalID, retrieved.ID)

	// Update
	retrieved.Name = "Updated Workflow Test"
	retrieved.MDNReceiptEndpoint = "https://example.com/mdn"
	err = repo.Update(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := repo.GetByID(ctx, "workflow-partner")
	require.NoError(t, err)
	assert.Equal(t, "Updated Workflow Test", updated.Name)
	assert.Equal(t, "https://example.com/mdn", updated.MDNReceiptEndpoint)

	// List
	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// Delete
	err = repo.Delete(ctx, "workflow-partner")
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.GetByID(ctx, "workflow-partner")
	assert.Error(t, err)
}

func TestPartnerRepository_Upsert(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	t.Run("inserts on first call", func(t *testing.T) {
		p := &domain.Partner{
			PartnerID: "upsert-1",
			Name:      "Original",
			JWKSUrl:   "https://example.com/jwks",
		}
		require.NoError(t, repo.Upsert(ctx, p))

		got, err := repo.GetByID(ctx, "upsert-1")
		require.NoError(t, err)
		assert.Equal(t, "Original", got.Name)
	})

	t.Run("updates on conflict", func(t *testing.T) {
		// Re-upsert with same partner_id, different fields
		p := &domain.Partner{
			PartnerID:     "upsert-1",
			Name:          "Renamed",
			JWKSUrl:       "https://example.com/jwks-v2",
			PublicKeyJWKS: "newjwks",
		}
		require.NoError(t, repo.Upsert(ctx, p))

		got, err := repo.GetByID(ctx, "upsert-1")
		require.NoError(t, err)
		assert.Equal(t, "Renamed", got.Name)
		assert.Equal(t, "https://example.com/jwks-v2", got.JWKSUrl)
		assert.Equal(t, "newjwks", got.PublicKeyJWKS)
	})

	t.Run("rejects empty partner_id", func(t *testing.T) {
		err := repo.Upsert(ctx, &domain.Partner{Name: "x", JWKSUrl: "y"})
		assert.Error(t, err)
	})
}

func TestPartnerRepository_DeleteByDBID(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	p := &domain.Partner{PartnerID: "del-1", Name: "X", JWKSUrl: "https://x"}
	require.NoError(t, repo.Create(ctx, p))

	t.Run("deletes existing row", func(t *testing.T) {
		require.NoError(t, repo.DeleteByDBID(ctx, p.ID))
		_, err := repo.GetByID(ctx, "del-1")
		assert.Error(t, err)
	})

	t.Run("returns error for unknown id", func(t *testing.T) {
		err := repo.DeleteByDBID(ctx, 99999)
		assert.Error(t, err)
	})
}

func TestPartnerRepository_UpdateNameByDBID(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	p := &domain.Partner{PartnerID: "ren-1", Name: "Before", JWKSUrl: "https://x"}
	require.NoError(t, repo.Create(ctx, p))

	t.Run("renames partner", func(t *testing.T) {
		require.NoError(t, repo.UpdateNameByDBID(ctx, p.ID, "After"))
		got, err := repo.GetByID(ctx, "ren-1")
		require.NoError(t, err)
		assert.Equal(t, "After", got.Name)
	})

	t.Run("rejects empty name", func(t *testing.T) {
		err := repo.UpdateNameByDBID(ctx, p.ID, "")
		assert.Error(t, err)
	})

	t.Run("errors when id absent", func(t *testing.T) {
		err := repo.UpdateNameByDBID(ctx, 99999, "Any")
		assert.Error(t, err)
	})
}

func TestPartnerRepository_Count(t *testing.T) {
	db := setupPartnerTestDB(t)
	defer db.Close()

	repo := NewSQLitePartnerRepository(db)
	ctx := context.Background()

	n, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(ctx, &domain.Partner{
			PartnerID: fmt.Sprintf("p-%d", i),
			Name:      "N",
			JWKSUrl:   "https://x",
		}))
	}

	n, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}
