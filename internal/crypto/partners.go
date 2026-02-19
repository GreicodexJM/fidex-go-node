package crypto

import (
	"crypto/rsa"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"time"

	"fidex-node/internal/db"
)

// TradingPartner represents a trading partner configuration
type TradingPartner struct {
	ID             int64
	PartnerID      string
	Name           string
	JWKSUrl        string
	PublicKeyJWKS  string
	LastKeyRefresh *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// KeyRefreshInterval defines how often to refresh partner keys (24 hours)
const KeyRefreshInterval = 24 * time.Hour

// GetTradingPartner retrieves a trading partner by partner_id
func GetTradingPartner(partnerID string) (*TradingPartner, error) {
	var partner TradingPartner
	var lastRefresh sql.NullTime

	err := db.DB.QueryRow(
		`SELECT id, partner_id, name, jwks_url, public_key_jwks, last_key_refresh, created_at, updated_at
		 FROM trading_partners WHERE partner_id = ?`,
		partnerID,
	).Scan(&partner.ID, &partner.PartnerID, &partner.Name, &partner.JWKSUrl,
		&partner.PublicKeyJWKS, &lastRefresh, &partner.CreatedAt, &partner.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("trading partner not found: %s", partnerID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trading partner: %w", err)
	}

	if lastRefresh.Valid {
		partner.LastKeyRefresh = &lastRefresh.Time
	}

	return &partner, nil
}

// UpsertTradingPartner inserts or updates a trading partner
func UpsertTradingPartner(partner *TradingPartner) error {
	now := time.Now()

	result, err := db.DB.Exec(
		`INSERT INTO trading_partners (partner_id, name, jwks_url, public_key_jwks, last_key_refresh, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(partner_id) DO UPDATE SET
		   name = excluded.name,
		   jwks_url = excluded.jwks_url,
		   public_key_jwks = excluded.public_key_jwks,
		   last_key_refresh = excluded.last_key_refresh,
		   updated_at = excluded.updated_at`,
		partner.PartnerID, partner.Name, partner.JWKSUrl, partner.PublicKeyJWKS,
		partner.LastKeyRefresh, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert trading partner: %w", err)
	}

	if partner.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			partner.ID = id
		}
	}

	return nil
}

// FetchPartnerPublicKey retrieves and caches a partner's public key
// It will fetch from the partner's JWKS URL if the key is stale or not cached
func FetchPartnerPublicKey(partnerID string) (*rsa.PublicKey, error) {
	// Try to get from database first
	partner, err := GetTradingPartner(partnerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get partner: %w", err)
	}

	// Check if we need to refresh the key
	needsRefresh := partner.PublicKeyJWKS == "" ||
		partner.LastKeyRefresh == nil ||
		time.Since(*partner.LastKeyRefresh) > KeyRefreshInterval

	if needsRefresh {
		// Fetch fresh JWKS from partner's endpoint
		jwksJSON, err := fetchJWKS(partner.JWKSUrl)
		if err != nil {
			// If fetch fails and we have a cached key, use it
			if partner.PublicKeyJWKS != "" {
				publicKey, err := ParsePublicKeyFromJWKS(partner.PublicKeyJWKS)
				if err == nil {
					return publicKey, nil
				}
			}
			return nil, fmt.Errorf("failed to fetch JWKS and no cached key available: %w", err)
		}

		// Update the cached key
		now := time.Now()
		partner.PublicKeyJWKS = jwksJSON
		partner.LastKeyRefresh = &now

		if err := UpsertTradingPartner(partner); err != nil {
			// Log error but continue with the fetched key
			fmt.Printf("Warning: failed to update cached key: %v\n", err)
		}
	}

	// Parse and return the public key
	publicKey, err := ParsePublicKeyFromJWKS(partner.PublicKeyJWKS)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return publicKey, nil
}

// fetchJWKS fetches a JWKS from a remote URL
func fetchJWKS(url string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch JWKS: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read JWKS response: %w", err)
	}

	return string(body), nil
}

// ListTradingPartners retrieves all trading partners
func ListTradingPartners() ([]TradingPartner, error) {
	rows, err := db.DB.Query(
		`SELECT id, partner_id, name, jwks_url, public_key_jwks, last_key_refresh, created_at, updated_at
		 FROM trading_partners ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list trading partners: %w", err)
	}
	defer rows.Close()

	var partners []TradingPartner
	for rows.Next() {
		var partner TradingPartner
		var lastRefresh sql.NullTime

		if err := rows.Scan(&partner.ID, &partner.PartnerID, &partner.Name, &partner.JWKSUrl,
			&partner.PublicKeyJWKS, &lastRefresh, &partner.CreatedAt, &partner.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan partner: %w", err)
		}

		if lastRefresh.Valid {
			partner.LastKeyRefresh = &lastRefresh.Time
		}

		partners = append(partners, partner)
	}

	return partners, nil
}

// DeleteTradingPartner removes a trading partner
func DeleteTradingPartner(partnerID string) error {
	result, err := db.DB.Exec(`DELETE FROM trading_partners WHERE partner_id = ?`, partnerID)
	if err != nil {
		return fmt.Errorf("failed to delete trading partner: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("trading partner not found: %s", partnerID)
	}

	return nil
}
