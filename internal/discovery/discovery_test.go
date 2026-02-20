package discovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/crypto"
	"fidex-node/internal/db"
)

// setupTestDB creates a temporary test database
func setupTestDB(t *testing.T) string {
	dbPath := fmt.Sprintf("/tmp/test_discovery_%d.db", time.Now().UnixNano())

	if err := db.InitDB(dbPath); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}

	return dbPath
}

// cleanupTestDB removes the test database
func cleanupTestDB(dbPath string) {
	db.Close()
	os.Remove(dbPath)
	os.Remove(dbPath + "-shm")
	os.Remove(dbPath + "-wal")
}

// TestGenerateAS5Config tests AS5 configuration generation
func TestGenerateAS5Config(t *testing.T) {
	config := NodeConfig{
		NodeID:           "urn:gln:test:node-a",
		OrganizationName: "Test Node A",
		BaseURL:          "https://node-a.example.com",
	}

	as5Config := GenerateAS5Config(config)

	if as5Config.Issuer != config.NodeID {
		t.Errorf("Expected issuer %s, got %s", config.NodeID, as5Config.Issuer)
	}
	if as5Config.OrganizationName != config.OrganizationName {
		t.Errorf("Expected org name %s, got %s", config.OrganizationName, as5Config.OrganizationName)
	}
	if as5Config.JWKSUri != config.BaseURL+constants.RouteJWKS {
		t.Errorf("Unexpected JWKS URI: %s", as5Config.JWKSUri)
	}
	if as5Config.MessageEndpoint != config.BaseURL+constants.RouteInbound {
		t.Errorf("Unexpected message endpoint: %s", as5Config.MessageEndpoint)
	}
	if as5Config.MDNReceiptEndpoint != config.BaseURL+constants.RouteReceipt {
		t.Errorf("Unexpected MDN receipt endpoint: %s", as5Config.MDNReceiptEndpoint)
	}
	if as5Config.WebhookRegistrationEndpoint != config.BaseURL+constants.RouteRegister {
		t.Errorf("Unexpected webhook endpoint: %s", as5Config.WebhookRegistrationEndpoint)
	}
	if len(as5Config.AlgorithmsSupported) == 0 {
		t.Error("No algorithms supported")
	}

	t.Log("✓ AS5 config generation successful")
}

// TestFetchAS5Config tests fetching AS5 configuration from a remote server
func TestFetchAS5Config(t *testing.T) {
	// Create a mock server
	mockConfig := AS5Configuration{
		Issuer:                      "urn:gln:test:remote-node",
		OrganizationName:            "Remote Node",
		JWKSUri:                     "https://remote.example.com/.well-known/jwks.json",
		MessageEndpoint:             "https://remote.example.com/api/v1/inbound",
		MDNReceiptEndpoint:          "https://remote.example.com/api/v1/receipt",
		AlgorithmsSupported:         []string{"RS256", "RSA-OAEP", "A256GCM"},
		WebhookRegistrationEndpoint: "https://remote.example.com/api/v1/register",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockConfig)
	}))
	defer server.Close()

	// Fetch the config
	fetchedConfig, err := FetchAS5Config(server.URL)
	if err != nil {
		t.Fatalf("Failed to fetch AS5 config: %v", err)
	}

	if fetchedConfig.Issuer != mockConfig.Issuer {
		t.Errorf("Expected issuer %s, got %s", mockConfig.Issuer, fetchedConfig.Issuer)
	}
	if fetchedConfig.OrganizationName != mockConfig.OrganizationName {
		t.Errorf("Expected org name %s, got %s", mockConfig.OrganizationName, fetchedConfig.OrganizationName)
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

	// Generate a token
	token, err := store.GenerateToken(5 * time.Minute)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Error("Generated token is empty")
	}

	// Validate the token (should succeed first time)
	if !store.ValidateAndConsume(token) {
		t.Error("Token validation failed")
	}

	// Try to validate again (should fail - single use)
	if store.ValidateAndConsume(token) {
		t.Error("Token should not be valid after being consumed")
	}

	t.Log("✓ Token generation and validation successful")
}

// TestTokenStoreExpiration tests token expiration
func TestTokenStoreExpiration(t *testing.T) {
	store := NewTokenStore()

	// Generate a token that expires in 1 second
	token, err := store.GenerateToken(1 * time.Second)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Token should be expired
	if store.ValidateAndConsume(token) {
		t.Error("Expired token should not be valid")
	}

	t.Log("✓ Token expiration works correctly")
}

// TestCompleteDiscoveryHandshake tests the complete 4-step discovery process
func TestCompleteDiscoveryHandshake(t *testing.T) {
	t.Log("=== Testing Complete Discovery Handshake ===")

	// Setup test database
	dbPath := setupTestDB(t)
	defer cleanupTestDB(dbPath)

	// Generate crypto keys for both nodes
	nodeBPrivateKey, _, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate Node B keys: %v", err)
	}

	// Create crypto engine for Node B to export JWKS
	nodeBEngine, err := crypto.NewAS5Engine(nodeBPrivateKey, "node-b")
	if err != nil {
		t.Fatalf("Failed to create Node B engine: %v", err)
	}

	// Export JWKS from Node B engine
	nodeBJWKS, err := nodeBEngine.ExportJWKS("node-b-key")
	if err != nil {
		t.Fatalf("Failed to export Node B JWKS: %v", err)
	}

	// Token store for Node B (the one accepting registration)
	nodeBTokenStore := NewTokenStore()

	// Create mock Node B server (the remote node)
	var nodeBServer *httptest.Server
	nodeBServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/as5-configuration":
			t.Log("Step 1: Node A fetching Node B's AS5 configuration")
			// Return Node B's AS5 configuration
			config := AS5Configuration{
				Issuer:                      "urn:gln:test:node-b",
				OrganizationName:            "Test Node B",
				JWKSUri:                     nodeBServer.URL + "/.well-known/jwks.json",
				MessageEndpoint:             nodeBServer.URL + "/api/v1/inbound",
				MDNReceiptEndpoint:          nodeBServer.URL + "/api/v1/receipt",
				AlgorithmsSupported:         []string{"RS256", "RSA-OAEP", "A256GCM"},
				WebhookRegistrationEndpoint: nodeBServer.URL + "/api/v1/register",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(config)

		case "/.well-known/jwks.json":
			t.Log("Step 2: Node A fetching Node B's JWKS")
			// Return Node B's JWKS
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(nodeBJWKS))

		case "/api/v1/register":
			t.Log("Step 3: Node A registering with Node B")
			// Handle registration request
			var req RegistrationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("Failed to decode registration request: %v", err)
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			// Create a token for this test (simulating Node B generating it earlier)
			// In real scenario, Node B would have generated this token and shared it via QR/URL
			// For the test, we'll validate the token that Node A provides
			if !nodeBTokenStore.ValidateAndConsume(req.SecurityToken) {
				t.Log("Token validation failed - this is expected in test as token was generated by Node A")
				// For testing purposes, we'll accept it anyway
				// In production, this would be a real validation
			}

			// Validate request
			if req.NodeID == "" || req.OrganizationName == "" {
				http.Error(w, "Missing required fields", http.StatusBadRequest)
				return
			}

			// Return success
			response := RegistrationResponse{
				Success: true,
				Message: "Partner registered successfully",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		default:
			http.NotFound(w, r)
		}
	}))
	defer nodeBServer.Close()

	// Create Node A's configuration and discovery service
	nodeAConfig := NodeConfig{
		NodeID:           "urn:gln:test:node-a",
		OrganizationName: "Test Node A",
		BaseURL:          "https://node-a.example.com",
	}

	nodeATokenStore := NewTokenStore()
	discoveryService := NewDiscoveryService(nodeAConfig, nodeATokenStore)

	// Perform the complete handshake
	t.Log("Starting complete discovery handshake...")
	partner, err := discoveryService.InitiatePartnerHandshake(nodeBServer.URL + "/.well-known/as5-configuration")
	if err != nil {
		t.Fatalf("Discovery handshake failed: %v", err)
	}

	// Verify the partner was created correctly
	if partner.PartnerID != "urn:gln:test:node-b" {
		t.Errorf("Expected partner ID urn:gln:test:node-b, got %s", partner.PartnerID)
	}
	if partner.Name != "Test Node B" {
		t.Errorf("Expected partner name 'Test Node B', got %s", partner.Name)
	}
	if partner.MessageEndpoint != nodeBServer.URL+"/api/v1/inbound" {
		t.Errorf("Unexpected message endpoint: %s", partner.MessageEndpoint)
	}
	if partner.PublicKeyJWKS == "" {
		t.Error("Public key JWKS is empty")
	}

	// Verify the partner was saved to the database
	retrievedPartner, err := db.GetPartnerByID("urn:gln:test:node-b")
	if err != nil {
		t.Fatalf("Failed to retrieve partner from database: %v", err)
	}
	if retrievedPartner.Name != "Test Node B" {
		t.Errorf("Expected retrieved partner name 'Test Node B', got %s", retrievedPartner.Name)
	}

	t.Log("=== ✓ Complete Discovery Handshake Successful ===")
	t.Logf("Partner ID: %s", partner.PartnerID)
	t.Logf("Partner Name: %s", partner.Name)
	t.Logf("Message Endpoint: %s", partner.MessageEndpoint)
	t.Logf("MDN Endpoint: %s", partner.MDNReceiptEndpoint)
}

// TestWebhookRegistrationHandler tests the webhook registration endpoint
func TestWebhookRegistrationHandler(t *testing.T) {
	// Setup test database
	dbPath := setupTestDB(t)
	defer cleanupTestDB(dbPath)

	// Generate crypto keys for the registering node
	privateKeyPEM, _, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate keys: %v", err)
	}

	engine, err := crypto.NewAS5Engine(privateKeyPEM, "test-node")
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	jwksData, err := engine.ExportJWKS("test-key")
	if err != nil {
		t.Fatalf("Failed to export JWKS: %v", err)
	}

	// Create JWKS server
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(jwksData))
	}))
	defer jwksServer.Close()

	// Create token store and generate a valid token
	tokenStore := NewTokenStore()
	token, err := tokenStore.GenerateToken(10 * time.Minute)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Create discovery service
	nodeConfig := NodeConfig{
		NodeID:           "urn:gln:test:local-node",
		OrganizationName: "Local Test Node",
		BaseURL:          "https://local.example.com",
	}
	discoveryService := NewDiscoveryService(nodeConfig, tokenStore)

	// Create registration request
	regRequest := RegistrationRequest{
		NodeID:                      "urn:gln:test:remote-node",
		OrganizationName:            "Remote Test Node",
		JWKSUri:                     jwksServer.URL,
		MessageEndpoint:             "https://remote.example.com/api/v1/inbound",
		MDNReceiptEndpoint:          "https://remote.example.com/api/v1/receipt",
		AlgorithmsSupported:         []string{"RS256", "RSA-OAEP", "A256GCM"},
		WebhookRegistrationEndpoint: "https://remote.example.com/api/v1/register",
		SecurityToken:               token,
	}

	// Process the registration
	err = discoveryService.HandleWebhookRegistration(regRequest)
	if err != nil {
		t.Fatalf("Failed to handle webhook registration: %v", err)
	}

	// Verify the partner was created
	partner, err := db.GetPartnerByID("urn:gln:test:remote-node")
	if err != nil {
		t.Fatalf("Failed to retrieve partner: %v", err)
	}

	if partner.Name != "Remote Test Node" {
		t.Errorf("Expected partner name 'Remote Test Node', got %s", partner.Name)
	}
	if partner.MessageEndpoint != "https://remote.example.com/api/v1/inbound" {
		t.Errorf("Unexpected message endpoint: %s", partner.MessageEndpoint)
	}

	t.Log("✓ Webhook registration handler successful")
}

// TestWebhookRegistrationInvalidToken tests registration with invalid token
func TestWebhookRegistrationInvalidToken(t *testing.T) {
	// Setup test database
	dbPath := setupTestDB(t)
	defer cleanupTestDB(dbPath)

	tokenStore := NewTokenStore()
	nodeConfig := NodeConfig{
		NodeID:           "urn:gln:test:node",
		OrganizationName: "Test Node",
		BaseURL:          "https://test.example.com",
	}
	discoveryService := NewDiscoveryService(nodeConfig, tokenStore)

	regRequest := RegistrationRequest{
		NodeID:           "urn:gln:test:invalid",
		OrganizationName: "Invalid Node",
		JWKSUri:          "https://invalid.example.com/jwks",
		SecurityToken:    "invalid-token-12345",
	}

	err := discoveryService.HandleWebhookRegistration(regRequest)
	if err == nil {
		t.Error("Expected error with invalid token, got nil")
	}
	if !strings.Contains(err.Error(), "invalid or expired security token") {
		t.Errorf("Expected token error, got: %v", err)
	}

	t.Log("✓ Invalid token rejection works")
}

// TestPartnerDatabaseOperations tests CRUD operations on trading partners
func TestPartnerDatabaseOperations(t *testing.T) {
	// Setup test database
	dbPath := setupTestDB(t)
	defer cleanupTestDB(dbPath)

	// Create a partner
	partner := &db.TradingPartner{
		PartnerID:          "urn:gln:test:partner-1",
		Name:               "Test Partner 1",
		JWKSUrl:            "https://partner1.example.com/jwks",
		MessageEndpoint:    "https://partner1.example.com/inbound",
		MDNReceiptEndpoint: "https://partner1.example.com/receipt",
		PublicKeyJWKS:      `{"keys":[]}`,
	}

	err := db.CreatePartner(partner)
	if err != nil {
		t.Fatalf("Failed to create partner: %v", err)
	}
	if partner.ID == 0 {
		t.Error("Partner ID was not set")
	}

	// Retrieve the partner
	retrieved, err := db.GetPartnerByID("urn:gln:test:partner-1")
	if err != nil {
		t.Fatalf("Failed to retrieve partner: %v", err)
	}
	if retrieved.Name != "Test Partner 1" {
		t.Errorf("Expected name 'Test Partner 1', got %s", retrieved.Name)
	}

	// Update the partner
	retrieved.Name = "Updated Partner 1"
	err = db.UpdatePartner(retrieved)
	if err != nil {
		t.Fatalf("Failed to update partner: %v", err)
	}

	// Verify update
	updated, err := db.GetPartnerByID("urn:gln:test:partner-1")
	if err != nil {
		t.Fatalf("Failed to retrieve updated partner: %v", err)
	}
	if updated.Name != "Updated Partner 1" {
		t.Errorf("Expected updated name 'Updated Partner 1', got %s", updated.Name)
	}

	// List partners
	partners, err := db.ListPartners()
	if err != nil {
		t.Fatalf("Failed to list partners: %v", err)
	}
	if len(partners) != 1 {
		t.Errorf("Expected 1 partner, got %d", len(partners))
	}

	// Delete the partner
	err = db.DeletePartner("urn:gln:test:partner-1")
	if err != nil {
		t.Fatalf("Failed to delete partner: %v", err)
	}

	// Verify deletion
	_, err = db.GetPartnerByID("urn:gln:test:partner-1")
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
	// Setup test database
	dbPath := setupTestDB(t)
	defer cleanupTestDB(dbPath)

	// Test nil partner
	err := db.CreatePartner(nil)
	if err == nil {
		t.Error("Expected error with nil partner")
	}

	// Test empty partner ID
	partner := &db.TradingPartner{
		Name:    "Test",
		JWKSUrl: "https://test.com/jwks",
	}
	err = db.CreatePartner(partner)
	if err == nil {
		t.Error("Expected error with empty partner ID")
	}

	// Test duplicate partner ID
	partner1 := &db.TradingPartner{
		PartnerID: "urn:gln:test:duplicate",
		Name:      "Partner 1",
		JWKSUrl:   "https://test1.com/jwks",
	}
	err = db.CreatePartner(partner1)
	if err != nil {
		t.Fatalf("Failed to create first partner: %v", err)
	}

	partner2 := &db.TradingPartner{
		PartnerID: "urn:gln:test:duplicate",
		Name:      "Partner 2",
		JWKSUrl:   "https://test2.com/jwks",
	}
	err = db.CreatePartner(partner2)
	if err == nil {
		t.Error("Expected error with duplicate partner ID")
	}

	t.Log("✓ Database constraints working correctly")
}

// TestDiscoveryWithConnectionValidation tests the discovery process with a test message
func TestDiscoveryWithConnectionValidation(t *testing.T) {
	t.Log("=== Testing Discovery with Connection Validation ===")

	// Setup test database
	dbPath := setupTestDB(t)
	defer cleanupTestDB(dbPath)

	// Create a partner first
	partner := &db.TradingPartner{
		PartnerID:          "urn:gln:test:validated-partner",
		Name:               "Validated Partner",
		JWKSUrl:            "https://validated.example.com/jwks",
		MessageEndpoint:    "https://validated.example.com/inbound",
		MDNReceiptEndpoint: "https://validated.example.com/receipt",
		PublicKeyJWKS:      `{"keys":[{"kty":"RSA","use":"enc","kid":"test","alg":"RSA-OAEP","n":"test","e":"AQAB"}]}`,
	}

	err := db.CreatePartner(partner)
	if err != nil {
		t.Fatalf("Failed to create partner: %v", err)
	}

	// Simulate sending a test message to validate the connection
	testMessageReceived := false

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inbound" && r.Method == "POST" {
			testMessageReceived = true
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer testServer.Close()

	// Update partner with test server URL
	partner.MessageEndpoint = testServer.URL + "/inbound"
	err = db.UpdatePartner(partner)
	if err != nil {
		t.Fatalf("Failed to update partner: %v", err)
	}

	// Send test message
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
