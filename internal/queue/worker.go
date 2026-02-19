package queue

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"fidex-node/internal/db"
)

// Worker represents the message queue worker
type Worker struct {
	stopChan chan struct{}
}

// NewWorker creates a new queue worker
func NewWorker() *Worker {
	return &Worker{
		stopChan: make(chan struct{}),
	}
}

// Start starts the worker loop
func (w *Worker) Start() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				w.processQueue()
			case <-w.stopChan:
				return
			}
		}
	}()
}

// Stop stops the worker
func (w *Worker) Stop() {
	close(w.stopChan)
}

// processQueue processes queued messages
func (w *Worker) processQueue() {
	messages, err := db.GetQueuedMessages()
	if err != nil {
		log.Printf("Failed to get queued messages: %v", err)
		return
	}

	for _, msg := range messages {
		// Check if it's time to retry
		if msg.NextRetryAt != nil && msg.NextRetryAt.After(time.Now()) {
			continue
		}

		// Process message
		if err := w.deliverMessage(&msg); err != nil {
			log.Printf("Failed to deliver message %s: %v", msg.MessageID, err)
			w.handleFailure(&msg, err)
		} else {
			log.Printf("Message %s delivered successfully", msg.MessageID)
			db.UpdateMessageStatus(msg.MessageID, string(db.StatusDelivered))
		}
	}
}

// deliverMessage attempts to deliver a message to the partner
func (w *Worker) deliverMessage(msg *db.Message) error {
	// Parse payload to get destination
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}

	destinationID, ok := payload["destination_partner_id"].(string)
	if !ok {
		return fmt.Errorf("destination_partner_id missing or invalid")
	}

	log.Printf("Delivering message to partner: %s", destinationID)

	// Get partner config
	// In a real implementation, we would look up the partner in the database
	// For now, we'll assume the destination ID is the discovery URL or we have it cached
	// This part needs to be expanded to look up the partner's endpoint from the trading_partners table

	// Mock delivery for now
	// In production:
	// 1. Get partner from DB
	// 2. Get their AS5 endpoint
	// 3. Encrypt payload
	// 4. Send HTTP POST

	// Simulate network delay
	time.Sleep(500 * time.Millisecond)

	// Simulate random failure (10% chance)
	// if time.Now().UnixNano()%10 == 0 {
	// 	return fmt.Errorf("simulated network error")
	// }

	return nil
}

// handleFailure handles delivery failure with exponential backoff
func (w *Worker) handleFailure(msg *db.Message, err error) {
	maxRetries := 5
	msg.RetryCount++
	msg.LastError = err.Error()

	if msg.RetryCount >= maxRetries {
		db.UpdateMessageStatus(msg.MessageID, string(db.StatusFailed))
		// Update last error in DB (need to add UpdateMessageError function)
		return
	}

	// Exponential backoff: 1m, 5m, 15m, 30m, 1h
	backoff := time.Duration(msg.RetryCount*msg.RetryCount) * time.Minute
	nextRetry := time.Now().Add(backoff)
	msg.NextRetryAt = &nextRetry

	// Update message in DB (need to add UpdateMessageRetry function)
	// For now, we'll just log it
	log.Printf("Message %s scheduled for retry %d at %s", msg.MessageID, msg.RetryCount, nextRetry)
}
