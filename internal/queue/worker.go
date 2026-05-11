package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

// queuedOutboundPayload is the JSON shape this worker expects on every
// outbound row in the messages table. It matches api.TransmitRequest plus
// the receipt-routing shape used by the inbound handler. The worker is the
// place where business documents become signed-and-encrypted FideX envelopes.
type queuedOutboundPayload struct {
	DestinationPartnerID string          `json:"destination_partner_id"`
	DocumentType         string          `json:"document_type"`
	ReceiptWebhook       string          `json:"receipt_webhook,omitempty"`
	Payload              json.RawMessage `json:"payload"`
}

// deliverMessage parses a queued outbound row, builds a signed+encrypted
// FideX envelope addressed to the destination partner, and POSTs it to the
// partner's published message endpoint.
func (w *Worker) deliverMessage(ctx context.Context, msg *domain.Message) error {
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
