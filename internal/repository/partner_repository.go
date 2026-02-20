package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"fidex-node/internal/domain"
)

// SQLitePartnerRepository implements domain.PartnerRepository for SQLite
type SQLitePartnerRepository struct {
	db *sql.DB
}

// NewSQLitePartnerRepository creates a new SQLite partner repository
func NewSQLitePartnerRepository(db *sql.DB) *SQLitePartnerRepository {
	return &SQLitePartnerRepository{db: db}
}

// Create inserts a new trading partner into the database
func (r *SQLitePartnerRepository) Create(ctx context.Context, partner *domain.Partner) error {
	if partner == nil {
		return fmt.Errorf("partner cannot be nil")
	}

	if partner.PartnerID == "" {
		return fmt.Errorf("partner_id cannot be empty")
	}

	if partner.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if partner.JWKSUrl == "" {
		return fmt.Errorf("jwks_url cannot be empty")
	}

	now := time.Now()
	if partner.CreatedAt.IsZero() {
		partner.CreatedAt = now
	}
	if partner.UpdatedAt.IsZero() {
		partner.UpdatedAt = now
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO trading_partners (partner_id, name, jwks_url, message_endpoint, mdn_receipt_endpoint, public_key_jwks, last_key_refresh, created_at, updated_at) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		partner.PartnerID,
		partner.Name,
		partner.JWKSUrl,
		partner.MessageEndpoint,
		partner.MDNReceiptEndpoint,
		partner.PublicKeyJWKS,
		partner.LastKeyRefresh,
		partner.CreatedAt,
		partner.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert partner: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	partner.ID = id
	return nil
}

// GetByID retrieves a trading partner by their partner_id
func (r *SQLitePartnerRepository) GetByID(ctx context.Context, partnerID string) (*domain.Partner, error) {
	if partnerID == "" {
		return nil, fmt.Errorf("partner_id cannot be empty")
	}

	var partner domain.Partner
	err := r.db.QueryRowContext(ctx,
		`SELECT id, partner_id, name, jwks_url, message_endpoint, mdn_receipt_endpoint, public_key_jwks, last_key_refresh, created_at, updated_at 
		 FROM trading_partners WHERE partner_id = ?`,
		partnerID,
	).Scan(
		&partner.ID,
		&partner.PartnerID,
		&partner.Name,
		&partner.JWKSUrl,
		&partner.MessageEndpoint,
		&partner.MDNReceiptEndpoint,
		&partner.PublicKeyJWKS,
		&partner.LastKeyRefresh,
		&partner.CreatedAt,
		&partner.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("partner not found: %s", partnerID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get partner: %w", err)
	}

	return &partner, nil
}

// Update updates an existing trading partner
func (r *SQLitePartnerRepository) Update(ctx context.Context, partner *domain.Partner) error {
	if partner == nil {
		return fmt.Errorf("partner cannot be nil")
	}

	if partner.PartnerID == "" {
		return fmt.Errorf("partner_id cannot be empty")
	}

	partner.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx,
		`UPDATE trading_partners SET name = ?, jwks_url = ?, message_endpoint = ?, mdn_receipt_endpoint = ?, public_key_jwks = ?, last_key_refresh = ?, updated_at = ? 
		 WHERE partner_id = ?`,
		partner.Name,
		partner.JWKSUrl,
		partner.MessageEndpoint,
		partner.MDNReceiptEndpoint,
		partner.PublicKeyJWKS,
		partner.LastKeyRefresh,
		partner.UpdatedAt,
		partner.PartnerID,
	)
	if err != nil {
		return fmt.Errorf("failed to update partner: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("partner not found: %s", partner.PartnerID)
	}

	return nil
}

// Delete removes a trading partner by their partner_id
func (r *SQLitePartnerRepository) Delete(ctx context.Context, partnerID string) error {
	if partnerID == "" {
		return fmt.Errorf("partner_id cannot be empty")
	}

	result, err := r.db.ExecContext(ctx, `DELETE FROM trading_partners WHERE partner_id = ?`, partnerID)
	if err != nil {
		return fmt.Errorf("failed to delete partner: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("partner not found: %s", partnerID)
	}

	return nil
}

// List retrieves all trading partners
func (r *SQLitePartnerRepository) List(ctx context.Context) ([]*domain.Partner, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, partner_id, name, jwks_url, message_endpoint, mdn_receipt_endpoint, public_key_jwks, last_key_refresh, created_at, updated_at 
		 FROM trading_partners ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query partners: %w", err)
	}
	defer rows.Close()

	var partners []*domain.Partner
	for rows.Next() {
		var partner domain.Partner
		if err := rows.Scan(
			&partner.ID,
			&partner.PartnerID,
			&partner.Name,
			&partner.JWKSUrl,
			&partner.MessageEndpoint,
			&partner.MDNReceiptEndpoint,
			&partner.PublicKeyJWKS,
			&partner.LastKeyRefresh,
			&partner.CreatedAt,
			&partner.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan partner: %w", err)
		}
		partners = append(partners, &partner)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating partners: %w", err)
	}

	return partners, nil
}
