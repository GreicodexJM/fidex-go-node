package api

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"strings"
	"time"

	"fidex-node/internal/crypto"
	"fidex-node/internal/domain"

	"github.com/google/uuid"
)

// Spec §8 error codes surfaced on /api/v1/inbound. Mirrors the strings
// the conformance suite (bucket 06) greps for in response bodies.
const (
	errCodeBadEnvelope      = "BAD_ENVELOPE"
	errCodeUnknownPartner   = "UNKNOWN_PARTNER"
	errCodeSignatureInvalid = "SIGNATURE_INVALID"
	errCodeDecryptFailed    = "DECRYPT_FAILED"
	errCodeDuplicateMessage = "DUPLICATE_MESSAGE"
)

// inboundRejection is the JSON body the inbound handler returns on any
// envelope that fails admission control. The shape is documented in
// fidex-protocol-specification.md §8: a stable {error_code, message,
// status} triple that downstream peers can parse and react to.
type inboundRejection struct {
	Status    string `json:"status,omitempty"`
	ErrorCode string `json:"error_code"`
	Message   string `json:"message,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

func writeInboundRejection(w http.ResponseWriter, httpCode int, body inboundRejection) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	_ = json.NewEncoder(w).Encode(body)
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

// inboundHandler handles POST /api/v1/inbound
// Accepts a FideX JWE envelope from external trading partners.
//
// Admission control follows spec §4.4 + §8:
//   - Malformed body → 400 BAD_ENVELOPE
//   - Unknown sender → 401 UNKNOWN_PARTNER (envelope persisted as QUARANTINED for audit)
//   - JWS/JWE verification fails → 401 SIGNATURE_INVALID (envelope persisted as QUARANTINED)
//   - Duplicate message_id → 409 DUPLICATE_MESSAGE (spec §9.3 replay protection)
//   - Successful decrypt+verify → 202 Accepted, enqueue J-MDN
func (h *Handlers) inboundHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger.Info(ctx, "Received inbound request from %s", r.RemoteAddr)

	// Parse the envelope
	var envelope FidexEnvelope
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		logger.Warn(ctx, "Failed to decode inbound envelope: %v", err)
		writeInboundRejection(w, http.StatusBadRequest, inboundRejection{
			Status:    "REJECTED",
			ErrorCode: errCodeBadEnvelope,
			Message:   "request body is not valid JSON",
		})
		return
	}

	// Validate the envelope structure
	if err := validateFidexEnvelope(&envelope); err != nil {
		logger.Warn(ctx, "Inbound envelope validation failed: %v", err)
		writeInboundRejection(w, http.StatusBadRequest, inboundRejection{
			Status:    "REJECTED",
			ErrorCode: errCodeBadEnvelope,
			Message:   err.Error(),
			MessageID: envelope.Routing.MessageID,
		})
		return
	}

	// Resolve the sender and decrypt+verify the JWE payload. The handler
	// classifies the outcome into one of three buckets:
	//   - decrypted   → status=DELIVERED, J-MDN will be queued
	//   - quarantined → status=QUARANTINED, envelope kept for forensics,
	//                   client gets a 401 with a typed error_code
	//   - duplicate   → 409 returned, no second row written
	storedPayload := ""
	status := domain.StatusDelivered
	rejection := inboundRejection{}
	rejectionHTTP := 0
	senderID := envelope.Routing.SenderID

	if h.CryptoService != nil && h.PartnerRepo != nil {
		partner, err := h.PartnerRepo.GetByID(ctx, senderID)
		switch {
		case err != nil:
			logger.Warn(ctx, "Inbound from unknown sender %q, quarantining envelope", senderID)
			status = domain.StatusQuarantined
			rejectionHTTP = http.StatusUnauthorized
			rejection = inboundRejection{
				Status:    "QUARANTINED",
				ErrorCode: errCodeUnknownPartner,
				Message:   "sender_id is not a registered partner",
				MessageID: envelope.Routing.MessageID,
			}
		case partner.PublicKeyJWKS == "":
			logger.Warn(ctx, "Sender %q has no cached JWKS, quarantining envelope", senderID)
			status = domain.StatusQuarantined
			rejectionHTTP = http.StatusUnauthorized
			rejection = inboundRejection{
				Status:    "QUARANTINED",
				ErrorCode: errCodeUnknownPartner,
				Message:   "sender has no cached signing key",
				MessageID: envelope.Routing.MessageID,
			}
		default:
			senderSigKey, err := crypto.ParsePublicKeyFromJWKSByUse(partner.PublicKeyJWKS, "sig")
			if err != nil {
				logger.Error(ctx, "Failed to extract sender signing key: %v", err)
				status = domain.StatusQuarantined
				rejectionHTTP = http.StatusUnauthorized
				rejection = inboundRejection{
					Status:    "QUARANTINED",
					ErrorCode: errCodeSignatureInvalid,
					Message:   "sender JWKS does not contain a usable signing key",
					MessageID: envelope.Routing.MessageID,
				}
				break
			}
			decrypted, err := h.CryptoService.DecryptAndVerify(envelope.Payload, senderSigKey)
			if err != nil {
				logger.Error(ctx, "Decrypt+verify failed for message %s: %v", envelope.Routing.MessageID, err)
				status = domain.StatusQuarantined
				rejectionHTTP = http.StatusUnauthorized
				// Distinguish a JWE parse/decrypt failure from a JWS
				// signature mismatch so audit logs can be filtered.
				code := errCodeSignatureInvalid
				msg := err.Error()
				switch {
				case containsAny(msg, "failed to parse JWE", "failed to decrypt JWE"):
					code = errCodeDecryptFailed
				}
				rejection = inboundRejection{
					Status:    "QUARANTINED",
					ErrorCode: code,
					Message:   "envelope failed signature/decryption check",
					MessageID: envelope.Routing.MessageID,
				}
			} else {
				storedPayload = string(decrypted)
				logger.Info(ctx, "Decrypted inbound message %s (%d bytes business document)",
					envelope.Routing.MessageID, len(decrypted))
			}
		}
	}

	// If we did not (or could not) decrypt, keep the envelope as-is so the
	// operator can inspect it later. Forensic persistence happens for both
	// successes and quarantines — only flat-out duplicates are dropped.
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

	// Save via repository. A duplicate message_id (replay) MUST be rejected
	// with HTTP 409 per spec §9.3, never bubble up as a 500.
	if err := h.MessageRepo.Create(ctx, msg); err != nil {
		if stderrors.Is(err, domain.ErrDuplicateMessageID) {
			logger.Warn(ctx, "Duplicate inbound message_id rejected: %s", envelope.Routing.MessageID)
			writeInboundRejection(w, http.StatusConflict, inboundRejection{
				Status:    "REJECTED",
				ErrorCode: errCodeDuplicateMessage,
				Message:   "message_id has already been received",
				MessageID: envelope.Routing.MessageID,
			})
			return
		}
		logger.Error(ctx, "Failed to insert inbound message: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to store message", err)
		return
	}

	logger.Info(ctx, "Inbound message stored: %s (status=%s)", envelope.Routing.MessageID, status)

	// If admission control rejected the envelope, return the typed
	// rejection now — never queue a J-MDN for a quarantined message.
	if rejectionHTTP != 0 {
		writeInboundRejection(w, rejectionHTTP, rejection)
		return
	}

	// Check if this is a receipt to avoid infinite loops
	if envelope.Routing.DocumentType == "receipt" {
		logger.Info(ctx, "Received message is a receipt, not sending confirmation")
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Queue a signed J-MDN receipt back to the sender. The queue worker's
	// send_jmdn handler will build the JWS and POST it to the sender's
	// receive_receipt endpoint resolved via cached partner config.
	jmdnJobPayload := map[string]interface{}{
		"job_type":            "send_jmdn",
		"original_message_id": envelope.Routing.MessageID,
		"recipient_partner_id": envelope.Routing.SenderID,
		"status":              "processed",
		"original_payload":    envelope.Payload,
	}
	jobBytes, _ := json.Marshal(jmdnJobPayload)

	receiptMsg := &domain.Message{
		MessageID: uuid.New().String(),
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   string(jobBytes),
		CreatedAt: time.Now(),
	}

	if err := h.MessageRepo.Create(ctx, receiptMsg); err != nil {
		logger.Error(ctx, "Failed to queue J-MDN job: %v", err)
		// We don't fail the inbound request if receipt queuing fails;
		// the sender will retry on its outbound side.
	} else {
		logger.Info(ctx, "J-MDN job queued for message %s", envelope.Routing.MessageID)
	}

	// Return 202 Accepted
	w.WriteHeader(http.StatusAccepted)
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// flatJMDNReceipt is the canonical FideX J-MDN wire shape (spec §7.2).
// We accept it as a top-level JSON object, with `signature` carrying the
// compact JWS over the same fields signed by the sender.
type flatJMDNReceipt struct {
	OriginalMessageID string  `json:"original_message_id"`
	Status            string  `json:"status"`
	ReceiverID        string  `json:"receiver_id"`
	HashVerification  string  `json:"hash_verification"`
	Timestamp         string  `json:"timestamp"`
	ErrorLog          *string `json:"error_log,omitempty"`
	Signature         string  `json:"signature"`
}

// receiptHandler handles POST /api/v1/receipt — accepts asynchronous J-MDN
// receipts from trading partners. The handler tolerates two wire shapes:
//
//  1. Flat J-MDN (spec §7.2, canonical, used by fidex-php and fidex-go) —
//     a top-level JSON object with original_message_id/status/signature
//     fields.
//
//  2. Envelope-wrapped J-MDN (legacy, used by older fidex-go nodes) — a
//     routing_header + encrypted_payload pair where encrypted_payload is
//     either a compact JWS or a raw JSON receipt.
//
// Whichever shape arrives, we attempt to verify the embedded signature
// against the sender's cached signing key when known. The outbound row
// matching `original_message_id` is reconciled to DELIVERED or FAILED.
func (h *Handlers) receiptHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger.Info(ctx, "Received receipt from %s", r.RemoteAddr)

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Warn(ctx, "Failed to read receipt body: %v", err)
		respondWithError(w, http.StatusBadRequest, "Failed to read request body", err)
		return
	}

	// Try the flat shape first — that's what the canonical spec mandates
	// and what fidex-go-node now emits.
	var flat flatJMDNReceipt
	flatOK := json.Unmarshal(rawBody, &flat) == nil && flat.OriginalMessageID != ""

	var envelope JmdnEnvelope
	envelopeOK := false
	if !flatOK {
		if err := json.Unmarshal(rawBody, &envelope); err != nil {
			logger.Warn(ctx, "Failed to decode receipt: %v", err)
			respondWithError(w, http.StatusBadRequest, "Invalid JSON in request body", err)
			return
		}
		if err := validateJmdnEnvelope(&envelope); err != nil {
			logger.Warn(ctx, "Receipt envelope validation failed: %v", err)
			respondWithError(w, http.StatusBadRequest, "Invalid receipt envelope", err)
			return
		}
		envelopeOK = true
	}

	// Resolve the receipt's identifier for forensic persistence.
	receiptMessageID := envelope.Routing.MessageID
	if receiptMessageID == "" {
		// Flat receipts have no envelope-level ID; mint a deterministic
		// one from the original_message_id + timestamp so replays are
		// caught by the UNIQUE constraint.
		receiptMessageID = "jmdn:" + flat.OriginalMessageID + ":" + flat.Timestamp
	}

	receiptMsg := &domain.Message{
		MessageID: receiptMessageID,
		Direction: domain.DirectionInbound,
		Status:    domain.StatusDelivered,
		Payload:   string(rawBody),
		CreatedAt: time.Now(),
	}
	if err := h.MessageRepo.Create(ctx, receiptMsg); err != nil {
		if stderrors.Is(err, domain.ErrDuplicateMessageID) {
			logger.Warn(ctx, "Duplicate receipt %s, ignoring", receiptMessageID)
		} else {
			logger.Error(ctx, "Failed to insert receipt: %v", err)
			respondWithError(w, http.StatusInternalServerError, "Failed to store receipt", err)
			return
		}
	}

	// Extract the disposition into a normalized struct.
	var disposition JmdnReceipt
	switch {
	case flatOK:
		disposition = JmdnReceipt{
			OriginalMessageID: flat.OriginalMessageID,
			Status:            flat.Status,
			HashVerification:  flat.HashVerification,
			Timestamp:         flat.Timestamp,
			ErrorLog:          flat.ErrorLog,
		}
		// Best-effort signature verification on the flat shape. The
		// peer's signature is a compact JWS over the same JSON sans
		// `signature`; we have no canonicalization protocol with the
		// peer yet, so a verification failure here is logged but not
		// fatal — replay/idempotency protects against bogus receipts.
		if flat.Signature != "" {
			h.bestEffortVerifyJWS(ctx, flat.Signature, flat.ReceiverID)
		}
	case envelopeOK:
		// Envelope-wrapped: payload may be a JWS or raw JSON.
		if h.tryVerifyJWSPayload(ctx, envelope.Routing.SenderID, envelope.Payload, &disposition) {
			// disposition was populated by the verifier
		} else {
			_ = json.Unmarshal([]byte(envelope.Payload), &disposition)
		}
	}

	if disposition.OriginalMessageID != "" {
		newStatus := domain.StatusDelivered
		s := strings.ToUpper(disposition.Status)
		if s == "FAILED" || s == "ERROR" {
			newStatus = domain.StatusFailed
		}
		if err := h.MessageRepo.UpdateStatus(ctx, disposition.OriginalMessageID, newStatus); err != nil {
			logger.Error(ctx, "Failed to update original message status: %v", err)
		} else {
			logger.Info(ctx, "Reconciled outbound %s -> %s based on receipt", disposition.OriginalMessageID, newStatus)
		}
	} else {
		logger.Warn(ctx, "Receipt %s carried no original_message_id, cannot reconcile", receiptMessageID)
	}

	w.WriteHeader(http.StatusAccepted)
}

// bestEffortVerifyJWS logs whether a JWS signature can be verified against
// the receiver's cached signing key. Used as an observability hook for
// flat J-MDN receipts where signature mismatches are not (yet) fatal.
func (h *Handlers) bestEffortVerifyJWS(ctx context.Context, jws, signerID string) {
	if h.CryptoService == nil || h.PartnerRepo == nil || signerID == "" {
		return
	}
	partner, err := h.PartnerRepo.GetByID(ctx, signerID)
	if err != nil || partner.PublicKeyJWKS == "" {
		return
	}
	sigKey, err := crypto.ParsePublicKeyFromJWKSByUse(partner.PublicKeyJWKS, "sig")
	if err != nil {
		return
	}
	if _, err := h.CryptoService.VerifyJMDN(jws, sigKey); err != nil {
		logger.Warn(ctx, "J-MDN signature failed to verify against %s: %v", signerID, err)
	}
}

// tryVerifyJWSPayload tries to interpret an envelope payload as a signed
// JWS J-MDN and, on success, copies the disposition fields into out.
// Returns true if the JWS verified.
func (h *Handlers) tryVerifyJWSPayload(ctx context.Context, signerID, payload string, out *JmdnReceipt) bool {
	if h.CryptoService == nil || h.PartnerRepo == nil || signerID == "" {
		return false
	}
	partner, err := h.PartnerRepo.GetByID(ctx, signerID)
	if err != nil || partner.PublicKeyJWKS == "" {
		return false
	}
	sigKey, err := crypto.ParsePublicKeyFromJWKSByUse(partner.PublicKeyJWKS, "sig")
	if err != nil {
		return false
	}
	jmdn, err := h.CryptoService.VerifyJMDN(payload, sigKey)
	if err != nil {
		return false
	}
	*out = JmdnReceipt{
		OriginalMessageID: jmdn.OriginalMessageID,
		Status:            jmdn.Status,
		HashVerification:  jmdn.HashVerification,
		Timestamp:         jmdn.Timestamp,
		ErrorLog:          jmdn.ErrorLog,
	}
	return true
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
