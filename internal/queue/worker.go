package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"fidex-node/internal/crypto"
	"fidex-node/internal/domain"
	"fidex-node/internal/errors"
)

// Worker represents the message queue worker responsible for delivering outbound messages
type Worker struct {
	messageRepo  domain.MessageRepository
	partnerRepo  domain.PartnerRepository
	cryptoEngine *crypto.AS5Engine
	httpClient   *http.Client
	stopChan     chan struct{}
	config       WorkerConfig
}

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
		log.Println("Queue worker started")
		ticker := time.NewTicker(w.config.PollInterval)
		defer ticker.Stop()

		// Process queue immediately on start
		w.processQueue()

		for {
			select {
			case <-ticker.C:
				w.processQueue()
			case <-w.stopChan:
				log.Println("Queue worker stopped")
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
		log.Printf("ERROR: Failed to get queued messages: %v", err)
		return
	}

	if len(messages) == 0 {
		return
	}

	log.Printf("Processing %d queued message(s)", len(messages))

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

// deliverMessage delivers a message to the destination partner
func (w *Worker) deliverMessage(ctx context.Context, msg *domain.Message) error {
	// Parse the FideX envelope from payload
	var envelope crypto.FidexEnvelope
	if err := json.Unmarshal([]byte(msg.Payload), &envelope); err != nil {
		return errors.Validation("invalid message payload format")
	}

	// Get partner information
	partner, err := w.partnerRepo.GetByID(ctx, envelope.Routing.ReceiverID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodePartnerNotFound,
			fmt.Sprintf("partner %s not found", envelope.Routing.ReceiverID))
	}

	if partner.MessageEndpoint == "" {
		return errors.Validation("partner has no message endpoint configured")
	}

	// Prepare HTTP request
	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return errors.InternalWrap(err, "failed to marshal envelope")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", partner.MessageEndpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return errors.InternalWrap(err, "failed to create HTTP request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "FideX-Node/1.0")
	req.Header.Set("X-FideX-Message-ID", envelope.Routing.MessageID)
	req.Header.Set("X-FideX-Sender", envelope.Routing.SenderID)

	// Send the request
	log.Printf("Delivering message %s to %s at %s", msg.MessageID, partner.Name, partner.MessageEndpoint)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return errors.Network(err, "message delivery")
	}
	defer resp.Body.Close()

	// Read response body for logging
	body, _ := io.ReadAll(resp.Body)

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New(errors.ErrCodeNetwork,
			fmt.Sprintf("partner returned status %d: %s", resp.StatusCode, string(body)))
	}

	log.Printf("Message %s delivered successfully to %s (status: %d)",
		msg.MessageID, partner.Name, resp.StatusCode)

	return nil
}

// handleSuccess marks a message as delivered
func (w *Worker) handleSuccess(ctx context.Context, msg *domain.Message) {
	if err := w.messageRepo.UpdateStatus(ctx, msg.MessageID, domain.StatusDelivered); err != nil {
		log.Printf("ERROR: Failed to update message status to DELIVERED: %v", err)
	} else {
		log.Printf("Message %s marked as DELIVERED", msg.MessageID)
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
		log.Printf("Message %s FAILED after %d attempts: %s", msg.MessageID, msg.RetryCount, lastError)

		if err := w.messageRepo.UpdateStatus(ctx, msg.MessageID, domain.StatusFailed); err != nil {
			log.Printf("ERROR: Failed to update message status to FAILED: %v", err)
		}

		// Update error info
		if err := w.messageRepo.UpdateRetryInfo(ctx, msg.MessageID, msg.RetryCount, nil, lastError); err != nil {
			log.Printf("ERROR: Failed to update retry info: %v", err)
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

	log.Printf("Message %s will retry %d/%d at %s (backoff: %v): %s",
		msg.MessageID, msg.RetryCount, w.config.MaxRetries,
		nextRetry.Format(time.RFC3339), backoff, lastError)

	// Update retry info in database
	if err := w.messageRepo.UpdateRetryInfo(ctx, msg.MessageID, msg.RetryCount, &nextRetry, lastError); err != nil {
		log.Printf("ERROR: Failed to update retry info: %v", err)
	}
}

// MessagePayload represents the structure of outbound message payloads
type MessagePayload struct {
	DestinationPartnerID string          `json:"destination_partner_id"`
	DocumentType         string          `json:"document_type"`
	BusinessDocument     json.RawMessage `json:"business_document"`
}
