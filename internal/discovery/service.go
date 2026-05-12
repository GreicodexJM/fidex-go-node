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
	NodeID              string   `json:"node_id"`
	OrganizationName    string   `json:"organization_name"`
	JWKSUri             string   `json:"jwks_uri"`
	MessageEndpoint     string   `json:"message_endpoint"`
	MDNReceiptEndpoint  string   `json:"mdn_receipt_endpoint"`
	AlgorithmsSupported []string `json:"algorithms_supported"`
	// SupportedEncryptionAlgorithms is the partner-facing capability
	// advertisement for JWE key wrapping per ADR-0003 / FID-4. Optional;
	// peers that don't read it fall back to algorithms_supported.
	SupportedEncryptionAlgorithms []string `json:"supported_encryption_algorithms,omitempty"`
	WebhookRegistrationEndpoint   string   `json:"webhook_registration_endpoint"`
	SecurityToken                 string   `json:"security_token"`
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
	logger.Info(ctx, "Step 2: Fetching partner JWKS from %s", remoteConfig.Endpoints.JWKS)
	jwksData, err := FetchAndCachePartnerKeys(remoteConfig.Endpoints.JWKS)
	if err != nil {
		return nil, fmt.Errorf("step 2 failed - could not fetch JWKS: %w", err)
	}
	logger.Info(ctx, "Step 2 complete: Cached partner's public keys")

	// Step 3: Generate security token and send registration request
	logger.Info(ctx, "Step 3: Registering with partner via %s", remoteConfig.Endpoints.Register)
	token, err := ds.tokenStore.GenerateToken(30 * time.Minute)
	if err != nil {
		return nil, fmt.Errorf("step 3 failed - could not generate token: %w", err)
	}

	ownConfig := GenerateAS5Config(ds.nodeConfig)
	regRequest := RegistrationRequest{
		NodeID:                        ownConfig.NodeID,
		OrganizationName:              ownConfig.OrganizationName,
		JWKSUri:                       ownConfig.Endpoints.JWKS,
		MessageEndpoint:               ownConfig.Endpoints.ReceiveMessage,
		MDNReceiptEndpoint:            ownConfig.Endpoints.ReceiveReceipt,
		AlgorithmsSupported:           []string{ownConfig.Security.SignatureAlgorithm, ownConfig.Security.EncryptionAlgorithm, ownConfig.Security.ContentEncryption},
		SupportedEncryptionAlgorithms: ownConfig.Security.SupportedEncryptionAlgorithms,
		WebhookRegistrationEndpoint:   ownConfig.Endpoints.Register,
		SecurityToken:                 token,
	}

	if err := ds.sendRegistrationRequest(remoteConfig.Endpoints.Register, regRequest); err != nil {
		return nil, fmt.Errorf("step 3 failed - registration rejected: %w", err)
	}
	logger.Info(ctx, "Step 3 complete: Registration accepted by partner")

	// Step 4: Create partner profile in local database
	logger.Info(ctx, "Step 4: Creating partner profile in database")
	now := time.Now()
	partner := &domain.Partner{
		PartnerID:          remoteConfig.NodeID,
		Name:               remoteConfig.OrganizationName,
		JWKSUrl:            remoteConfig.Endpoints.JWKS,
		MessageEndpoint:    remoteConfig.Endpoints.ReceiveMessage,
		MDNReceiptEndpoint: remoteConfig.Endpoints.ReceiveReceipt,
		PublicKeyJWKS:      jwksData,
		// ADR-0003 / FID-4: hydrate the partner's advertised encryption
		// capability from the AS5 config. Honours the back-compat fallback
		// to the single encryption_algorithm field when the array is
		// absent. Not persisted (see Partner struct doc).
		SupportedEncryptionAlgorithms: ResolveSupportedEncryptionAlgorithms(remoteConfig),
		LastKeyRefresh:                &now,
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
		// ADR-0003 / FID-4: pick the partner's declared encryption capability
		// from the registration body when present.
		SupportedEncryptionAlgorithms: req.SupportedEncryptionAlgorithms,
		LastKeyRefresh:                &now,
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
