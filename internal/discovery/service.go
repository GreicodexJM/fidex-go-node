package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"fidex-node/internal/domain"
	"fidex-node/internal/logging"
)

// logger is the package-level structured logger for discovery.
var logger = logging.New("discovery")

// RegistrationRequest represents the payload sent to partner webhook registration endpoint
type RegistrationRequest struct {
	NodeID                      string   `json:"node_id"`
	OrganizationName            string   `json:"organization_name"`
	JWKSUri                     string   `json:"jwks_uri"`
	MessageEndpoint             string   `json:"message_endpoint"`
	MDNReceiptEndpoint          string   `json:"mdn_receipt_endpoint"`
	AlgorithmsSupported         []string `json:"algorithms_supported"`
	WebhookRegistrationEndpoint string   `json:"webhook_registration_endpoint"`
	SecurityToken               string   `json:"security_token"`
}

// RegistrationResponse represents the response from partner webhook registration endpoint
type RegistrationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// DiscoveryService handles partner discovery and registration
type DiscoveryService struct {
	nodeConfig  NodeConfig
	tokenStore  *TokenStore
	partnerRepo domain.PartnerRepository
}

// NewDiscoveryService creates a new discovery service with an injected partner repository
func NewDiscoveryService(nodeConfig NodeConfig, tokenStore *TokenStore, partnerRepo domain.PartnerRepository) *DiscoveryService {
	return &DiscoveryService{
		nodeConfig:  nodeConfig,
		tokenStore:  tokenStore,
		partnerRepo: partnerRepo,
	}
}

// InitiatePartnerHandshake performs the complete 4-step discovery process
func (ds *DiscoveryService) InitiatePartnerHandshake(ctx context.Context, discoveryURL string) (*domain.Partner, error) {
	// Step 1: Fetch AS5 configuration from remote node
	logger.Info(ctx, "Step 1: Fetching AS5 configuration from %s", discoveryURL)
	remoteConfig, err := FetchAS5Config(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("step 1 failed - could not fetch AS5 config: %w", err)
	}
	logger.Info(ctx, "Step 1 complete: Retrieved config from %s", remoteConfig.OrganizationName)

	// Step 2: Fetch and cache partner's public keys
	logger.Info(ctx, "Step 2: Fetching partner JWKS from %s", remoteConfig.JWKSUri)
	jwksData, err := FetchAndCachePartnerKeys(remoteConfig.JWKSUri)
	if err != nil {
		return nil, fmt.Errorf("step 2 failed - could not fetch JWKS: %w", err)
	}
	logger.Info(ctx, "Step 2 complete: Cached partner's public keys")

	// Step 3: Generate security token and send registration request
	logger.Info(ctx, "Step 3: Registering with partner via %s", remoteConfig.WebhookRegistrationEndpoint)
	token, err := ds.tokenStore.GenerateToken(30 * time.Minute)
	if err != nil {
		return nil, fmt.Errorf("step 3 failed - could not generate token: %w", err)
	}

	ownConfig := GenerateAS5Config(ds.nodeConfig)
	regRequest := RegistrationRequest{
		NodeID:                      ownConfig.Issuer,
		OrganizationName:            ownConfig.OrganizationName,
		JWKSUri:                     ownConfig.JWKSUri,
		MessageEndpoint:             ownConfig.MessageEndpoint,
		MDNReceiptEndpoint:          ownConfig.MDNReceiptEndpoint,
		AlgorithmsSupported:         ownConfig.AlgorithmsSupported,
		WebhookRegistrationEndpoint: ownConfig.WebhookRegistrationEndpoint,
		SecurityToken:               token,
	}

	if err := ds.sendRegistrationRequest(remoteConfig.WebhookRegistrationEndpoint, regRequest); err != nil {
		return nil, fmt.Errorf("step 3 failed - registration rejected: %w", err)
	}
	logger.Info(ctx, "Step 3 complete: Registration accepted by partner")

	// Step 4: Create partner profile in local database
	logger.Info(ctx, "Step 4: Creating partner profile in database")
	now := time.Now()
	partner := &domain.Partner{
		PartnerID:          remoteConfig.Issuer,
		Name:               remoteConfig.OrganizationName,
		JWKSUrl:            remoteConfig.JWKSUri,
		MessageEndpoint:    remoteConfig.MessageEndpoint,
		MDNReceiptEndpoint: remoteConfig.MDNReceiptEndpoint,
		PublicKeyJWKS:      jwksData,
		LastKeyRefresh:     &now,
	}

	if err := ds.partnerRepo.Create(ctx, partner); err != nil {
		return nil, fmt.Errorf("step 4 failed - could not create partner: %w", err)
	}
	logger.Info(ctx, "Step 4 complete: Partner profile created for %s", partner.Name)

	logger.Info(ctx, "✓ Handshake complete with %s", partner.Name)
	return partner, nil
}

// sendRegistrationRequest sends the registration request to partner's webhook endpoint
func (ds *DiscoveryService) sendRegistrationRequest(webhookURL string, req RegistrationRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal registration request: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to send registration request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration rejected with status: %d", resp.StatusCode)
	}

	var regResp RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		return fmt.Errorf("failed to decode registration response: %w", err)
	}

	if !regResp.Success {
		return fmt.Errorf("registration rejected: %s", regResp.Message)
	}

	return nil
}

// HandleWebhookRegistration processes incoming registration requests from partners
func (ds *DiscoveryService) HandleWebhookRegistration(ctx context.Context, req RegistrationRequest) error {
	// Validate security token
	if !ds.tokenStore.ValidateAndConsume(req.SecurityToken) {
		return fmt.Errorf("invalid or expired security token")
	}

	// Validate required fields
	if req.NodeID == "" || req.OrganizationName == "" || req.JWKSUri == "" {
		return fmt.Errorf("missing required fields")
	}

	// Fetch and cache partner's public keys
	jwksData, err := FetchAndCachePartnerKeys(req.JWKSUri)
	if err != nil {
		return fmt.Errorf("failed to fetch partner JWKS: %w", err)
	}

	// Create partner profile
	now := time.Now()
	partner := &domain.Partner{
		PartnerID:          req.NodeID,
		Name:               req.OrganizationName,
		JWKSUrl:            req.JWKSUri,
		MessageEndpoint:    req.MessageEndpoint,
		MDNReceiptEndpoint: req.MDNReceiptEndpoint,
		PublicKeyJWKS:      jwksData,
		LastKeyRefresh:     &now,
	}

	// Check if partner already exists
	existingPartner, err := ds.partnerRepo.GetByID(ctx, req.NodeID)
	if err == nil && existingPartner != nil {
		// Update existing partner
		partner.ID = existingPartner.ID
		if err := ds.partnerRepo.Update(ctx, partner); err != nil {
			return fmt.Errorf("failed to update partner: %w", err)
		}
		logger.Info(ctx, "Updated existing partner: %s", partner.Name)
	} else {
		// Create new partner
		if err := ds.partnerRepo.Create(ctx, partner); err != nil {
			return fmt.Errorf("failed to create partner: %w", err)
		}
		logger.Info(ctx, "Created new partner: %s", partner.Name)
	}

	return nil
}
