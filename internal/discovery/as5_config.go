package discovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/crypto"
)

// AS5Configuration represents the AS5 discovery document
type AS5Configuration struct {
	Issuer                      string   `json:"issuer"`
	OrganizationName            string   `json:"organization_name"`
	JWKSUri                     string   `json:"jwks_uri"`
	MessageEndpoint             string   `json:"message_endpoint"`
	MDNReceiptEndpoint          string   `json:"mdn_receipt_endpoint"`
	AlgorithmsSupported         []string `json:"algorithms_supported"`
	WebhookRegistrationEndpoint string   `json:"webhook_registration_endpoint"`
}

// NodeConfig holds the node's configuration for discovery
type NodeConfig struct {
	NodeID           string
	OrganizationName string
	BaseURL          string
}

// GenerateAS5Config creates the AS5 discovery document for this node
func GenerateAS5Config(config NodeConfig) *AS5Configuration {
	return &AS5Configuration{
		Issuer:                      config.NodeID,
		OrganizationName:            config.OrganizationName,
		JWKSUri:                     config.BaseURL + constants.RouteJWKS,
		MessageEndpoint:             config.BaseURL + constants.RouteInbound,
		MDNReceiptEndpoint:          config.BaseURL + constants.RouteReceipt,
		AlgorithmsSupported:         []string{"RS256", "RSA-OAEP", "A256GCM"},
		WebhookRegistrationEndpoint: config.BaseURL + constants.RouteRegister,
	}
}

// FetchAS5Config fetches and validates an AS5 configuration from a remote URL
func FetchAS5Config(discoveryURL string) (*AS5Configuration, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch AS5 config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch AS5 config: HTTP %d", resp.StatusCode)
	}

	var config AS5Configuration
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse AS5 config: %w", err)
	}

	// Validate required fields
	if err := validateAS5Config(&config); err != nil {
		return nil, fmt.Errorf("invalid AS5 config: %w", err)
	}

	return &config, nil
}

// validateAS5Config validates that all required fields are present
func validateAS5Config(config *AS5Configuration) error {
	if config.Issuer == "" {
		return fmt.Errorf("issuer is required")
	}
	if config.JWKSUri == "" {
		return fmt.Errorf("jwks_uri is required")
	}
	if config.MessageEndpoint == "" {
		return fmt.Errorf("message_endpoint is required")
	}
	if config.MDNReceiptEndpoint == "" {
		return fmt.Errorf("mdn_receipt_endpoint is required")
	}
	return nil
}

// FetchAndCachePartnerKeys fetches a partner's JWKS and returns it as a string
func FetchAndCachePartnerKeys(jwksURL string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(jwksURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch JWKS: HTTP %d", resp.StatusCode)
	}

	// Read and validate JSON
	var jwksData interface{}
	if err := json.NewDecoder(resp.Body).Decode(&jwksData); err != nil {
		return "", fmt.Errorf("invalid JWKS JSON: %w", err)
	}

	// Re-marshal to ensure it's valid
	jwksBytes, err := json.Marshal(jwksData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWKS: %w", err)
	}

	// Validate it's parseable as a JWKS
	_, err = crypto.ParsePublicKeyFromJWKS(string(jwksBytes))
	if err != nil {
		return "", fmt.Errorf("invalid JWKS format: %w", err)
	}

	return string(jwksBytes), nil
}
