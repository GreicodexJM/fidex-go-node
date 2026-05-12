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

// Supported encryption algorithm names exchanged at the wire / AS5 layer.
// These mirror the JWA registry strings (RFC 7518 §4) and are what peers
// publish in their AS5 supported_encryption_algorithms array. Keep these
// in sync with the AS5 advertisement in
// internal/discovery/as5_config.go.
const (
	// AlgRSAOAEP is RSA-OAEP with MGF1+SHA-1 (RFC 7518 §4.2). Default for
	// back-compat with legacy peers that pre-date dual-alg advertisement.
	AlgRSAOAEP = "RSA-OAEP"
	// AlgRSAOAEP256 is RSA-OAEP with MGF1+SHA-256 (RFC 7518 §4.3).
	// Preferred when both peers advertise it (NIST is deprecating SHA-1).
	AlgRSAOAEP256 = "RSA-OAEP-256"
)

// joseKeyAlgorithm maps a wire-level alg string to the go-jose constant
// used by the encrypter. Unknown algs return an error so we never silently
// downgrade to a default that the caller didn't ask for.
func joseKeyAlgorithm(alg string) (jose.KeyAlgorithm, error) {
	switch alg {
	case "", AlgRSAOAEP:
		return jose.RSA_OAEP, nil
	case AlgRSAOAEP256:
		return jose.RSA_OAEP_256, nil
	default:
		return "", fmt.Errorf("unsupported encryption algorithm %q (want %s or %s)",
			alg, AlgRSAOAEP, AlgRSAOAEP256)
	}
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

// FidexEnvelope represents the complete FideX transmission.
// JSON keys match the canonical protocol spec §4: routing_header + encrypted_payload.
type FidexEnvelope struct {
	Routing RoutingHeader `json:"routing_header"`
	Payload string        `json:"encrypted_payload"` // JWE string
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

// SignAndEncrypt performs the complete FideX encryption workflow using the
// default RSA-OAEP (SHA-1) algorithm. Preserved for back-compat with
// callers that pre-date FID-4 / ADR-0003 capability negotiation.
//
// Step 1: Hash and sign the payload with sender's private key (JWS)
// Step 2: Encrypt the JWS with receiver's public key (JWE)
func (e *AS5Engine) SignAndEncrypt(payload []byte, receiverPublicKey *rsa.PublicKey) (string, error) {
	return e.SignAndEncryptWithAlg(payload, receiverPublicKey, AlgRSAOAEP)
}

// SignAndEncryptWithAlg is the alg-aware variant of SignAndEncrypt. It
// drives the JWE key-encryption algorithm from the alg argument so callers
// (e.g. the queue worker) can pick the strongest mutually-supported
// algorithm per FID-4 / ADR-0003 negotiation. Empty or "RSA-OAEP" keeps
// the historical SHA-1 path; "RSA-OAEP-256" switches to SHA-256.
func (e *AS5Engine) SignAndEncryptWithAlg(payload []byte, receiverPublicKey *rsa.PublicKey, alg string) (string, error) {
	keyAlg, err := joseKeyAlgorithm(alg)
	if err != nil {
		return "", err
	}

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

	// Step 2: Encrypt the JWS (create JWE) with the negotiated algorithm.
	encrypter, err := jose.NewEncrypter(
		jose.A256GCM,
		jose.Recipient{
			Algorithm: keyAlg,
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

// NegotiateEncryptionAlgorithm picks the strongest mutually-supported JWE
// key-encryption algorithm between our list and the peer's list, per ADR-0003.
//
// Selection rules (deterministic):
//
//  1. If both peers advertise RSA-OAEP-256, return RSA-OAEP-256.
//  2. Else if both advertise RSA-OAEP, return RSA-OAEP.
//  3. Else if the peer didn't advertise anything (empty list), fall back
//     to RSA-OAEP for back-compat (legacy partners using only the
//     single encryption_algorithm field route through this case).
//  4. Else there is no overlap — return the empty string and a non-nil
//     error so the worker can mark the row FAILED rather than guess.
//
// `ours` is typically the local node's static support list
// ([]string{AlgRSAOAEP, AlgRSAOAEP256}). `theirs` comes from the
// partner's AS5 advertisement, resolved via
// discovery.ResolveSupportedEncryptionAlgorithms.
func NegotiateEncryptionAlgorithm(ours, theirs []string) (string, error) {
	has := func(list []string, want string) bool {
		for _, a := range list {
			if a == want {
				return true
			}
		}
		return false
	}

	if len(theirs) == 0 {
		// Legacy peer with no advertisement: assume RSA-OAEP (the only
		// algorithm the original spec §5 mandates). Verify we actually
		// support it locally before claiming so.
		if has(ours, AlgRSAOAEP) {
			return AlgRSAOAEP, nil
		}
		return "", fmt.Errorf("peer advertised no encryption algorithms and local node does not support %s", AlgRSAOAEP)
	}

	if has(ours, AlgRSAOAEP256) && has(theirs, AlgRSAOAEP256) {
		return AlgRSAOAEP256, nil
	}
	if has(ours, AlgRSAOAEP) && has(theirs, AlgRSAOAEP) {
		return AlgRSAOAEP, nil
	}
	return "", fmt.Errorf("no mutually supported encryption algorithm (ours=%v, theirs=%v)", ours, theirs)
}

// LocalSupportedEncryptionAlgorithms returns the static list of encryption
// algorithms this node can speak on the wire. The order is significant
// only for documentation; NegotiateEncryptionAlgorithm explicitly prefers
// RSA-OAEP-256 regardless of slice ordering.
func LocalSupportedEncryptionAlgorithms() []string {
	return []string{AlgRSAOAEP, AlgRSAOAEP256}
}

// DecryptAndVerify performs the complete FideX decryption workflow
// Step 1: Decrypt the JWE with receiver's private key
// Step 2: Verify the JWS signature with sender's public key
func (e *AS5Engine) DecryptAndVerify(jweCompact string, senderPublicKey *rsa.PublicKey) (payload []byte, err error) {
	// Step 1: Decrypt the JWE. We accept both RSA-OAEP (SHA-1) and
	// RSA-OAEP-256 here — the inbound peer chose the algorithm based on
	// its negotiation; we never refuse a stronger alg just because we
	// only emit the weaker one. See ADR-0003 / FID-4.
	jwe, err := jose.ParseEncrypted(
		jweCompact,
		[]jose.KeyAlgorithm{jose.RSA_OAEP, jose.RSA_OAEP_256},
		[]jose.ContentEncryption{jose.A256GCM},
	)
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

// ExportJWKS exports the node's public key as a JWKS document.
// The same RSA key material is published twice with distinct use/alg
// pairs ("sig" for RS256 signatures, "enc" for RSA-OAEP encryption)
// so peers consuming this JWKS can pick the correct key by `use`.
// keyIDBase is used as a prefix; "-sig" and "-enc" are appended.
func (e *AS5Engine) ExportJWKS(keyIDBase string) (string, error) {
	sigJWK := jose.JSONWebKey{
		Key:       e.publicKey,
		KeyID:     keyIDBase + "-sig",
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}
	encJWK := jose.JSONWebKey{
		Key:       e.publicKey,
		KeyID:     keyIDBase + "-enc",
		Algorithm: string(jose.RSA_OAEP),
		Use:       "enc",
	}

	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{sigJWK, encJWK},
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

// ParsePublicKeyFromJWKS extracts the first RSA public key from a JWKS.
// Used for generic validation; callers that need a specific key (sig vs enc)
// should use ParsePublicKeyFromJWKSByUse.
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

// ParsePublicKeyFromJWKSByUse extracts the first RSA public key with a
// matching `use` field ("sig" or "enc") from a JWKS document. Falls back to
// the first key in the set when no key declares the requested use — useful
// for nodes that publish a single dual-purpose key.
func ParsePublicKeyFromJWKSByUse(jwksJSON, use string) (*rsa.PublicKey, error) {
	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal([]byte(jwksJSON), &jwks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWKS: %w", err)
	}
	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("no keys found in JWKS")
	}

	for _, jwk := range jwks.Keys {
		if jwk.Use == use {
			if pk, ok := jwk.Key.(*rsa.PublicKey); ok {
				return pk, nil
			}
		}
	}
	// Fallback: first RSA key.
	for _, jwk := range jwks.Keys {
		if pk, ok := jwk.Key.(*rsa.PublicKey); ok {
			return pk, nil
		}
	}
	return nil, fmt.Errorf("no RSA public key found in JWKS")
}
