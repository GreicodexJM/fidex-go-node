package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// AS5Engine handles all FideX Protocol cryptographic operations
type AS5Engine struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	nodeID     string
}

// RoutingHeader represents the FideX routing metadata
type RoutingHeader struct {
	FidexVersion   string `json:"fidex_version"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
	ReceiverID     string `json:"receiver_id"`
	DocumentType   string `json:"document_type,omitempty"`
	Timestamp      string `json:"timestamp"`
	ReceiptWebhook string `json:"receipt_webhook,omitempty"`
}

// FidexEnvelope represents the complete FideX transmission
type FidexEnvelope struct {
	Routing RoutingHeader `json:"routing"`
	Payload string        `json:"payload"` // JWE string
}

// JMDNReceipt represents a message disposition notification
type JMDNReceipt struct {
	OriginalMessageID string  `json:"original_message_id"`
	Status            string  `json:"status"` // DELIVERED or FAILED
	HashVerification  string  `json:"hash_verification"`
	Timestamp         string  `json:"timestamp"`
	ErrorLog          *string `json:"error_log,omitempty"`
}

// NewAS5Engine creates a new crypto engine instance
func NewAS5Engine(privateKeyPEM, nodeID string) (*AS5Engine, error) {
	// Parse the private key
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		privateKey, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("key is not RSA private key")
		}
	}

	return &AS5Engine{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		nodeID:     nodeID,
	}, nil
}

// GenerateKeyPair generates a new RSA key pair for the node
func GenerateKeyPair() (privateKeyPEM, publicKeyPEM string, err error) {
	// Generate 2048-bit RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate key: %w", err)
	}

	// Encode private key to PEM
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}))

	// Encode public key to PEM
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal public key: %w", err)
	}
	publicKeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}))

	return privateKeyPEM, publicKeyPEM, nil
}

// SignAndEncrypt performs the complete FideX encryption workflow
// Step 1: Hash and sign the payload with sender's private key (JWS)
// Step 2: Encrypt the JWS with receiver's public key (JWE)
func (e *AS5Engine) SignAndEncrypt(payload []byte, receiverPublicKey *rsa.PublicKey) (string, error) {
	// Step 1: Sign the payload (create JWS)
	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       e.privateKey,
		},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create signer: %w", err)
	}

	// Create JWS with the payload
	jws, err := signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("failed to sign payload: %w", err)
	}

	// Serialize the JWS
	jwsCompact, err := jws.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize JWS: %w", err)
	}

	// Step 2: Encrypt the JWS (create JWE)
	encrypter, err := jose.NewEncrypter(
		jose.A256GCM,
		jose.Recipient{
			Algorithm: jose.RSA_OAEP,
			Key:       receiverPublicKey,
		},
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create encrypter: %w", err)
	}

	// Encrypt the JWS
	jwe, err := encrypter.Encrypt([]byte(jwsCompact))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt JWS: %w", err)
	}

	// Serialize the JWE
	jweCompact, err := jwe.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize JWE: %w", err)
	}

	return jweCompact, nil
}

// DecryptAndVerify performs the complete FideX decryption workflow
// Step 1: Decrypt the JWE with receiver's private key
// Step 2: Verify the JWS signature with sender's public key
func (e *AS5Engine) DecryptAndVerify(jweCompact string, senderPublicKey *rsa.PublicKey) (payload []byte, err error) {
	// Step 1: Decrypt the JWE
	jwe, err := jose.ParseEncrypted(jweCompact, []jose.KeyAlgorithm{jose.RSA_OAEP}, []jose.ContentEncryption{jose.A256GCM})
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWE: %w", err)
	}

	// Decrypt using our private key
	jwsBytes, err := jwe.Decrypt(e.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt JWE: %w", err)
	}

	// Step 2: Verify the JWS signature
	jws, err := jose.ParseSigned(string(jwsBytes), []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWS: %w", err)
	}

	// Verify the signature with sender's public key
	payload, err = jws.Verify(senderPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to verify JWS signature: %w", err)
	}

	return payload, nil
}

// CreateFidexEnvelope creates a complete FideX envelope
func (e *AS5Engine) CreateFidexEnvelope(
	messageID, receiverID, documentType string,
	receiptWebhook string,
	businessDocument []byte,
	receiverPublicKey *rsa.PublicKey,
) (*FidexEnvelope, error) {
	// Create routing header
	routing := RoutingHeader{
		FidexVersion:   "1.0",
		MessageID:      messageID,
		SenderID:       e.nodeID,
		ReceiverID:     receiverID,
		DocumentType:   documentType,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		ReceiptWebhook: receiptWebhook,
	}

	// Sign and encrypt the business document
	jwePayload, err := e.SignAndEncrypt(businessDocument, receiverPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt payload: %w", err)
	}

	return &FidexEnvelope{
		Routing: routing,
		Payload: jwePayload,
	}, nil
}

// ProcessInboundEnvelope decrypts and verifies an inbound FideX envelope
func (e *AS5Engine) ProcessInboundEnvelope(envelope *FidexEnvelope, senderPublicKey *rsa.PublicKey) ([]byte, error) {
	// Decrypt and verify the payload
	businessDocument, err := e.DecryptAndVerify(envelope.Payload, senderPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt envelope: %w", err)
	}

	return businessDocument, nil
}

// CreateJMDN creates a signed J-MDN receipt
func (e *AS5Engine) CreateJMDN(originalMessageID string, status string, originalPayload []byte, errorLog *string) (string, error) {
	// Calculate hash of original payload
	hash := sha256.Sum256(originalPayload)
	hashVerification := base64.StdEncoding.EncodeToString(hash[:])

	// Create J-MDN receipt
	receipt := JMDNReceipt{
		OriginalMessageID: originalMessageID,
		Status:            status,
		HashVerification:  hashVerification,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		ErrorLog:          errorLog,
	}

	// Marshal to JSON
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return "", fmt.Errorf("failed to marshal J-MDN: %w", err)
	}

	// Sign the receipt (JWS)
	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       e.privateKey,
		},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create signer: %w", err)
	}

	jws, err := signer.Sign(receiptJSON)
	if err != nil {
		return "", fmt.Errorf("failed to sign J-MDN: %w", err)
	}

	// Serialize to compact form
	jwsCompact, err := jws.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize J-MDN: %w", err)
	}

	return jwsCompact, nil
}

// VerifyJMDN verifies a J-MDN receipt signature
func (e *AS5Engine) VerifyJMDN(jwsCompact string, senderPublicKey *rsa.PublicKey) (*JMDNReceipt, error) {
	// Parse the JWS
	jws, err := jose.ParseSigned(jwsCompact, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, fmt.Errorf("failed to parse J-MDN JWS: %w", err)
	}

	// Verify signature
	receiptJSON, err := jws.Verify(senderPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to verify J-MDN signature: %w", err)
	}

	// Unmarshal the receipt
	var receipt JMDNReceipt
	if err := json.Unmarshal(receiptJSON, &receipt); err != nil {
		return nil, fmt.Errorf("failed to unmarshal J-MDN: %w", err)
	}

	return &receipt, nil
}

// ExportJWKS exports the public key as a JWKS
func (e *AS5Engine) ExportJWKS(keyID string) (string, error) {
	jwk := jose.JSONWebKey{
		Key:       e.publicKey,
		KeyID:     keyID,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}

	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{jwk},
	}

	jwksJSON, err := json.MarshalIndent(jwks, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWKS: %w", err)
	}

	return string(jwksJSON), nil
}

// ParsePublicKeyFromPEM parses a public key from PEM format
func ParsePublicKeyFromPEM(publicKeyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	publicKey, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA public key")
	}

	return publicKey, nil
}

// ParsePublicKeyFromJWKS extracts the first RSA public key from a JWKS
func ParsePublicKeyFromJWKS(jwksJSON string) (*rsa.PublicKey, error) {
	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal([]byte(jwksJSON), &jwks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWKS: %w", err)
	}

	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("no keys found in JWKS")
	}

	// Get the first key
	jwk := jwks.Keys[0]
	publicKey, ok := jwk.Key.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA public key")
	}

	return publicKey, nil
}
