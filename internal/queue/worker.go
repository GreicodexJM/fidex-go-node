package queue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/crypto"
	"fidex-node/internal/domain"
	"fidex-node/internal/errors"
	"fidex-node/internal/logging"
)

// logger is the package-level structured logger for the queue worker.
var logger = logging.New("queue")

// Worker represents the message queue worker responsible for delivering outbound messages
type Worker struct {
	messageRepo  domain.MessageRepository
	partnerRepo  domain.PartnerRepository
	cryptoEngine *crypto.AS5Engine
	httpClient   *http.Client
	stopChan     chan struct{}
	config       WorkerConfig
	// nodeID is this node's own URN, stamped as sender_id on every outbound
	// FideX envelope. Defaults to empty (set via SetNodeID by the container).
	nodeID string
}

// SetNodeID configures the local node URN used as sender_id when building
// outbound FideX envelopes. Must be called once at startup before Start().
func (w *Worker) SetNodeID(nodeID string) { w.nodeID = nodeID }

// WorkerConfig holds worker configuration
type WorkerConfig struct {
	PollInterval   time.Duration
	MaxRetries     int
	RequestTimeout time.Duration
}

// DefaultWorkerConfig returns the default worker configuration
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		PollInterval:   10 * time.Second,
		MaxRetries:     5,
		RequestTimeout: 30 * time.Second,
	}
}

// NewWorker creates a new queue worker
func NewWorker(
	messageRepo domain.MessageRepository,
	partnerRepo domain.PartnerRepository,
	cryptoEngine *crypto.AS5Engine,
) *Worker {
	return &Worker{
		messageRepo:  messageRepo,
		partnerRepo:  partnerRepo,
		cryptoEngine: cryptoEngine,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		stopChan: make(chan struct{}),
		config:   DefaultWorkerConfig(),
	}
}

// NewWorkerWithConfig creates a worker with custom configuration
func NewWorkerWithConfig(
	messageRepo domain.MessageRepository,
	partnerRepo domain.PartnerRepository,
	cryptoEngine *crypto.AS5Engine,
	config WorkerConfig,
) *Worker {
	return &Worker{
		messageRepo:  messageRepo,
		partnerRepo:  partnerRepo,
		cryptoEngine: cryptoEngine,
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
		},
		stopChan: make(chan struct{}),
		config:   config,
	}
}

// Start starts the worker loop
func (w *Worker) Start() {
	go func() {
		ctx := context.Background()
		logger.Info(ctx, "Queue worker started")
		ticker := time.NewTicker(w.config.PollInterval)
		defer ticker.Stop()

		// Process queue immediately on start
		w.processQueue()

		for {
			select {
			case <-ticker.C:
				w.processQueue()
			case <-w.stopChan:
				logger.Info(ctx, "Queue worker stopped")
				return
			}
		}
	}()
}

// Stop stops the worker gracefully
func (w *Worker) Stop() {
	close(w.stopChan)
}

// processQueue processes all queued messages
func (w *Worker) processQueue() {
	ctx := context.Background()

	// Get all queued messages
	messages, err := w.messageRepo.ListByStatus(ctx, domain.StatusQueued)
	if err != nil {
		logger.Error(ctx, "Failed to get queued messages: %v", err)
		return
	}

	if len(messages) == 0 {
		return
	}

	logger.Info(ctx, "Processing %d queued message(s)", len(messages))

	for _, msg := range messages {
		// Skip if not time to retry yet
		if msg.NextRetryAt != nil && msg.NextRetryAt.After(time.Now()) {
			continue
		}

		// Process message with timeout
		if err := w.deliverMessage(ctx, msg); err != nil {
			w.handleFailure(ctx, msg, err)
		} else {
			w.handleSuccess(ctx, msg)
		}
	}
}

// queuedOutboundPayload is the JSON shape this worker expects for regular
// business-document outbound rows. It matches api.TransmitRequest. The
// worker is the place where business documents become signed-and-encrypted
// FideX envelopes.
type queuedOutboundPayload struct {
	DestinationPartnerID string          `json:"destination_partner_id"`
	DocumentType         string          `json:"document_type"`
	ReceiptWebhook       string          `json:"receipt_webhook,omitempty"`
	Payload              json.RawMessage `json:"payload"`
}

// queuedJMDNJob is the JSON shape an inbound handler writes when it wants
// the worker to emit a signed J-MDN receipt back to the original sender.
// Spec §7: receipts are JWS-signed disposition notifications addressed to
// the sender's receive_receipt endpoint.
type queuedJMDNJob struct {
	JobType            string `json:"job_type"`
	OriginalMessageID  string `json:"original_message_id"`
	RecipientPartnerID string `json:"recipient_partner_id"`
	Status             string `json:"status"`
	OriginalPayload    string `json:"original_payload"`
}

// jmdnReceiptBody is the JSON wire shape this worker POSTs to a partner's
// receive_receipt endpoint. The structure matches the receipt schema
// expected by the reference peer (fidex-php) and the canonical J-MDN
// definition in spec §7.2:
//
//	original_message_id  — the message_id the receipt acknowledges
//	status               — DELIVERED | FAILED (uppercase, spec §7.3)
//	receiver_id          — URN of the receipt's signer (us, the NUT)
//	hash_verification    — "sha256:" + lowercase-hex SHA-256 of the
//	                       original encrypted_payload (anti-tamper)
//	timestamp            — RFC3339 with millisecond precision
//	signature            — compact JWS (alg=RS256) over the same fields,
//	                       signed by the NUT's private signing key
type jmdnReceiptBody struct {
	OriginalMessageID string `json:"original_message_id"`
	Status            string `json:"status"`
	ReceiverID        string `json:"receiver_id"`
	HashVerification  string `json:"hash_verification"`
	Timestamp         string `json:"timestamp"`
	ErrorLog          string `json:"error_log,omitempty"`
	Signature         string `json:"signature"`
}

// deliverMessage dispatches a queued row to the right delivery path based
// on its `job_type` column.
//
// Per ADR-0002 the column is the canonical discriminator; the JSON
// payload still carries `job_type` redundantly for cross-system
// portability, but the dispatcher must not depend on it. For rows that
// somehow reach this method with an empty job_type column (older DBs
// against which the boot-time backfill has not yet run, or in-memory
// fixtures from tests that bypass the repository), we fall back to the
// payload's `job_type` and finally to JobTypeProcessOutbound — the
// behaviour that predates the column.
func (w *Worker) deliverMessage(ctx context.Context, msg *domain.Message) error {
	jobType := msg.JobType
	if jobType == "" {
		var probe struct {
			JobType string `json:"job_type"`
		}
		_ = json.Unmarshal([]byte(msg.Payload), &probe)
		jobType = probe.JobType
		if jobType == "" {
			jobType = constants.JobTypeProcessOutbound
		}
	}

	switch jobType {
	case constants.JobTypeSendJMDN:
		return w.deliverJMDN(ctx, msg)
	case constants.JobTypeProcessOutbound:
		return w.deliverBusinessDocument(ctx, msg)
	default:
		// Unknown job_type: per ADR-0002 these belong in a dead-letter
		// status rather than silently falling through to the business-
		// document path. Surface as a validation error so the standard
		// failure handler (non-retryable) marks the row FAILED.
		return errors.Validation(fmt.Sprintf("unknown job_type %q", jobType))
	}
}

// deliverBusinessDocument is the original outbound delivery path: parse the
// TransmitRequest-shaped payload, sign+encrypt the business document, POST
// it as a FideX envelope to the partner's message endpoint.
func (w *Worker) deliverBusinessDocument(ctx context.Context, msg *domain.Message) error {
	// 1. Parse the queued payload (TransmitRequest-shaped).
	var queued queuedOutboundPayload
	if err := json.Unmarshal([]byte(msg.Payload), &queued); err != nil {
		return errors.Validation("invalid queued payload format")
	}
	if queued.DestinationPartnerID == "" {
		return errors.Validation("destination_partner_id is empty")
	}
	if len(queued.Payload) == 0 {
		return errors.Validation("payload (business document) is empty")
	}

	// 2. Resolve the destination partner + their published endpoint.
	partner, err := w.partnerRepo.GetByID(ctx, queued.DestinationPartnerID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodePartnerNotFound,
			fmt.Sprintf("partner %s not found", queued.DestinationPartnerID))
	}
	if partner.MessageEndpoint == "" {
		return errors.Validation("partner has no message endpoint configured")
	}
	if partner.PublicKeyJWKS == "" {
		return errors.Validation("partner has no cached JWKS — cannot encrypt envelope")
	}

	// 3. Extract the partner's encryption public key.
	encKey, err := crypto.ParsePublicKeyFromJWKSByUse(partner.PublicKeyJWKS, "enc")
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeValidation,
			"failed to parse partner encryption key from cached JWKS")
	}

	// 4. Build a signed + encrypted FideX envelope. The business document
	// is signed with our private key, then JWE-encrypted to the partner's
	// public key. Crypto details live in crypto.AS5Engine.
	if w.cryptoEngine == nil {
		return errors.Validation("crypto engine not configured on worker")
	}
	jwePayload, err := w.cryptoEngine.SignAndEncrypt(queued.Payload, encKey)
	if err != nil {
		return errors.InternalWrap(err, "failed to sign+encrypt business document")
	}

	envelope := crypto.FidexEnvelope{
		Routing: crypto.RoutingHeader{
			FidexVersion:   "1.0",
			MessageID:      msg.MessageID,
			SenderID:       w.nodeID,
			ReceiverID:     queued.DestinationPartnerID,
			DocumentType:   queued.DocumentType,
			Timestamp:      time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			ReceiptWebhook: queued.ReceiptWebhook,
		},
		Payload: jwePayload,
	}

	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		return errors.InternalWrap(err, "failed to marshal envelope")
	}

	// 5. Deliver to the partner's message endpoint.
	req, err := http.NewRequestWithContext(ctx, "POST", partner.MessageEndpoint, bytes.NewReader(envelopeBytes))
	if err != nil {
		return errors.InternalWrap(err, "failed to create HTTP request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "FideX-Node/1.0")
	req.Header.Set("X-FideX-Message-ID", msg.MessageID)
	req.Header.Set("X-FideX-Sender", w.nodeID)

	logger.Info(ctx, "Delivering message %s to %s at %s", msg.MessageID, partner.Name, partner.MessageEndpoint)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return errors.Network(err, "message delivery")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New(errors.ErrCodeNetwork,
			fmt.Sprintf("partner returned status %d: %s", resp.StatusCode, string(body)))
	}

	logger.Info(ctx, "Message %s delivered successfully to %s (status: %d)",
		msg.MessageID, partner.Name, resp.StatusCode)

	return nil
}

// deliverJMDN parses a queued send_jmdn job, builds a signed J-MDN
// disposition notification, and POSTs it to the original sender's
// receive_receipt endpoint. Spec §7 + interop with the reference peer
// (fidex-php) require the receipt to ship as a flat JSON object — not
// wrapped in a FidexEnvelope — with a detached `signature` JWS over the
// receipt body itself.
func (w *Worker) deliverJMDN(ctx context.Context, msg *domain.Message) error {
	var job queuedJMDNJob
	if err := json.Unmarshal([]byte(msg.Payload), &job); err != nil {
		return errors.Validation("invalid send_jmdn job payload")
	}
	if job.OriginalMessageID == "" {
		return errors.Validation("send_jmdn job has empty original_message_id")
	}
	if job.RecipientPartnerID == "" {
		return errors.Validation("send_jmdn job has empty recipient_partner_id")
	}
	if w.cryptoEngine == nil {
		return errors.Validation("crypto engine not configured on worker")
	}

	partner, err := w.partnerRepo.GetByID(ctx, job.RecipientPartnerID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodePartnerNotFound,
			fmt.Sprintf("partner %s not found for J-MDN delivery", job.RecipientPartnerID))
	}

	// Receipt destination: prefer mdn_receipt_endpoint (the explicit
	// receive_receipt URL from the partner's AS5 config). Fall back to the
	// message endpoint only as a last resort — that's a spec violation by
	// the partner but at least the receipt lands somewhere observable.
	receiptURL := partner.MDNReceiptEndpoint
	if receiptURL == "" {
		receiptURL = partner.MessageEndpoint
		if receiptURL == "" {
			return errors.Validation("partner has no receipt endpoint configured")
		}
		logger.Warn(ctx, "Partner %s has no mdn_receipt_endpoint; falling back to message endpoint",
			job.RecipientPartnerID)
	}

	// Normalize disposition status. Spec §7.3 uses uppercase tokens —
	// DELIVERED / FAILED — and the reference peer rejects anything else
	// silently by leaving the outbound row in SENT. Accept the legacy
	// lowercase "processed" alias for backwards-compat with older inbound
	// handlers that may still queue jobs with that string.
	status := strings.ToUpper(job.Status)
	switch status {
	case "DELIVERED", "FAILED":
		// pass through
	case "", "PROCESSED":
		status = "DELIVERED"
	default:
		// Anything we don't recognise is funnelled to DELIVERED rather
		// than blocking the sender's outbound row indefinitely.
		status = "DELIVERED"
	}

	receipt := w.buildJMDN(job.OriginalMessageID, status, job.OriginalPayload)
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return errors.InternalWrap(err, "failed to marshal J-MDN receipt")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", receiptURL, bytes.NewReader(receiptBytes))
	if err != nil {
		return errors.InternalWrap(err, "failed to create J-MDN HTTP request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "FideX-Node/1.0")
	req.Header.Set("X-FideX-Message-ID", msg.MessageID)
	req.Header.Set("X-FideX-Sender", w.nodeID)
	req.Header.Set("X-FideX-Receipt-For", job.OriginalMessageID)

	logger.Info(ctx, "Delivering J-MDN for %s to %s at %s",
		job.OriginalMessageID, partner.Name, receiptURL)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return errors.Network(err, "J-MDN delivery")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New(errors.ErrCodeNetwork,
			fmt.Sprintf("partner returned status %d on J-MDN: %s", resp.StatusCode, string(body)))
	}

	logger.Info(ctx, "J-MDN for %s delivered to %s (status: %d)",
		job.OriginalMessageID, partner.Name, resp.StatusCode)
	return nil
}

// buildJMDN constructs a signed J-MDN receipt body. The `signature` field
// is a compact JWS over the receipt's own JSON (sans signature) so the
// sender can verify integrity without trusting the transport. The hash
// over the original encrypted_payload provides defence in depth against a
// MITM swapping the receipt for a different message.
func (w *Worker) buildJMDN(originalMessageID, status, originalPayload string) jmdnReceiptBody {
	hash := sha256.Sum256([]byte(originalPayload))
	receipt := jmdnReceiptBody{
		OriginalMessageID: originalMessageID,
		Status:            status,
		ReceiverID:        w.nodeID,
		HashVerification:  "sha256:" + hex.EncodeToString(hash[:]),
		Timestamp:         time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}

	// Sign the JSON shape MINUS the signature field to keep verification
	// deterministic on the peer side. Errors from CreateJMDN fall through
	// as an empty signature — the peer will then 4xx the receipt and the
	// retry loop will pick it up.
	signingBytes, _ := json.Marshal(receipt)
	if jws, err := w.cryptoEngine.CreateJMDN(originalMessageID, status, signingBytes, nil); err == nil {
		receipt.Signature = jws
	}
	return receipt
}

// handleSuccess marks a message as delivered
func (w *Worker) handleSuccess(ctx context.Context, msg *domain.Message) {
	if err := w.messageRepo.UpdateStatus(ctx, msg.MessageID, domain.StatusDelivered); err != nil {
		logger.Error(ctx, "Failed to update message status to DELIVERED: %v", err)
	} else {
		logger.Info(ctx, "Message %s marked as DELIVERED", msg.MessageID)
	}
}

// handleFailure handles delivery failure with exponential backoff
func (w *Worker) handleFailure(ctx context.Context, msg *domain.Message, deliveryErr error) {
	msg.RetryCount++

	var lastError string
	if appErr, ok := errors.IsAppError(deliveryErr); ok {
		lastError = appErr.Message
	} else {
		lastError = deliveryErr.Error()
	}

	// Check if we should retry
	shouldRetry := errors.IsRetryable(deliveryErr) && msg.RetryCount < w.config.MaxRetries

	if !shouldRetry {
		// Mark as failed
		logger.Warn(ctx, "Message %s FAILED after %d attempts: %s", msg.MessageID, msg.RetryCount, lastError)

		if err := w.messageRepo.UpdateStatus(ctx, msg.MessageID, domain.StatusFailed); err != nil {
			logger.Error(ctx, "Failed to update message status to FAILED: %v", err)
		}

		// Update error info
		if err := w.messageRepo.UpdateRetryInfo(ctx, msg.MessageID, msg.RetryCount, nil, lastError); err != nil {
			logger.Error(ctx, "Failed to update retry info: %v", err)
		}
		return
	}

	// Calculate exponential backoff: 1m, 5m, 15m, 30m, 1h
	backoffMinutes := msg.RetryCount * msg.RetryCount
	if backoffMinutes > 60 {
		backoffMinutes = 60 // Cap at 1 hour
	}
	backoff := time.Duration(backoffMinutes) * time.Minute
	nextRetry := time.Now().Add(backoff)

	logger.Warn(ctx, "Message %s will retry %d/%d at %s (backoff: %v): %s",
		msg.MessageID, msg.RetryCount, w.config.MaxRetries,
		nextRetry.Format(time.RFC3339), backoff, lastError)

	// Update retry info in database
	if err := w.messageRepo.UpdateRetryInfo(ctx, msg.MessageID, msg.RetryCount, &nextRetry, lastError); err != nil {
		logger.Error(ctx, "Failed to update retry info: %v", err)
	}
}

// MessagePayload represents the structure of outbound message payloads
type MessagePayload struct {
	DestinationPartnerID string          `json:"destination_partner_id"`
	DocumentType         string          `json:"document_type"`
	BusinessDocument     json.RawMessage `json:"business_document"`
}
