package db

import (
	"database/sql"
	"fmt"
	"time"
)

// TradingPartner represents a partner connection profile
type TradingPartner struct {
	ID                 int64      `json:"id"`
	PartnerID          string     `json:"partner_id"`
	Name               string     `json:"name"`
	JWKSUrl            string     `json:"jwks_url"`
	MessageEndpoint    string     `json:"message_endpoint"`
	MDNReceiptEndpoint string     `json:"mdn_receipt_endpoint"`
	PublicKeyJWKS      string     `json:"public_key_jwks"`
	LastKeyRefresh     *time.Time `json:"last_key_refresh"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// CreatePartner creates a new trading partner in the database
func CreatePartner(partner *TradingPartner) error {
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

	result, err := DB.Exec(
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

// GetPartnerByID retrieves a trading partner by their partner_id
func GetPartnerByID(partnerID string) (*TradingPartner, error) {
	if partnerID == "" {
		return nil, fmt.Errorf("partner_id cannot be empty")
	}

	var partner TradingPartner
	err := DB.QueryRow(
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

// UpdatePartner updates an existing trading partner
func UpdatePartner(partner *TradingPartner) error {
	if partner == nil {
		return fmt.Errorf("partner cannot be nil")
	}

	if partner.PartnerID == "" {
		return fmt.Errorf("partner_id cannot be empty")
	}

	partner.UpdatedAt = time.Now()

	result, err := DB.Exec(
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

// DeletePartner deletes a trading partner by their partner_id
func DeletePartner(partnerID string) error {
	if partnerID == "" {
		return fmt.Errorf("partner_id cannot be empty")
	}

	result, err := DB.Exec(`DELETE FROM trading_partners WHERE partner_id = ?`, partnerID)
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

// ListPartners retrieves all trading partners
func ListPartners() ([]TradingPartner, error) {
	rows, err := DB.Query(
		`SELECT id, partner_id, name, jwks_url, message_endpoint, mdn_receipt_endpoint, public_key_jwks, last_key_refresh, created_at, updated_at 
		 FROM trading_partners ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query partners: %w", err)
	}
	defer rows.Close()

	var partners []TradingPartner
	for rows.Next() {
		var partner TradingPartner
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
		partners = append(partners, partner)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating partners: %w", err)
	}

	return partners, nil
}
