package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// TransmitRequest represents the request body for /api/v1/transmit
type TransmitRequest struct {
	DestinationPartnerID string                 `json:"destination_partner_id"`
	DocumentType         string                 `json:"document_type"`
	ReceiptWebhook       string                 `json:"receipt_webhook,omitempty"`
	Payload              map[string]interface{} `json:"payload"`
}

// TransmitResponse represents the response for /api/v1/transmit
type TransmitResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// RoutingHeader represents the FideX routing metadata
type RoutingHeader struct {
	FidexVersion   string `json:"fidex_version"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
	ReceiverID     string `json:"receiver_id"`
	DocumentType   string `json:"document_type,omitempty"`
	Timestamp      string `json:"timestamp,omitempty"`
	ReceiptWebhook string `json:"receipt_webhook,omitempty"`
}

// FidexEnvelope represents the incoming FideX envelope structure.
// JSON keys match the canonical protocol spec §4: routing_header + encrypted_payload.
type FidexEnvelope struct {
	Routing RoutingHeader `json:"routing_header"`
	Payload string        `json:"encrypted_payload"` // JWE string
}

// JmdnReceipt represents a message disposition notification receipt
type JmdnReceipt struct {
	OriginalMessageID string  `json:"original_message_id"`
	Status            string  `json:"status"`
	HashVerification  string  `json:"hash_verification,omitempty"`
	Timestamp         string  `json:"timestamp"`
	ErrorLog          *string `json:"error_log,omitempty"`
}

// JmdnEnvelope represents the receipt envelope structure.
// JSON keys match the canonical protocol spec §4: routing_header + encrypted_payload.
type JmdnEnvelope struct {
	Routing RoutingHeader `json:"routing_header"`
	Payload string        `json:"encrypted_payload"` // JWS string
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// Validation functions

// validateTransmitRequest validates the TransmitRequest fields
func validateTransmitRequest(req *TransmitRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if req.DestinationPartnerID == "" {
		return fmt.Errorf("destination_partner_id is required")
	}

	if req.DocumentType == "" {
		return fmt.Errorf("document_type is required")
	}

	if req.Payload == nil || len(req.Payload) == 0 {
		return fmt.Errorf("payload is required and cannot be empty")
	}

	return nil
}

// validateFidexEnvelope validates the FidexEnvelope structure
func validateFidexEnvelope(envelope *FidexEnvelope) error {
	if envelope == nil {
		return fmt.Errorf("envelope cannot be nil")
	}

	if envelope.Routing.FidexVersion == "" {
		return fmt.Errorf("routing_header.fidex_version is required")
	}

	if envelope.Routing.MessageID == "" {
		return fmt.Errorf("routing_header.message_id is required")
	}

	if envelope.Routing.SenderID == "" {
		return fmt.Errorf("routing_header.sender_id is required")
	}

	if envelope.Routing.ReceiverID == "" {
		return fmt.Errorf("routing_header.receiver_id is required")
	}

	if envelope.Payload == "" {
		return fmt.Errorf("encrypted_payload is required")
	}

	return nil
}

// validateJmdnEnvelope validates the JmdnEnvelope structure
func validateJmdnEnvelope(envelope *JmdnEnvelope) error {
	if envelope == nil {
		return fmt.Errorf("envelope cannot be nil")
	}

	if envelope.Routing.MessageID == "" {
		return fmt.Errorf("routing_header.message_id is required")
	}

	if envelope.Payload == "" {
		return fmt.Errorf("encrypted_payload is required")
	}

	return nil
}

// Utility functions

// respondWithError sends a JSON error response
func respondWithError(w http.ResponseWriter, code int, message string, err error) {
	response := ErrorResponse{
		Error: message,
	}

	if err != nil {
		response.Message = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(response)
}
