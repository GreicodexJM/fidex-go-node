package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"fidex-node/internal/domain"

	"github.com/google/uuid"
)

// healthHandler provides a simple health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"node":   "fidex-edge",
	})
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
	msg := &domain.Message{
		MessageID: envelope.Routing.MessageID,
		Direction: domain.DirectionInbound,
		Status:    domain.StatusDelivered,
		Payload:   string(envelopeBytes),
		CreatedAt: time.Now(),
	}

	// Save via repository
	if err := MessageRepo.Create(r.Context(), msg); err != nil {
		log.Printf("Failed to insert inbound message: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to store message", err)
		return
	}

	log.Printf("Inbound message stored successfully: %s", envelope.Routing.MessageID)

	// Check if this is a receipt to avoid infinite loops
	if envelope.Routing.DocumentType == "receipt" {
		log.Printf("Received message is a receipt, not sending confirmation")
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Queue an asynchronous receipt (MDN)
	// In a real implementation, we would sign this receipt
	receipt := JmdnReceipt{
		OriginalMessageID: envelope.Routing.MessageID,
		Status:            "processed",
		Timestamp:         time.Now().Format(time.RFC3339),
	}

	// Create receipt envelope
	receiptEnvelope := JmdnEnvelope{
		Routing: RoutingHeader{
			FidexVersion: "1.0",
			MessageID:    uuid.New().String(),
			SenderID:     envelope.Routing.ReceiverID, // We are the sender of the receipt
			ReceiverID:   envelope.Routing.SenderID,   // Original sender is the receiver
			DocumentType: "receipt",
			Timestamp:    time.Now().Format(time.RFC3339),
		},
		// In production, Payload would be a JWS. Here we just JSON stringify the receipt
	}

	receiptJSON, _ := json.Marshal(receipt)
	receiptEnvelope.Payload = string(receiptJSON)

	// Create outbound message for the receipt
	// We need to wrap the receipt envelope in a structure that the queue worker understands
	// The queue worker expects a TransmitRequest-like payload structure to extract destination

	receiptWorkerPayload := map[string]interface{}{
		"destination_partner_id": envelope.Routing.SenderID,
		"document_type":          "receipt",
		"payload":                receiptEnvelope, // The actual content to send
	}

	workerPayloadBytes, _ := json.Marshal(receiptWorkerPayload)

	receiptMsg := &domain.Message{
		MessageID: receiptEnvelope.Routing.MessageID,
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   string(workerPayloadBytes),
		CreatedAt: time.Now(),
	}

	if err := MessageRepo.Create(r.Context(), receiptMsg); err != nil {
		log.Printf("Failed to queue receipt: %v", err)
		// We don't fail the request if receipt queuing fails, but we log it
	} else {
		log.Printf("Receipt queued for message %s", envelope.Routing.MessageID)
	}

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
	// In production, you would verify the JWS signature
	receiptBytes, err := json.Marshal(envelope)
	if err != nil {
		log.Printf("Failed to marshal receipt: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to process receipt", err)
		return
	}

	// Create a new message record for the receipt
	msg := &domain.Message{
		MessageID: envelope.Routing.MessageID,
		Direction: domain.DirectionInbound,
		Status:    domain.StatusDelivered,
		Payload:   string(receiptBytes),
		CreatedAt: time.Now(),
	}

	if err := MessageRepo.Create(r.Context(), msg); err != nil {
		log.Printf("Failed to insert receipt: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to store receipt", err)
		return
	}

	log.Printf("Receipt stored successfully: %s", envelope.Routing.MessageID)

	// Process the receipt payload to update the original message status
	var receiptPayload JmdnReceipt
	// The payload in the envelope is a JWS string, but for this implementation we assume it might be raw JSON or we need to parse it
	// In a real AS5 implementation, we would verify the JWS signature and extract the payload
	// For this mock implementation, we'll assume the payload field contains the JSON string of JmdnReceipt directly if it's not a JWS
	// Or we try to unmarshal it directly if it's just JSON

	// NOTE: In the current mock implementation of inbound/outbound, we are storing raw JSON in Payload for simplicity
	// If envelope.Payload is a JWS string, we would need to decode it.
	// Let's assume for now it's a JSON string for simplicity of this task, or try to decode it.

	// Try to unmarshal the payload as JmdnReceipt
	if err := json.Unmarshal([]byte(envelope.Payload), &receiptPayload); err == nil {
		// Update the original message status
		if receiptPayload.OriginalMessageID != "" {
			newStatus := domain.StatusDelivered
			if receiptPayload.Status == "error" || receiptPayload.Status == "failed" {
				newStatus = domain.StatusFailed
			}

			if err := MessageRepo.UpdateStatus(r.Context(), receiptPayload.OriginalMessageID, newStatus); err != nil {
				log.Printf("Failed to update original message status: %v", err)
			} else {
				log.Printf("Updated status of message %s to %s based on receipt", receiptPayload.OriginalMessageID, newStatus)
			}
		}
	} else {
		log.Printf("Failed to parse receipt payload: %v", err)
	}

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
