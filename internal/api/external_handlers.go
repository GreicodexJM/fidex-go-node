package api

import (
	"encoding/json"
	"net/http"
	"time"

	"fidex-node/internal/crypto"
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
func (h *Handlers) inboundHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger.Info(ctx, "Received inbound request from %s", r.RemoteAddr)

	// Parse the envelope
	var envelope FidexEnvelope
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		logger.Warn(ctx, "Failed to decode inbound envelope: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid JSON in request body", err)
		return
	}

	// Validate the envelope structure
	if err := validateFidexEnvelope(&envelope); err != nil {
		logger.Warn(ctx, "Inbound envelope validation failed: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid FideX envelope", err)
		return
	}

	// Resolve the sender and decrypt+verify the JWE payload. If the sender is
	// unknown we still persist the envelope for forensic visibility (audit)
	// but we cannot verify its signature. Spec §5: an unknown sender on
	// /receive is a soft fail — the node returns 202 and quarantines.
	storedPayload := ""
	status := domain.StatusDelivered
	senderID := envelope.Routing.SenderID
	if h.CryptoService != nil && h.PartnerRepo != nil {
		partner, err := h.PartnerRepo.GetByID(ctx, senderID)
		if err != nil {
			logger.Warn(ctx, "Inbound from unknown sender %q, quarantining envelope", senderID)
			status = domain.StatusQuarantined
		} else if partner.PublicKeyJWKS == "" {
			logger.Warn(ctx, "Sender %q has no cached JWKS, quarantining envelope", senderID)
			status = domain.StatusQuarantined
		} else {
			senderSigKey, err := crypto.ParsePublicKeyFromJWKSByUse(partner.PublicKeyJWKS, "sig")
			if err != nil {
				logger.Error(ctx, "Failed to extract sender signing key: %v", err)
				status = domain.StatusQuarantined
			} else {
				decrypted, err := h.CryptoService.DecryptAndVerify(envelope.Payload, senderSigKey)
				if err != nil {
					logger.Error(ctx, "Decrypt+verify failed for message %s: %v", envelope.Routing.MessageID, err)
					status = domain.StatusQuarantined
				} else {
					storedPayload = string(decrypted)
					logger.Info(ctx, "Decrypted inbound message %s (%d bytes business document)",
						envelope.Routing.MessageID, len(decrypted))
				}
			}
		}
	}

	// If we did not (or could not) decrypt, keep the envelope as-is so the
	// operator can inspect it later.
	if storedPayload == "" {
		envelopeBytes, err := json.Marshal(envelope)
		if err != nil {
			logger.Error(ctx, "Failed to marshal envelope: %v", err)
			respondWithError(w, http.StatusInternalServerError, "Failed to process envelope", err)
			return
		}
		storedPayload = string(envelopeBytes)
	}

	// Create the message record
	msg := &domain.Message{
		MessageID: envelope.Routing.MessageID,
		Direction: domain.DirectionInbound,
		Status:    status,
		Payload:   storedPayload,
		CreatedAt: time.Now(),
	}

	// Save via repository
	if err := h.MessageRepo.Create(ctx, msg); err != nil {
		logger.Error(ctx, "Failed to insert inbound message: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to store message", err)
		return
	}

	logger.Info(ctx, "Inbound message stored successfully: %s", envelope.Routing.MessageID)

	// Check if this is a receipt to avoid infinite loops
	if envelope.Routing.DocumentType == "receipt" {
		logger.Info(ctx, "Received message is a receipt, not sending confirmation")
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

	if err := h.MessageRepo.Create(ctx, receiptMsg); err != nil {
		logger.Error(ctx, "Failed to queue receipt: %v", err)
		// We don't fail the request if receipt queuing fails, but we log it
	} else {
		logger.Info(ctx, "Receipt queued for message %s", envelope.Routing.MessageID)
	}

	// Return 202 Accepted
	w.WriteHeader(http.StatusAccepted)
}

// receiptHandler handles POST /api/v1/receipt
// Accepts asynchronous J-MDN receipts from external trading partners
func (h *Handlers) receiptHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger.Info(ctx, "Received receipt from %s", r.RemoteAddr)

	// Parse the receipt envelope
	var envelope JmdnEnvelope
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		logger.Warn(ctx, "Failed to decode receipt envelope: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid JSON in request body", err)
		return
	}

	// Validate the envelope structure
	if err := validateJmdnEnvelope(&envelope); err != nil {
		logger.Warn(ctx, "Receipt envelope validation failed: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid receipt envelope", err)
		return
	}

	// Store the receipt
	// In production, you would verify the JWS signature
	receiptBytes, err := json.Marshal(envelope)
	if err != nil {
		logger.Error(ctx, "Failed to marshal receipt: %v", err)
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

	if err := h.MessageRepo.Create(ctx, msg); err != nil {
		logger.Error(ctx, "Failed to insert receipt: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to store receipt", err)
		return
	}

	logger.Info(ctx, "Receipt stored successfully: %s", envelope.Routing.MessageID)

	// Process the receipt payload to update the original message status
	var receiptPayload JmdnReceipt
	// Try to unmarshal the payload as JmdnReceipt
	if err := json.Unmarshal([]byte(envelope.Payload), &receiptPayload); err == nil {
		// Update the original message status
		if receiptPayload.OriginalMessageID != "" {
			newStatus := domain.StatusDelivered
			if receiptPayload.Status == "error" || receiptPayload.Status == "failed" {
				newStatus = domain.StatusFailed
			}

			if err := h.MessageRepo.UpdateStatus(ctx, receiptPayload.OriginalMessageID, newStatus); err != nil {
				logger.Error(ctx, "Failed to update original message status: %v", err)
			} else {
				logger.Info(ctx, "Updated status of message %s to %s based on receipt", receiptPayload.OriginalMessageID, newStatus)
			}
		}
	} else {
		logger.Warn(ctx, "Failed to parse receipt payload: %v", err)
	}

	// Return 202 Accepted
	w.WriteHeader(http.StatusAccepted)
}

// jwksHandler handles GET /.well-known/jwks.json
// Returns the node's real public key as a JWKS document so partners can
// encrypt messages addressed to this node (spec §5.1).
func (h *Handlers) jwksHandler(w http.ResponseWriter, r *http.Request) {
	logger.Info(r.Context(), "JWKS requested from %s", r.RemoteAddr)

	if h.CryptoService == nil {
		logger.Error(r.Context(), "Crypto service not initialized")
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	jwksJSON, err := h.CryptoService.ExportJWKS(h.Config.NodeID + ":enc")
	if err != nil {
		logger.Error(r.Context(), "Failed to export JWKS: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(jwksJSON))
}
