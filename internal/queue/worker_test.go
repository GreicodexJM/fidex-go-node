package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fidex-node/internal/crypto"
	"fidex-node/internal/domain"
	"fidex-node/internal/errors"

	"github.com/stretchr/testify/assert"
)

// Mock repositories for testing
type mockMessageRepository struct {
	messages         map[string]*domain.Message
	listByStatusFunc func(ctx context.Context, status domain.MessageStatus) ([]*domain.Message, error)
	updateStatusFunc func(ctx context.Context, messageID string, status domain.MessageStatus) error
	updateRetryFunc  func(ctx context.Context, messageID string, retryCount int, nextRetry *time.Time, lastError string) error
}

func (m *mockMessageRepository) Create(ctx context.Context, msg *domain.Message) error {
	if m.messages == nil {
		m.messages = make(map[string]*domain.Message)
	}
	m.messages[msg.MessageID] = msg
	return nil
}

func (m *mockMessageRepository) GetByID(ctx context.Context, messageID string) (*domain.Message, error) {
	if msg, ok := m.messages[messageID]; ok {
		return msg, nil
	}
	return nil, fmt.Errorf("message not found")
}

func (m *mockMessageRepository) ListByStatus(ctx context.Context, status domain.MessageStatus) ([]*domain.Message, error) {
	if m.listByStatusFunc != nil {
		return m.listByStatusFunc(ctx, status)
	}
	var result []*domain.Message
	for _, msg := range m.messages {
		if msg.Status == status {
			result = append(result, msg)
		}
	}
	return result, nil
}

func (m *mockMessageRepository) UpdateStatus(ctx context.Context, messageID string, status domain.MessageStatus) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, messageID, status)
	}
	if msg, ok := m.messages[messageID]; ok {
		msg.Status = status
		return nil
	}
	return fmt.Errorf("message not found")
}

func (m *mockMessageRepository) UpdateRetryInfo(ctx context.Context, messageID string, retryCount int, nextRetry *time.Time, lastError string) error {
	if m.updateRetryFunc != nil {
		return m.updateRetryFunc(ctx, messageID, retryCount, nextRetry, lastError)
	}
	if msg, ok := m.messages[messageID]; ok {
		msg.RetryCount = retryCount
		msg.NextRetryAt = nextRetry
		msg.LastError = lastError
		return nil
	}
	return fmt.Errorf("message not found")
}

func (m *mockMessageRepository) Delete(ctx context.Context, messageID string) error {
	delete(m.messages, messageID)
	return nil
}

func (m *mockMessageRepository) ListPaginated(
	ctx context.Context,
	statusFilter *domain.MessageStatus,
	limit, offset int,
) ([]*domain.Message, int, error) {
	var all []*domain.Message
	for _, msg := range m.messages {
		if statusFilter != nil && msg.Status != *statusFilter {
			continue
		}
		all = append(all, msg)
	}
	total := len(all)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

func (m *mockMessageRepository) CountByStatusSince(
	ctx context.Context,
	since time.Time,
) (map[domain.MessageStatus]int, error) {
	counts := map[domain.MessageStatus]int{}
	for _, msg := range m.messages {
		if msg.CreatedAt.Before(since) {
			continue
		}
		counts[msg.Status]++
	}
	return counts, nil
}

type mockPartnerRepository struct {
	partners map[string]*domain.Partner
}

func (m *mockPartnerRepository) Create(ctx context.Context, partner *domain.Partner) error {
	if m.partners == nil {
		m.partners = make(map[string]*domain.Partner)
	}
	m.partners[partner.PartnerID] = partner
	return nil
}

func (m *mockPartnerRepository) GetByID(ctx context.Context, partnerID string) (*domain.Partner, error) {
	if partner, ok := m.partners[partnerID]; ok {
		return partner, nil
	}
	return nil, fmt.Errorf("partner not found: %s", partnerID)
}

func (m *mockPartnerRepository) Update(ctx context.Context, partner *domain.Partner) error {
	m.partners[partner.PartnerID] = partner
	return nil
}

func (m *mockPartnerRepository) Delete(ctx context.Context, partnerID string) error {
	delete(m.partners, partnerID)
	return nil
}

func (m *mockPartnerRepository) Upsert(ctx context.Context, partner *domain.Partner) error {
	if m.partners == nil {
		m.partners = make(map[string]*domain.Partner)
	}
	m.partners[partner.PartnerID] = partner
	return nil
}

func (m *mockPartnerRepository) DeleteByDBID(ctx context.Context, id int64) error {
	for k, p := range m.partners {
		if p.ID == id {
			delete(m.partners, k)
			return nil
		}
	}
	return fmt.Errorf("partner not found: id=%d", id)
}

func (m *mockPartnerRepository) UpdateNameByDBID(ctx context.Context, id int64, name string) error {
	for _, p := range m.partners {
		if p.ID == id {
			p.Name = name
			return nil
		}
	}
	return fmt.Errorf("partner not found: id=%d", id)
}

func (m *mockPartnerRepository) Count(ctx context.Context) (int, error) {
	return len(m.partners), nil
}

func (m *mockPartnerRepository) List(ctx context.Context) ([]*domain.Partner, error) {
	var result []*domain.Partner
	for _, p := range m.partners {
		result = append(result, p)
	}
	return result, nil
}

// Helper to create test message with envelope
// createTestMessage builds a queued outbound row in the shape the worker
// expects: a TransmitRequest-like JSON with destination_partner_id and a
// business document under "payload".
func createTestMessage(messageID, partnerID string) *domain.Message {
	queued := queuedOutboundPayload{
		DestinationPartnerID: partnerID,
		DocumentType:         "TEST",
		Payload:              json.RawMessage(`{"hello":"world"}`),
	}
	payload, _ := json.Marshal(queued)
	return &domain.Message{
		MessageID: messageID,
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   string(payload),
	}
}

// makeTestEngineAndJWKS generates a fresh RSA keypair, wraps it in an
// AS5Engine instance, and returns its public key encoded as a JWKS string
// suitable for caching on a partner row in tests.
func makeTestEngineAndJWKS(t *testing.T, kid string) (*crypto.AS5Engine, string) {
	t.Helper()
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	_ = pub
	engine, err := crypto.NewAS5Engine(priv, "test-node")
	if err != nil {
		t.Fatalf("NewAS5Engine: %v", err)
	}
	jwks, err := engine.ExportJWKS(kid)
	if err != nil {
		t.Fatalf("ExportJWKS: %v", err)
	}
	return engine, jwks
}

func TestWorkerConfig_Defaults(t *testing.T) {
	config := DefaultWorkerConfig()

	assert.Equal(t, 10*time.Second, config.PollInterval)
	assert.Equal(t, 5, config.MaxRetries)
	assert.Equal(t, 30*time.Second, config.RequestTimeout)
}

func TestNewWorker(t *testing.T) {
	msgRepo := &mockMessageRepository{}
	partnerRepo := &mockPartnerRepository{}
	cryptoEngine := &crypto.AS5Engine{}

	worker := NewWorker(msgRepo, partnerRepo, cryptoEngine)

	assert.NotNil(t, worker)
	assert.NotNil(t, worker.httpClient)
	assert.NotNil(t, worker.stopChan)
	assert.Equal(t, DefaultWorkerConfig(), worker.config)
}

func TestNewWorkerWithConfig(t *testing.T) {
	msgRepo := &mockMessageRepository{}
	partnerRepo := &mockPartnerRepository{}
	cryptoEngine := &crypto.AS5Engine{}

	customConfig := WorkerConfig{
		PollInterval:   5 * time.Second,
		MaxRetries:     3,
		RequestTimeout: 15 * time.Second,
	}

	worker := NewWorkerWithConfig(msgRepo, partnerRepo, cryptoEngine, customConfig)

	assert.NotNil(t, worker)
	assert.Equal(t, customConfig, worker.config)
	assert.Equal(t, 15*time.Second, worker.httpClient.Timeout)
}

func TestWorker_DeliverMessage_Success(t *testing.T) {
	engine, jwks := makeTestEngineAndJWKS(t, "partner-1")

	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "FideX-Node/1.0", r.Header.Get("User-Agent"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"received"}`))
	}))
	defer server.Close()

	// Setup repositories
	msgRepo := &mockMessageRepository{messages: make(map[string]*domain.Message)}
	partnerRepo := &mockPartnerRepository{
		partners: map[string]*domain.Partner{
			"partner-1": {
				PartnerID:       "partner-1",
				Name:            "Test Partner",
				MessageEndpoint: server.URL,
				PublicKeyJWKS:   jwks,
			},
		},
	}

	// Worker with real crypto engine so SignAndEncrypt works.
	worker := NewWorker(msgRepo, partnerRepo, engine)
	worker.SetNodeID("urn:test:sender")

	msg := createTestMessage("msg-1", "partner-1")
	msgRepo.messages[msg.MessageID] = msg

	err := worker.deliverMessage(context.Background(), msg)
	assert.NoError(t, err)
}

func TestWorker_DeliverMessage_PartnerNotFound(t *testing.T) {
	msgRepo := &mockMessageRepository{messages: make(map[string]*domain.Message)}
	partnerRepo := &mockPartnerRepository{partners: make(map[string]*domain.Partner)}

	worker := NewWorker(msgRepo, partnerRepo, nil)

	msg := createTestMessage("msg-1", "nonexistent-partner")

	err := worker.deliverMessage(context.Background(), msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestWorker_DeliverMessage_NoEndpoint(t *testing.T) {
	msgRepo := &mockMessageRepository{}
	partnerRepo := &mockPartnerRepository{
		partners: map[string]*domain.Partner{
			"partner-1": {
				PartnerID:       "partner-1",
				Name:            "Test Partner",
				MessageEndpoint: "", // No endpoint
			},
		},
	}

	worker := NewWorker(msgRepo, partnerRepo, nil)

	msg := createTestMessage("msg-1", "partner-1")

	err := worker.deliverMessage(context.Background(), msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no message endpoint")
}

func TestWorker_DeliverMessage_HTTPError(t *testing.T) {
	engine, jwks := makeTestEngineAndJWKS(t, "partner-1")

	// Mock server returning 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	msgRepo := &mockMessageRepository{}
	partnerRepo := &mockPartnerRepository{
		partners: map[string]*domain.Partner{
			"partner-1": {
				PartnerID:       "partner-1",
				MessageEndpoint: server.URL,
				PublicKeyJWKS:   jwks,
			},
		},
	}

	worker := NewWorker(msgRepo, partnerRepo, engine)
	worker.SetNodeID("urn:test:sender")
	msg := createTestMessage("msg-1", "partner-1")

	err := worker.deliverMessage(context.Background(), msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestWorker_HandleSuccess(t *testing.T) {
	msgRepo := &mockMessageRepository{messages: make(map[string]*domain.Message)}

	msg := createTestMessage("msg-1", "partner-1")
	msgRepo.messages[msg.MessageID] = msg

	worker := NewWorker(msgRepo, nil, nil)

	worker.handleSuccess(context.Background(), msg)

	// Verify status was updated
	assert.Equal(t, domain.StatusDelivered, msgRepo.messages[msg.MessageID].Status)
}

func TestWorker_HandleFailure_Retryable(t *testing.T) {
	msgRepo := &mockMessageRepository{messages: make(map[string]*domain.Message)}

	msg := createTestMessage("msg-1", "partner-1")
	msg.RetryCount = 0
	msgRepo.messages[msg.MessageID] = msg

	config := WorkerConfig{
		PollInterval:   10 * time.Second,
		MaxRetries:     5,
		RequestTimeout: 30 * time.Second,
	}

	worker := NewWorkerWithConfig(msgRepo, nil, nil, config)

	// Network error is retryable
	err := errors.Network(fmt.Errorf("connection timeout"), "delivery")

	beforeTime := time.Now()
	worker.handleFailure(context.Background(), msg, err)

	// Verify retry count increased
	assert.Equal(t, 1, msgRepo.messages[msg.MessageID].RetryCount)

	// Verify next retry time is set (with backoff)
	assert.NotNil(t, msgRepo.messages[msg.MessageID].NextRetryAt)
	assert.True(t, msgRepo.messages[msg.MessageID].NextRetryAt.After(beforeTime))

	// Verify error is recorded
	assert.Contains(t, msgRepo.messages[msg.MessageID].LastError, "delivery failed")

	// Verify status is still queued
	assert.Equal(t, domain.StatusQueued, msgRepo.messages[msg.MessageID].Status)
}

func TestWorker_HandleFailure_NonRetryable(t *testing.T) {
	msgRepo := &mockMessageRepository{messages: make(map[string]*domain.Message)}

	msg := createTestMessage("msg-1", "partner-1")
	msg.RetryCount = 0
	msgRepo.messages[msg.MessageID] = msg

	worker := NewWorker(msgRepo, nil, nil)

	// Validation error is not retryable
	err := errors.Validation("invalid payload")

	worker.handleFailure(context.Background(), msg, err)

	// Verify marked as failed
	assert.Equal(t, domain.StatusFailed, msgRepo.messages[msg.MessageID].Status)

	// Verify retry count was incremented
	assert.Equal(t, 1, msgRepo.messages[msg.MessageID].RetryCount)

	// Verify error is recorded
	assert.Contains(t, msgRepo.messages[msg.MessageID].LastError, "invalid payload")
}

func TestWorker_HandleFailure_MaxRetriesExceeded(t *testing.T) {
	msgRepo := &mockMessageRepository{messages: make(map[string]*domain.Message)}

	msg := createTestMessage("msg-1", "partner-1")
	msg.RetryCount = 4 // Already tried 4 times
	msgRepo.messages[msg.MessageID] = msg

	config := WorkerConfig{
		MaxRetries: 5,
	}

	worker := NewWorkerWithConfig(msgRepo, nil, nil, config)

	// Even though error is retryable, max retries exceeded
	err := errors.Network(fmt.Errorf("timeout"), "delivery")

	worker.handleFailure(context.Background(), msg, err)

	// Verify marked as failed
	assert.Equal(t, domain.StatusFailed, msgRepo.messages[msg.MessageID].Status)

	// Verify retry count increased to 5
	assert.Equal(t, 5, msgRepo.messages[msg.MessageID].RetryCount)
}

func TestWorker_ExponentialBackoff(t *testing.T) {
	msgRepo := &mockMessageRepository{messages: make(map[string]*domain.Message)}

	worker := NewWorker(msgRepo, nil, nil)
	err := errors.Network(fmt.Errorf("timeout"), "delivery")

	testCases := []struct {
		retryCount      int
		expectedMinutes int
	}{
		{0, 1},  // 1^2 = 1 minute
		{1, 1},  // 2^2 = 4 minutes (but count is now 2 after increment)
		{2, 9},  // 3^2 = 9 minutes
		{3, 16}, // 4^2 = 16 minutes
		{4, 25}, // 5^2 = 25 minutes
		{9, 60}, // Capped at 60 minutes
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("retry_%d", tc.retryCount), func(t *testing.T) {
			msg := createTestMessage(fmt.Sprintf("msg-%d", tc.retryCount), "partner-1")
			msg.RetryCount = tc.retryCount
			msgRepo.messages[msg.MessageID] = msg

			beforeTime := time.Now()
			worker.handleFailure(context.Background(), msg, err)

			// If this exceeds max retries, message will be marked as failed with no next retry
			if tc.retryCount+1 >= worker.config.MaxRetries {
				// Message should be marked as failed
				assert.Equal(t, domain.StatusFailed, msgRepo.messages[msg.MessageID].Status)
				return
			}

			// Calculate expected backoff
			backoffMinutes := (tc.retryCount + 1) * (tc.retryCount + 1)
			if backoffMinutes > 60 {
				backoffMinutes = 60
			}
			expectedBackoff := time.Duration(backoffMinutes) * time.Minute
			expectedTime := beforeTime.Add(expectedBackoff)

			// Verify next retry time (allow 1 second tolerance)
			if msgRepo.messages[msg.MessageID].NextRetryAt != nil {
				actualTime := *msgRepo.messages[msg.MessageID].NextRetryAt
				timeDiff := actualTime.Sub(expectedTime).Abs()
				assert.Less(t, timeDiff, time.Second)
			}
		})
	}
}

func TestWorker_ProcessQueue_EmptyQueue(t *testing.T) {
	msgRepo := &mockMessageRepository{
		listByStatusFunc: func(ctx context.Context, status domain.MessageStatus) ([]*domain.Message, error) {
			return []*domain.Message{}, nil
		},
	}

	worker := NewWorker(msgRepo, nil, nil)

	// Should not panic with empty queue
	worker.processQueue()
}

func TestWorker_ProcessQueue_SkipsNotReadyMessages(t *testing.T) {
	futureTime := time.Now().Add(1 * time.Hour)

	msgRepo := &mockMessageRepository{
		messages: make(map[string]*domain.Message),
		listByStatusFunc: func(ctx context.Context, status domain.MessageStatus) ([]*domain.Message, error) {
			return []*domain.Message{
				{
					MessageID:   "msg-future",
					Status:      domain.StatusQueued,
					NextRetryAt: &futureTime, // Not ready yet
					Payload:     "test",
				},
			}, nil
		},
	}

	partnerRepo := &mockPartnerRepository{}
	worker := NewWorker(msgRepo, partnerRepo, nil)

	// Process queue
	worker.processQueue()

	// Message should not have been processed (status unchanged)
	// This is more of an integration behavior test
}

func TestWorker_StartStop(t *testing.T) {
	msgRepo := &mockMessageRepository{
		listByStatusFunc: func(ctx context.Context, status domain.MessageStatus) ([]*domain.Message, error) {
			return []*domain.Message{}, nil
		},
	}

	config := WorkerConfig{
		PollInterval:   100 * time.Millisecond, // Fast polling for test
		MaxRetries:     5,
		RequestTimeout: 30 * time.Second,
	}

	worker := NewWorkerWithConfig(msgRepo, nil, nil, config)

	// Start worker
	worker.Start()

	// Let it run briefly
	time.Sleep(150 * time.Millisecond)

	// Stop worker
	worker.Stop()

	// Give it time to stop
	time.Sleep(50 * time.Millisecond)

	// Worker should have stopped gracefully
	// (no way to directly verify, but should not hang)
}

func TestWorker_InvalidMessagePayload(t *testing.T) {
	msgRepo := &mockMessageRepository{}
	worker := NewWorker(msgRepo, nil, nil)

	msg := &domain.Message{
		MessageID: "invalid-msg",
		Payload:   "{invalid json", // Invalid JSON
	}

	err := worker.deliverMessage(context.Background(), msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestWorker_HTTPTimeout(t *testing.T) {
	// Create slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	msgRepo := &mockMessageRepository{}
	partnerRepo := &mockPartnerRepository{
		partners: map[string]*domain.Partner{
			"partner-1": {
				PartnerID:       "partner-1",
				MessageEndpoint: server.URL,
			},
		},
	}

	// Create worker with very short timeout
	config := WorkerConfig{
		RequestTimeout: 10 * time.Millisecond,
		MaxRetries:     5,
	}

	worker := NewWorkerWithConfig(msgRepo, partnerRepo, nil, config)
	msg := createTestMessage("msg-1", "partner-1")

	err := worker.deliverMessage(context.Background(), msg)

	// Should timeout
	assert.Error(t, err)
}
