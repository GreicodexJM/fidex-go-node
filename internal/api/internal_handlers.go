package api

import (
	"encoding/json"
	"net/http"
	"time"

	"fidex-node/internal/domain"

	"github.com/google/uuid"
)

// transmitHandler handles POST /api/v1/transmit
// Accepts a raw JSON payload from the local ERP, wraps it in the FideX envelope, and queues it
func (h *Handlers) transmitHandler(w http.ResponseWriter, r *http.Request) {
	logger.Info(r.Context(), "Received transmit request from %s", r.RemoteAddr)

	// Parse the request body
	var req TransmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn(r.Context(), "Failed to decode transmit request: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid JSON in request body", err)
		return
	}

	// Validate the request
	if err := validateTransmitRequest(&req); err != nil {
		logger.Warn(r.Context(), "Transmit request validation failed: %v", err)
		respondWithError(w, http.StatusBadRequest, "Validation failed", err)
		return
	}

	// Generate a new message ID
	messageID := uuid.New().String()

	// Convert the entire request back to JSON to store as payload
	payloadBytes, err := json.Marshal(req)
	if err != nil {
		logger.Error(r.Context(), "Failed to marshal payload: %v", err)
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
	if err := h.MessageRepo.Create(r.Context(), msg); err != nil {
		logger.Error(r.Context(), "Failed to insert message: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to queue message", err)
		return
	}

	logger.Info(r.Context(), "Message queued successfully: %s", messageID)

	// Return success response
	response := TransmitResponse{
		MessageID: messageID,
		Status:    string(domain.StatusQueued),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}
