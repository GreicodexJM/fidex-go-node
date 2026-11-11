package discovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/domain"
	"fidex-node/internal/repository"
)

// testEnv holds the per-test database connection and partner repository.
type testEnv struct {
	dbPath      string
	conn        *sql.DB
	partnerRepo domain.PartnerRepository
}

// setupTestDB creates a temporary test database and partner repository.
func setupTestDB(t *testing.T) *testEnv {
	t.Helper()
	dbPath := fmt.Sprintf("/tmp/test_discovery_%d.db", time.Now().UnixNano())

	conn, err := repository.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	if err := repository.InitSchema(conn); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	return &testEnv{
		dbPath:      dbPath,
		conn:        conn,
		partnerRepo: repository.NewSQLitePartnerRepository(conn),
	}
}

// cleanup releases resources for the test environment.
func (env *testEnv) cleanup() {
	_ = env.conn.Close()
	os.Remove(env.dbPath)
	os.Remove(env.dbPath + "-shm")
	os.Remove(env.dbPath + "-wal")
}

// TestGenerateAS5Config tests AS5 configuration generation against the spec schema.
func TestGenerateAS5Config(t *testing.T) {
	config := NodeConfig{
		NodeID:           "urn:gln:test:node-a",
		OrganizationName: "Test Node A",
		BaseURL:          "https://node-a.example.com",
		PublicDomain:     "node-a.example.com",
	}

	as5Config := GenerateAS5Config(config)

	if as5Config.NodeID != config.NodeID {
		t.Errorf("Expected node_id %s, got %s", config.NodeID, as5Config.NodeID)
	}
	if as5Config.OrganizationName != config.OrganizationName {
		t.Errorf("Expected organization_name %s, got %s", config.OrganizationName, as5Config.OrganizationName)
	}
	if as5Config.PublicDomain != config.PublicDomain {
		t.Errorf("Expected public_domain %s, got %s", config.PublicDomain, as5Config.PublicDomain)
	}
	if as5Config.FidexVersion != "1.0" {
		t.Errorf("Expected fidex_version 1.0, got %s", as5Config.FidexVersion)
	}
	if len(as5Config.SupportedVersions) == 0 {
		t.Error("supported_versions must be non-empty")
	}
	if as5Config.Endpoints.JWKS != config.BaseURL+constants.RouteJWKS {
		t.Errorf("Unexpected endpoints.jwks: %s", as5Config.Endpoints.JWKS)
	}
	if as5Config.Endpoints.ReceiveMessage != config.BaseURL+constants.RouteInbound {
		t.Errorf("Unexpected endpoints.receive_message: %s", as5Config.Endpoints.ReceiveMessage)
	}
	if as5Config.Endpoints.ReceiveReceipt != config.BaseURL+constants.RouteReceipt {
		t.Errorf("Unexpected endpoints.receive_receipt: %s", as5Config.Endpoints.ReceiveReceipt)
	}
	if as5Config.Endpoints.Register != config.BaseURL+constants.RouteRegister {
		t.Errorf("Unexpected endpoints.register: %s", as5Config.Endpoints.Register)
	}
	if as5Config.Security.SignatureAlgorithm == "" {
		t.Error("security.signature_algorithm is required")
	}
	if as5Config.Security.EncryptionAlgorithm == "" {
		t.Error("security.encryption_algorithm is required")
	}
	if as5Config.Security.MinimumKeySize <= 0 {
		t.Error("security.minimum_key_size must be positive")
	}

	t.Log("✓ AS5 config generation successful")
}

// TestFetchAS5Config tests fetching AS5 configuration from a remote server.
func TestFetchAS5Config(t *testing.T) {
	mockConfig := AS5Configuration{
		FidexVersion:      "1.0",
		SupportedVersions: []string{"1.0"},
		NodeID:            "urn:gln:test:remote-node",
		OrganizationName:  "Remote Node",
		PublicDomain:      "remote.example.com",
		Endpoints: AS5Endpoints{
			JWKS:           "https://remote.example.com/.well-known/jwks.json",
			ReceiveMessage: "https://remote.example.com/api/v1/receive",
			ReceiveReceipt: "https://remote.example.com/api/v1/receipt",
			Register:       "https://remote.example.com/api/v1/register",
		},
		Security: AS5SecurityConfig{
			SignatureAlgorithm:  "RS256",
			EncryptionAlgorithm: "RSA-OAEP",
			ContentEncryption:   "A256GCM",
			MinimumKeySize:      2048,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockConfig)
	}))
	defer server.Close()

	fetchedConfig, err := FetchAS5Config(server.URL)
	if err != nil {
		t.Fatalf("Failed to fetch AS5 config: %v", err)
	}

	if fetchedConfig.NodeID != mockConfig.NodeID {
		t.Errorf("Expected node_id %s, got %s", mockConfig.NodeID, fetchedConfig.NodeID)
	}
	if fetchedConfig.OrganizationName != mockConfig.OrganizationName {
		t.Errorf("Expected organization_name %s, got %s", mockConfig.OrganizationName, fetchedConfig.OrganizationName)
	}
	if fetchedConfig.Endpoints.ReceiveMessage != mockConfig.Endpoints.ReceiveMessage {
		t.Errorf("Expected endpoints.receive_message %s, got %s", mockConfig.Endpoints.ReceiveMessage, fetchedConfig.Endpoints.ReceiveMessage)
	}

	t.Log("✓ AS5 config fetching successful")
}

// TestFetchAS5ConfigInvalidURL tests fetching from an invalid URL
func TestFetchAS5ConfigInvalidURL(t *testing.T) {
	_, err := FetchAS5Config("http://invalid-domain-that-does-not-exist.local/config")
	if err == nil {
		t.Error("Expected error with invalid URL, got nil")
	}
	t.Log("✓ Invalid URL error handling works")
}

// TestFetchAS5ConfigInvalidJSON tests fetching invalid JSON
func TestFetchAS5ConfigInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	_, err := FetchAS5Config(server.URL)
	if err == nil {
		t.Error("Expected error with invalid JSON, got nil")
	}
	t.Log("✓ Invalid JSON error handling works")
}

// TestTokenStore tests the token generation and validation
func TestTokenStore(t *testing.T) {
	store := NewTokenStore()

	token, err := store.GenerateToken(5 * time.Minute)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Error("Generated token is empty")
	}

	if !store.ValidateAndConsume(token) {
		t.Error("Token validation failed")
	}

	if store.ValidateAndConsume(token) {
		t.Error("Token should not be valid after being consumed")
	}

	t.Log("✓ Token generation and validation successful")
}

// TestTokenStoreExpiration tests token expiration
func TestTokenStoreExpiration(t *testing.T) {
	store := NewTokenStore()

	token, err := store.GenerateToken(1 * time.Second)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	time.Sleep(2 * time.Second)

	if store.ValidateAndConsume(token) {
		t.Error("Expired token should not be valid")
	}

	t.Log("✓ Token expiration works correctly")
}

// The registration handshake (spec §6.3–§6.4), initiator and responder
// sides, is tested in registration_test.go.

// TestPartnerDatabaseOperations tests CRUD operations on trading partners
func TestPartnerDatabaseOperations(t *testing.T) {
	env := setupTestDB(t)
	defer env.cleanup()

	ctx := context.Background()

	partner := &domain.Partner{
		PartnerID:          "urn:gln:test:partner-1",
		Name:               "Test Partner 1",
		JWKSUrl:            "https://partner1.example.com/jwks",
		MessageEndpoint:    "https://partner1.example.com/inbound",
		MDNReceiptEndpoint: "https://partner1.example.com/receipt",
		PublicKeyJWKS:      `{"keys":[]}`,
	}

	if err := env.partnerRepo.Create(ctx, partner); err != nil {
		t.Fatalf("Failed to create partner: %v", err)
	}
	if partner.ID == 0 {
		t.Error("Partner ID was not set")
	}

	retrieved, err := env.partnerRepo.GetByID(ctx, "urn:gln:test:partner-1")
	if err != nil {
		t.Fatalf("Failed to retrieve partner: %v", err)
	}
	if retrieved.Name != "Test Partner 1" {
		t.Errorf("Expected name 'Test Partner 1', got %s", retrieved.Name)
	}

	retrieved.Name = "Updated Partner 1"
	if err := env.partnerRepo.Update(ctx, retrieved); err != nil {
		t.Fatalf("Failed to update partner: %v", err)
	}

	updated, err := env.partnerRepo.GetByID(ctx, "urn:gln:test:partner-1")
	if err != nil {
		t.Fatalf("Failed to retrieve updated partner: %v", err)
	}
	if updated.Name != "Updated Partner 1" {
		t.Errorf("Expected updated name 'Updated Partner 1', got %s", updated.Name)
	}

	partners, err := env.partnerRepo.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list partners: %v", err)
	}
	if len(partners) != 1 {
		t.Errorf("Expected 1 partner, got %d", len(partners))
	}

	if err := env.partnerRepo.Delete(ctx, "urn:gln:test:partner-1"); err != nil {
		t.Fatalf("Failed to delete partner: %v", err)
	}

	_, err = env.partnerRepo.GetByID(ctx, "urn:gln:test:partner-1")
	if err == nil {
		t.Error("Expected error when retrieving deleted partner")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}

	t.Log("✓ Partner database operations successful")
}

// TestPartnerDatabaseConstraints tests database constraints
func TestPartnerDatabaseConstraints(t *testing.T) {
	env := setupTestDB(t)
	defer env.cleanup()

	ctx := context.Background()

	if err := env.partnerRepo.Create(ctx, nil); err == nil {
		t.Error("Expected error with nil partner")
	}

	partner := &domain.Partner{
		Name:    "Test",
		JWKSUrl: "https://test.com/jwks",
	}
	if err := env.partnerRepo.Create(ctx, partner); err == nil {
		t.Error("Expected error with empty partner ID")
	}

	partner1 := &domain.Partner{
		PartnerID: "urn:gln:test:duplicate",
		Name:      "Partner 1",
		JWKSUrl:   "https://test1.com/jwks",
	}
	if err := env.partnerRepo.Create(ctx, partner1); err != nil {
		t.Fatalf("Failed to create first partner: %v", err)
	}

	partner2 := &domain.Partner{
		PartnerID: "urn:gln:test:duplicate",
		Name:      "Partner 2",
		JWKSUrl:   "https://test2.com/jwks",
	}
	if err := env.partnerRepo.Create(ctx, partner2); err == nil {
		t.Error("Expected error with duplicate partner ID")
	}

	t.Log("✓ Database constraints working correctly")
}

// TestDiscoveryWithConnectionValidation tests the discovery process with a test message
func TestDiscoveryWithConnectionValidation(t *testing.T) {
	t.Log("=== Testing Discovery with Connection Validation ===")

	env := setupTestDB(t)
	defer env.cleanup()

	ctx := context.Background()

	partner := &domain.Partner{
		PartnerID:          "urn:gln:test:validated-partner",
		Name:               "Validated Partner",
		JWKSUrl:            "https://validated.example.com/jwks",
		MessageEndpoint:    "https://validated.example.com/inbound",
		MDNReceiptEndpoint: "https://validated.example.com/receipt",
		PublicKeyJWKS:      `{"keys":[{"kty":"RSA","use":"enc","kid":"test","alg":"RSA-OAEP","n":"test","e":"AQAB"}]}`,
	}

	if err := env.partnerRepo.Create(ctx, partner); err != nil {
		t.Fatalf("Failed to create partner: %v", err)
	}

	testMessageReceived := false

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inbound" && r.Method == "POST" {
			testMessageReceived = true
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer testServer.Close()

	partner.MessageEndpoint = testServer.URL + "/inbound"
	if err := env.partnerRepo.Update(ctx, partner); err != nil {
		t.Fatalf("Failed to update partner: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	testPayload := `{"test":true}`
	resp, err := client.Post(partner.MessageEndpoint, "application/json", strings.NewReader(testPayload))
	if err != nil {
		t.Fatalf("Failed to send test message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", resp.StatusCode)
	}

	if !testMessageReceived {
		t.Error("Test message was not received by partner")
	}

	t.Log("=== ✓ Connection Validation Successful ===")
}
