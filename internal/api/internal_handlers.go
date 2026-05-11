package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"fidex-node/internal/domain"

	"github.com/google/uuid"
)

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
	msg := &domain.Message{
		MessageID: messageID,
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   string(payloadBytes),
		CreatedAt: time.Now(),
	}

	// Save via repository
	if err := MessageRepo.Create(r.Context(), msg); err != nil {
		log.Printf("Failed to insert message: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to queue message", err)
		return
	}

	log.Printf("Message queued successfully: %s", messageID)

	// Return success response
	response := TransmitResponse{
		MessageID: messageID,
		Status:    string(domain.StatusQueued),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}
