package crypto

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	privateKeyPEM, publicKeyPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Verify private key format
	if !strings.Contains(privateKeyPEM, "-----BEGIN RSA PRIVATE KEY-----") {
		t.Error("Private key PEM format invalid")
	}
	if !strings.Contains(privateKeyPEM, "-----END RSA PRIVATE KEY-----") {
		t.Error("Private key PEM format invalid")
	}

	// Verify public key format
	if !strings.Contains(publicKeyPEM, "-----BEGIN PUBLIC KEY-----") {
		t.Error("Public key PEM format invalid")
	}
	if !strings.Contains(publicKeyPEM, "-----END PUBLIC KEY-----") {
		t.Error("Public key PEM format invalid")
	}

	// Verify keys are not empty
	if len(privateKeyPEM) < 100 {
		t.Error("Private key seems too short")
	}
	if len(publicKeyPEM) < 100 {
		t.Error("Public key seems too short")
	}

	t.Logf("Generated private key length: %d bytes", len(privateKeyPEM))
	t.Logf("Generated public key length: %d bytes", len(publicKeyPEM))
}

func TestNewAS5Engine(t *testing.T) {
	// Generate a test key pair
	privateKeyPEM, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create engine
	engine, err := NewAS5Engine(privateKeyPEM, "urn:gln:test:sender")
	if err != nil {
		t.Fatalf("Failed to create AS5 engine: %v", err)
	}

	// Verify engine properties
	if engine.nodeID != "urn:gln:test:sender" {
		t.Errorf("Expected nodeID 'urn:gln:test:sender', got '%s'", engine.nodeID)
	}
	if engine.privateKey == nil {
		t.Error("Private key is nil")
	}
	if engine.publicKey == nil {
		t.Error("Public key is nil")
	}

	t.Log("AS5Engine created successfully")
}

func TestNewAS5EngineInvalidKey(t *testing.T) {
	// Test with invalid PEM
	_, err := NewAS5Engine("invalid-pem", "test-node")
	if err == nil {
		t.Error("Expected error with invalid PEM, got nil")
	}

	// Test with empty string
	_, err = NewAS5Engine("", "test-node")
	if err == nil {
		t.Error("Expected error with empty PEM, got nil")
	}

	t.Log("Invalid key handling works correctly")
}

func TestSignAndEncrypt_DecryptAndVerify(t *testing.T) {
	// Generate keys for sender and receiver
	senderPrivateKeyPEM, senderPublicKeyPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	receiverPrivateKeyPEM, receiverPublicKeyPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate receiver key pair: %v", err)
	}

	// Create engines
	senderEngine, err := NewAS5Engine(senderPrivateKeyPEM, "urn:gln:sender")
	if err != nil {
		t.Fatalf("Failed to create sender engine: %v", err)
	}

	receiverEngine, err := NewAS5Engine(receiverPrivateKeyPEM, "urn:gln:receiver")
	if err != nil {
		t.Fatalf("Failed to create receiver engine: %v", err)
	}

	// Parse public keys
	senderPublicKey, err := ParsePublicKeyFromPEM(senderPublicKeyPEM)
	if err != nil {
		t.Fatalf("Failed to parse sender public key: %v", err)
	}

	receiverPublicKey, err := ParsePublicKeyFromPEM(receiverPublicKeyPEM)
	if err != nil {
		t.Fatalf("Failed to parse receiver public key: %v", err)
	}

	// Test payload
	originalPayload := []byte(`{"orderNumber": "12345", "items": [{"sku": "ABC-001", "quantity": 10}]}`)

	// Sender: Sign and encrypt
	jweCompact, err := senderEngine.SignAndEncrypt(originalPayload, receiverPublicKey)
	if err != nil {
		t.Fatalf("Failed to sign and encrypt: %v", err)
	}

	// Verify JWE format (should have 5 parts separated by dots)
	parts := strings.Split(jweCompact, ".")
	if len(parts) != 5 {
		t.Errorf("Expected JWE with 5 parts, got %d", len(parts))
	}

	t.Logf("JWE length: %d bytes", len(jweCompact))

	// Receiver: Decrypt and verify
	decryptedPayload, err := receiverEngine.DecryptAndVerify(jweCompact, senderPublicKey)
	if err != nil {
		t.Fatalf("Failed to decrypt and verify: %v", err)
	}

	// Verify payload matches
	if string(decryptedPayload) != string(originalPayload) {
		t.Errorf("Decrypted payload doesn't match original.\nExpected: %s\nGot: %s",
			string(originalPayload), string(decryptedPayload))
	}

	t.Log("✓ End-to-end encryption/decryption successful")
}

func TestDecryptAndVerifyWithWrongKey(t *testing.T) {
	// Generate keys
	senderPrivateKeyPEM, _, _ := GenerateKeyPair()
	_, receiverPublicKeyPEM, _ := GenerateKeyPair()
	wrongPrivateKeyPEM, wrongPublicKeyPEM, _ := GenerateKeyPair()

	senderEngine, _ := NewAS5Engine(senderPrivateKeyPEM, "sender")
	wrongEngine, _ := NewAS5Engine(wrongPrivateKeyPEM, "wrong")

	receiverPublicKey, _ := ParsePublicKeyFromPEM(receiverPublicKeyPEM)
	wrongPublicKey, _ := ParsePublicKeyFromPEM(wrongPublicKeyPEM)

	payload := []byte("secret message")

	// Encrypt with receiver's public key
	jweCompact, err := senderEngine.SignAndEncrypt(payload, receiverPublicKey)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Try to decrypt with wrong private key (should fail)
	_, err = wrongEngine.DecryptAndVerify(jweCompact, wrongPublicKey)
	if err == nil {
		t.Error("Expected error when decrypting with wrong key, got nil")
	}

	t.Log("✓ Wrong key detection works correctly")
}

func TestCreateFidexEnvelope(t *testing.T) {
	// Generate keys
	senderPrivateKeyPEM, _, _ := GenerateKeyPair()
	_, receiverPublicKeyPEM, _ := GenerateKeyPair()

	senderEngine, _ := NewAS5Engine(senderPrivateKeyPEM, "urn:gln:0614141000005")
	receiverPublicKey, _ := ParsePublicKeyFromPEM(receiverPublicKeyPEM)

	// Create envelope
	businessDocument := []byte(`{"documentType": "GS1_ORDER", "data": {}}`)
	envelope, err := senderEngine.CreateFidexEnvelope(
		"msg-uuid-12345",
		"urn:gln:0614141000012",
		"GS1_ORDER_JSON",
		"https://sender.com/receipt",
		businessDocument,
		receiverPublicKey,
	)

	if err != nil {
		t.Fatalf("Failed to create FideX envelope: %v", err)
	}

	// Verify routing header
	if envelope.Routing.FidexVersion != "1.0" {
		t.Errorf("Expected FideX version 1.0, got %s", envelope.Routing.FidexVersion)
	}
	if envelope.Routing.MessageID != "msg-uuid-12345" {
		t.Errorf("Expected message ID msg-uuid-12345, got %s", envelope.Routing.MessageID)
	}
	if envelope.Routing.SenderID != "urn:gln:0614141000005" {
		t.Errorf("Expected sender ID urn:gln:0614141000005, got %s", envelope.Routing.SenderID)
	}
	if envelope.Routing.ReceiverID != "urn:gln:0614141000012" {
		t.Errorf("Expected receiver ID urn:gln:0614141000012, got %s", envelope.Routing.ReceiverID)
	}
	if envelope.Routing.DocumentType != "GS1_ORDER_JSON" {
		t.Errorf("Expected document type GS1_ORDER_JSON, got %s", envelope.Routing.DocumentType)
	}
	if envelope.Routing.ReceiptWebhook != "https://sender.com/receipt" {
		t.Errorf("Expected receipt webhook https://sender.com/receipt, got %s", envelope.Routing.ReceiptWebhook)
	}
	if envelope.Routing.Timestamp == "" {
		t.Error("Timestamp is empty")
	}

	// Verify payload is JWE
	if len(envelope.Payload) < 100 {
		t.Error("Payload seems too short to be a valid JWE")
	}

	t.Log("✓ FideX envelope created successfully")
}

func TestProcessInboundEnvelope(t *testing.T) {
	// Generate keys
	senderPrivateKeyPEM, senderPublicKeyPEM, _ := GenerateKeyPair()
	receiverPrivateKeyPEM, receiverPublicKeyPEM, _ := GenerateKeyPair()

	senderEngine, _ := NewAS5Engine(senderPrivateKeyPEM, "sender")
	receiverEngine, _ := NewAS5Engine(receiverPrivateKeyPEM, "receiver")

	senderPublicKey, _ := ParsePublicKeyFromPEM(senderPublicKeyPEM)
	receiverPublicKey, _ := ParsePublicKeyFromPEM(receiverPublicKeyPEM)

	businessDocument := []byte(`{"invoice": "INV-2024-001"}`)

	// Create envelope
	envelope, err := senderEngine.CreateFidexEnvelope(
		"msg-123",
		"receiver-id",
		"INVOICE",
		"",
		businessDocument,
		receiverPublicKey,
	)
	if err != nil {
		t.Fatalf("Failed to create envelope: %v", err)
	}

	// Process inbound envelope
	decryptedDoc, err := receiverEngine.ProcessInboundEnvelope(envelope, senderPublicKey)
	if err != nil {
		t.Fatalf("Failed to process inbound envelope: %v", err)
	}

	// Verify
	if string(decryptedDoc) != string(businessDocument) {
		t.Errorf("Decrypted document doesn't match original")
	}

	t.Log("✓ Inbound envelope processed successfully")
}

func TestCreateJMDN(t *testing.T) {
	privateKeyPEM, _, _ := GenerateKeyPair()
	engine, _ := NewAS5Engine(privateKeyPEM, "receiver-node")

	originalPayload := []byte("original business document")
	errorLog := "No errors"

	// Create J-MDN
	jmdnJWS, err := engine.CreateJMDN("original-msg-id", "DELIVERED", originalPayload, &errorLog)
	if err != nil {
		t.Fatalf("Failed to create J-MDN: %v", err)
	}

	// Verify JWS format (should have 3 parts)
	parts := strings.Split(jmdnJWS, ".")
	if len(parts) != 3 {
		t.Errorf("Expected JWS with 3 parts, got %d", len(parts))
	}

	t.Logf("J-MDN JWS length: %d bytes", len(jmdnJWS))
	t.Log("✓ J-MDN created successfully")
}

func TestVerifyJMDN(t *testing.T) {
	privateKeyPEM, publicKeyPEM, _ := GenerateKeyPair()
	engine, _ := NewAS5Engine(privateKeyPEM, "receiver")
	publicKey, _ := ParsePublicKeyFromPEM(publicKeyPEM)

	originalPayload := []byte("test payload")
	originalMsgID := "msg-uuid-789"

	// Create J-MDN
	jmdnJWS, err := engine.CreateJMDN(originalMsgID, "DELIVERED", originalPayload, nil)
	if err != nil {
		t.Fatalf("Failed to create J-MDN: %v", err)
	}

	// Verify J-MDN
	receipt, err := engine.VerifyJMDN(jmdnJWS, publicKey)
	if err != nil {
		t.Fatalf("Failed to verify J-MDN: %v", err)
	}

	// Check receipt fields
	if receipt.OriginalMessageID != originalMsgID {
		t.Errorf("Expected original message ID %s, got %s", originalMsgID, receipt.OriginalMessageID)
	}
	if receipt.Status != "DELIVERED" {
		t.Errorf("Expected status DELIVERED, got %s", receipt.Status)
	}
	if receipt.HashVerification == "" {
		t.Error("Hash verification is empty")
	}
	if receipt.Timestamp == "" {
		t.Error("Timestamp is empty")
	}

	// Verify hash
	expectedHash := sha256.Sum256(originalPayload)
	expectedHashB64 := base64.StdEncoding.EncodeToString(expectedHash[:])
	if receipt.HashVerification != expectedHashB64 {
		t.Error("Hash verification mismatch")
	}

	t.Log("✓ J-MDN verification successful")
}

func TestVerifyJMDNWithWrongKey(t *testing.T) {
	privateKeyPEM, _, _ := GenerateKeyPair()
	_, wrongPublicKeyPEM, _ := GenerateKeyPair()

	engine, _ := NewAS5Engine(privateKeyPEM, "receiver")
	wrongPublicKey, _ := ParsePublicKeyFromPEM(wrongPublicKeyPEM)

	// Create J-MDN
	jmdnJWS, _ := engine.CreateJMDN("msg-id", "DELIVERED", []byte("data"), nil)

	// Try to verify with wrong key
	_, err := engine.VerifyJMDN(jmdnJWS, wrongPublicKey)
	if err == nil {
		t.Error("Expected error when verifying with wrong key, got nil")
	}

	t.Log("✓ Wrong key detection in J-MDN verification works")
}

func TestExportJWKS(t *testing.T) {
	privateKeyPEM, _, _ := GenerateKeyPair()
	engine, _ := NewAS5Engine(privateKeyPEM, "test-node")

	// Export JWKS
	jwksJSON, err := engine.ExportJWKS("fidex-node-2024-01")
	if err != nil {
		t.Fatalf("Failed to export JWKS: %v", err)
	}

	// Verify JSON structure
	if !strings.Contains(jwksJSON, "\"keys\"") {
		t.Error("JWKS doesn't contain 'keys' field")
	}
	if !strings.Contains(jwksJSON, "\"kty\"") {
		t.Error("JWKS doesn't contain 'kty' field")
	}
	if !strings.Contains(jwksJSON, "\"kid\"") {
		t.Error("JWKS doesn't contain 'kid' field")
	}
	if !strings.Contains(jwksJSON, "fidex-node-2024-01") {
		t.Error("JWKS doesn't contain key ID")
	}

	t.Logf("JWKS JSON:\n%s", jwksJSON)
	t.Log("✓ JWKS export successful")
}

func TestParsePublicKeyFromJWKS(t *testing.T) {
	privateKeyPEM, _, _ := GenerateKeyPair()
	engine, _ := NewAS5Engine(privateKeyPEM, "test")

	// Export JWKS
	jwksJSON, _ := engine.ExportJWKS("test-key")

	// Parse it back
	publicKey, err := ParsePublicKeyFromJWKS(jwksJSON)
	if err != nil {
		t.Fatalf("Failed to parse public key from JWKS: %v", err)
	}

	if publicKey == nil {
		t.Error("Parsed public key is nil")
	}

	// Verify it's the same key by comparing modulus
	if publicKey.N.Cmp(engine.publicKey.N) != 0 {
		t.Error("Parsed public key doesn't match original")
	}

	t.Log("✓ JWKS parsing successful")
}

func TestParsePublicKeyFromPEM(t *testing.T) {
	_, publicKeyPEM, _ := GenerateKeyPair()

	publicKey, err := ParsePublicKeyFromPEM(publicKeyPEM)
	if err != nil {
		t.Fatalf("Failed to parse public key from PEM: %v", err)
	}

	if publicKey == nil {
		t.Error("Parsed public key is nil")
	}

	t.Log("✓ PEM public key parsing successful")
}

func TestEndToEndFidexWorkflow(t *testing.T) {
	t.Log("=== Testing Complete FideX AS5 Workflow ===")

	// 1. Generate keys for both parties
	senderPrivateKeyPEM, senderPublicKeyPEM, _ := GenerateKeyPair()
	receiverPrivateKeyPEM, receiverPublicKeyPEM, _ := GenerateKeyPair()

	// 2. Create engines
	senderEngine, _ := NewAS5Engine(senderPrivateKeyPEM, "urn:gln:sender:12345")
	receiverEngine, _ := NewAS5Engine(receiverPrivateKeyPEM, "urn:gln:receiver:67890")

	// 3. Parse public keys
	senderPublicKey, _ := ParsePublicKeyFromPEM(senderPublicKeyPEM)
	receiverPublicKey, _ := ParsePublicKeyFromPEM(receiverPublicKeyPEM)

	// 4. Sender creates business document
	businessDocument := []byte(`{
		"orderNumber": "PO-2024-001",
		"customer": "ACME Corp",
		"items": [
			{"sku": "WIDGET-001", "quantity": 100, "price": 9.99}
		],
		"total": 999.00
	}`)

	t.Log("Step 1: Sender creates FideX envelope")
	// 5. Sender creates FideX envelope
	envelope, err := senderEngine.CreateFidexEnvelope(
		"msg-uuid-complete-test",
		"urn:gln:receiver:67890",
		"GS1_ORDER_JSON",
		"https://sender.com/fidex/receipt",
		businessDocument,
		receiverPublicKey,
	)
	if err != nil {
		t.Fatalf("Failed to create envelope: %v", err)
	}

	t.Log("Step 2: Receiver processes inbound envelope")
	// 6. Receiver processes inbound envelope
	decryptedDocument, err := receiverEngine.ProcessInboundEnvelope(envelope, senderPublicKey)
	if err != nil {
		t.Fatalf("Failed to process envelope: %v", err)
	}

	// Verify document
	if string(decryptedDocument) != string(businessDocument) {
		t.Fatal("Decrypted document doesn't match original")
	}

	t.Log("Step 3: Receiver creates J-MDN receipt")
	// 7. Receiver creates J-MDN receipt
	jmdnJWS, err := receiverEngine.CreateJMDN(
		envelope.Routing.MessageID,
		"DELIVERED",
		decryptedDocument,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create J-MDN: %v", err)
	}

	t.Log("Step 4: Sender verifies J-MDN receipt")
	// 8. Sender verifies J-MDN receipt
	receipt, err := senderEngine.VerifyJMDN(jmdnJWS, receiverPublicKey)
	if err != nil {
		t.Fatalf("Failed to verify J-MDN: %v", err)
	}

	// Verify receipt
	if receipt.OriginalMessageID != envelope.Routing.MessageID {
		t.Error("Receipt message ID mismatch")
	}
	if receipt.Status != "DELIVERED" {
		t.Errorf("Expected status DELIVERED, got %s", receipt.Status)
	}

	t.Log("=== ✓ Complete FideX AS5 Workflow Successful ===")
	t.Logf("Original document size: %d bytes", len(businessDocument))
	t.Logf("Encrypted envelope size: %d bytes", len(envelope.Payload))
	t.Logf("Compression ratio: %.2f%%", float64(len(envelope.Payload))/float64(len(businessDocument))*100)
}
