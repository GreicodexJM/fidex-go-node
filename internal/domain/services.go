package domain

import (
	"crypto/rsa"
)

// CryptoService defines the interface for encryption and signing operations
type CryptoService interface {
	// SignAndEncrypt encrypts payload with receiver's public key and signs with own private key
	// Returns JWE compact serialization
	SignAndEncrypt(payload []byte, receiverPublicKey *rsa.PublicKey) (string, error)

	// DecryptAndVerify decrypts JWE with own private key and verifies signature with sender's public key
	// Returns decrypted payload
	DecryptAndVerify(jweCompact string, senderPublicKey *rsa.PublicKey) ([]byte, error)

	// CreateJMDN creates a signed J-MDN (Message Disposition Notification) receipt
	// Returns JWS compact serialization
	CreateJMDN(originalMessageID string, status string, originalPayload []byte, errorLog *string) (string, error)

	// VerifyJMDN verifies and parses a J-MDN receipt
	VerifyJMDN(jwsCompact string, senderPublicKey *rsa.PublicKey) (*JMDNReceipt, error)

	// ExportJWKS exports the node's public key as JWKS
	ExportJWKS(keyID string) (string, error)

	// GetPublicKey returns the node's public key
	GetPublicKey() *rsa.PublicKey
}

// JMDNReceipt represents a parsed J-MDN receipt
type JMDNReceipt struct {
	OriginalMessageID string  `json:"original_message_id"`
	Status            string  `json:"status"`
	Timestamp         string  `json:"timestamp"`
	ErrorLog          *string `json:"error_log,omitempty"`
}

// RoutingHeader represents the routing metadata for a message
type RoutingHeader struct {
	FidexVersion string `json:"fidex_version"`
	MessageID    string `json:"message_id"`
	SenderID     string `json:"sender_id"`
	ReceiverID   string `json:"receiver_id"`
	DocumentType string `json:"document_type"`
	Timestamp    string `json:"timestamp"`
}

// FidexEnvelope represents a complete FideX AS5 message.
// JSON keys match the canonical protocol spec §4: routing_header + encrypted_payload.
type FidexEnvelope struct {
	Routing RoutingHeader `json:"routing_header"`
	Payload string        `json:"encrypted_payload"` // JWE encrypted payload
}
