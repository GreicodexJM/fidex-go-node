package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"fidex-node/internal/db"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
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

// FidexEnvelope represents the incoming FideX envelope structure
type FidexEnvelope struct {
	Routing RoutingHeader `json:"routing"`
	Payload string        `json:"payload"` // JWE string
}

// JmdnReceipt represents a message disposition notification receipt
type JmdnReceipt struct {
	OriginalMessageID string  `json:"original_message_id"`
	Status            string  `json:"status"`
	HashVerification  string  `json:"hash_verification,omitempty"`
	Timestamp         string  `json:"timestamp"`
	ErrorLog          *string `json:"error_log,omitempty"`
}

// JmdnEnvelope represents the receipt envelope structure
type JmdnEnvelope struct {
	Routing RoutingHeader `json:"routing"`
	Payload string        `json:"payload"` // JWS string
}

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// SetupInternalRouter configures the internal API routes (ERP-facing)
// This router should be bound to localhost only and protected with IP allowlist + API key
func SetupInternalRouter(allowedIPs []string, apiKey string, enableIPAllowlist bool) *chi.Mux {
	r := chi.NewRouter()

	// Common middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))

	// Security middleware
	if enableIPAllowlist && len(allowedIPs) > 0 {
		r.Use(IPAllowlistMiddleware(allowedIPs))
	}
	if apiKey != "" {
		r.Use(APIKeyMiddleware(apiKey))
	}

	// Internal API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/transmit", transmitHandler)
	})

	return r
}

// SetupPublicRouter configures the public API routes (B2B-facing)
// This router should be accessible from external networks
func SetupPublicRouter() *chi.Mux {
	r := chi.NewRouter()

	// Common middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Timeout(60 * time.Second))

	// Health check
	r.Get("/health", healthHandler)

	// Public B2B API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/inbound", inboundHandler)
		r.Post("/receipt", receiptHandler)
	})

	// JWKS discovery endpoint
	r.Get("/.well-known/jwks.json", jwksHandler)

	return r
}

// healthHandler provides a simple health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"node":   "fidex-edge",
	})
}

// transmitHandler handles POST /api/v1/transmit
// Accepts a raw JSON payload from the local ERP, wraps it in the FideX envelope, and queues it
func transmitHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received transmit request from %s", r.RemoteAddr)

	// Parse the request body
	var req TransmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Failed to decode transmit request: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid JSON in request body", err)
		return
	}

	// Validate the request
	if err := validateTransmitRequest(&req); err != nil {
		log.Printf("Transmit request validation failed: %v", err)
		respondWithError(w, http.StatusBadRequest, "Validation failed", err)
		return
	}

	// Generate a new message ID
	messageID := uuid.New().String()

	// Convert the entire request back to JSON to store as payload
	payloadBytes, err := json.Marshal(req)
	if err != nil {
		log.Printf("Failed to marshal payload: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to process payload", err)
		return
	}

	// Create the message record
	msg := &db.Message{
		MessageID: messageID,
		Direction: db.DirectionOutbound,
		Status:    db.StatusQueued,
		Payload:   string(payloadBytes),
		CreatedAt: time.Now(),
	}

	// Save to database
	if err := db.InsertMessage(msg); err != nil {
		log.Printf("Failed to insert message: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to queue message", err)
		return
	}

	log.Printf("Message queued successfully: %s", messageID)

	// Return success response
	response := TransmitResponse{
		MessageID: messageID,
		Status:    string(db.StatusQueued),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}

// inboundHandler handles POST /api/v1/inbound
// Accepts a FideX JWE envelope from external trading partners
func inboundHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received inbound request from %s", r.RemoteAddr)

	// Parse the envelope
	var envelope FidexEnvelope
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		log.Printf("Failed to decode inbound envelope: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid JSON in request body", err)
		return
	}

	// Validate the envelope structure
	if err := validateFidexEnvelope(&envelope); err != nil {
		log.Printf("Inbound envelope validation failed: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid FideX envelope", err)
		return
	}

	// For now, we accept the envelope and store it as-is
	// In production, you would decrypt the JWE payload here
	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		log.Printf("Failed to marshal envelope: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to process envelope", err)
		return
	}

	// Create the message record
	msg := &db.Message{
		MessageID: envelope.Routing.MessageID,
		Direction: db.DirectionInbound,
		Status:    db.StatusDelivered,
		Payload:   string(envelopeBytes),
		CreatedAt: time.Now(),
	}

	// Save to database
	if err := db.InsertMessage(msg); err != nil {
		log.Printf("Failed to insert inbound message: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to store message", err)
		return
	}

	log.Printf("Inbound message stored successfully: %s", envelope.Routing.MessageID)

	// Return 202 Accepted
	w.WriteHeader(http.StatusAccepted)
}

// receiptHandler handles POST /api/v1/receipt
// Accepts asynchronous J-MDN receipts from external trading partners
func receiptHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received receipt from %s", r.RemoteAddr)

	// Parse the receipt envelope
	var envelope JmdnEnvelope
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		log.Printf("Failed to decode receipt envelope: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid JSON in request body", err)
		return
	}

	// Validate the envelope structure
	if err := validateJmdnEnvelope(&envelope); err != nil {
		log.Printf("Receipt envelope validation failed: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid receipt envelope", err)
		return
	}

	// Store the receipt
	// In production, you would verify the JWS signature and update the original message status
	receiptBytes, err := json.Marshal(envelope)
	if err != nil {
		log.Printf("Failed to marshal receipt: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to process receipt", err)
		return
	}

	// Create a new message record for the receipt
	msg := &db.Message{
		MessageID: envelope.Routing.MessageID,
		Direction: db.DirectionInbound,
		Status:    db.StatusDelivered,
		Payload:   string(receiptBytes),
		CreatedAt: time.Now(),
	}

	if err := db.InsertMessage(msg); err != nil {
		log.Printf("Failed to insert receipt: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to store receipt", err)
		return
	}

	log.Printf("Receipt stored successfully: %s", envelope.Routing.MessageID)

	// Return 202 Accepted
	w.WriteHeader(http.StatusAccepted)
}

// jwksHandler handles GET /.well-known/jwks.json
// Returns a mock JSON Web Key Set for public key discovery
func jwksHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("JWKS requested from %s", r.RemoteAddr)

	// Mock JWKS response
	// In production, this would contain the actual public keys from the node's configuration
	jwks := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "enc",
				"kid": "fidex-node-2024-01",
				"alg": "RSA-OAEP",
				"n":   "mock_modulus_base64url_encoded",
				"e":   "AQAB",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(jwks)
}

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
		return fmt.Errorf("routing.fidex_version is required")
	}

	if envelope.Routing.MessageID == "" {
		return fmt.Errorf("routing.message_id is required")
	}

	if envelope.Routing.SenderID == "" {
		return fmt.Errorf("routing.sender_id is required")
	}

	if envelope.Routing.ReceiverID == "" {
		return fmt.Errorf("routing.receiver_id is required")
	}

	if envelope.Payload == "" {
		return fmt.Errorf("payload is required")
	}

	return nil
}

// validateJmdnEnvelope validates the JmdnEnvelope structure
func validateJmdnEnvelope(envelope *JmdnEnvelope) error {
	if envelope == nil {
		return fmt.Errorf("envelope cannot be nil")
	}

	if envelope.Routing.MessageID == "" {
		return fmt.Errorf("routing.message_id is required")
	}

	if envelope.Payload == "" {
		return fmt.Errorf("payload is required")
	}

	return nil
}

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
