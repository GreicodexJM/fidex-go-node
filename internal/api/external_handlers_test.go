package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fidex-node/internal/crypto"
	"fidex-node/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ----------------------------------------------------------------

// fakeMessageRepo implements just enough of domain.MessageRepository to drive
// the inbound handler. Tracks the inserted message so tests can assert on it
// and can be told to fail with ErrDuplicateMessageID to simulate replay.
type fakeMessageRepo struct {
	created     []*domain.Message
	failWith    error
	failOnIndex int
}

func (r *fakeMessageRepo) Create(_ context.Context, msg *domain.Message) error {
	if r.failWith != nil && len(r.created) == r.failOnIndex {
		return r.failWith
	}
	r.created = append(r.created, msg)
	return nil
}

func (r *fakeMessageRepo) GetByID(_ context.Context, _ string) (*domain.Message, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeMessageRepo) ListByStatus(_ context.Context, _ domain.MessageStatus) ([]*domain.Message, error) {
	return nil, nil
}
func (r *fakeMessageRepo) ListPaginated(_ context.Context, _ *domain.MessageStatus, _, _ int) ([]*domain.Message, int, error) {
	return nil, 0, nil
}
func (r *fakeMessageRepo) CountByStatusSince(_ context.Context, _ time.Time) (map[domain.MessageStatus]int, error) {
	return nil, nil
}
func (r *fakeMessageRepo) UpdateStatus(_ context.Context, _ string, _ domain.MessageStatus) error {
	return nil
}
func (r *fakeMessageRepo) UpdateRetryInfo(_ context.Context, _ string, _ int, _ *time.Time, _ string) error {
	return nil
}
func (r *fakeMessageRepo) Delete(_ context.Context, _ string) error { return nil }

// fakePartnerRepo returns a single configured partner or NotFound for
// anything else. Lets us drive both the unknown-sender and tampered-JWS
// rejection paths.
type fakePartnerRepo struct {
	known *domain.Partner
}

func (r *fakePartnerRepo) GetByID(_ context.Context, partnerID string) (*domain.Partner, error) {
	if r.known != nil && r.known.PartnerID == partnerID {
		return r.known, nil
	}
	return nil, errors.New("partner not found")
}
func (r *fakePartnerRepo) Create(_ context.Context, _ *domain.Partner) error { return nil }
func (r *fakePartnerRepo) Update(_ context.Context, _ *domain.Partner) error { return nil }
func (r *fakePartnerRepo) Upsert(_ context.Context, _ *domain.Partner) error { return nil }
func (r *fakePartnerRepo) Delete(_ context.Context, _ string) error          { return nil }
func (r *fakePartnerRepo) DeleteByDBID(_ context.Context, _ int64) error     { return nil }
func (r *fakePartnerRepo) UpdateNameByDBID(_ context.Context, _ int64, _ string) error {
	return nil
}
func (r *fakePartnerRepo) List(_ context.Context) ([]*domain.Partner, error) { return nil, nil }
func (r *fakePartnerRepo) Count(_ context.Context) (int, error)              { return 0, nil }

// --- helpers --------------------------------------------------------------

func envelopeJSON(t *testing.T, senderID, messageID string) []byte {
	t.Helper()
	env := map[string]interface{}{
		"routing_header": map[string]interface{}{
			"fidex_version": "1.0",
			"message_id":    messageID,
			"sender_id":     senderID,
			"receiver_id":   "urn:test:nut",
			"document_type": "GS1_INVOICE_JSON",
			"timestamp":     time.Now().UTC().Format(time.RFC3339),
		},
		"encrypted_payload": "eyJhbGciOiJSU0EtT0FFUC0yNTYiLCJlbmMiOiJBMjU2R0NNIn0.AAAA.BBBB.CCCC.DDDD",
	}
	b, err := json.Marshal(env)
	require.NoError(t, err)
	return b
}

// newHandlersForInboundTest wires a Handlers with the given repos plus a
// real (but isolated) AS5 engine so the decrypt path can run for real.
func newHandlersForInboundTest(t *testing.T, msgRepo domain.MessageRepository, partnerRepo domain.PartnerRepository) *Handlers {
	t.Helper()
	priv, _, err := crypto.GenerateKeyPair()
	require.NoError(t, err)
	engine, err := crypto.NewAS5Engine(priv, "urn:test:nut")
	require.NoError(t, err)
	return &Handlers{
		MessageRepo:   msgRepo,
		PartnerRepo:   partnerRepo,
		CryptoService: engine,
	}
}

// --- FID-1: rejection on bad envelope ------------------------------------

func TestInboundHandler_RejectsUnknownSender(t *testing.T) {
	msgRepo := &fakeMessageRepo{}
	partnerRepo := &fakePartnerRepo{} // no known partners
	h := newHandlersForInboundTest(t, msgRepo, partnerRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inbound",
		bytes.NewReader(envelopeJSON(t, "urn:custom:unregistered-nobody", "msg-unknown-001")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.inboundHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"unknown sender must surface as 401, not silent 202")

	var body inboundRejection
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, errCodeUnknownPartner, body.ErrorCode)
	assert.Equal(t, "QUARANTINED", body.Status)
	// Envelope must still be persisted for forensic visibility.
	require.Len(t, msgRepo.created, 1, "envelope should be persisted for audit even on rejection")
	assert.Equal(t, domain.StatusQuarantined, msgRepo.created[0].Status)
}

func TestInboundHandler_RejectsTamperedJWS(t *testing.T) {
	// Build a partner with a real JWKS so the handler reaches the crypto step.
	priv, _, err := crypto.GenerateKeyPair()
	require.NoError(t, err)
	peerEngine, err := crypto.NewAS5Engine(priv, "urn:test:peer")
	require.NoError(t, err)
	jwks, err := peerEngine.ExportJWKS("urn:test:peer")
	require.NoError(t, err)

	msgRepo := &fakeMessageRepo{}
	partnerRepo := &fakePartnerRepo{
		known: &domain.Partner{
			PartnerID:     "urn:test:peer",
			Name:          "Test Peer",
			PublicKeyJWKS: jwks,
		},
	}
	h := newHandlersForInboundTest(t, msgRepo, partnerRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inbound",
		bytes.NewReader(envelopeJSON(t, "urn:test:peer", "msg-tampered-001")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.inboundHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"tampered/unparseable JWE must be 4xx, not 5xx")

	var body inboundRejection
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t,
		body.ErrorCode == errCodeSignatureInvalid || body.ErrorCode == errCodeDecryptFailed,
		"unexpected error_code: %s", body.ErrorCode)
	require.Len(t, msgRepo.created, 1)
	assert.Equal(t, domain.StatusQuarantined, msgRepo.created[0].Status)
}

func TestInboundHandler_RejectsMalformedBody(t *testing.T) {
	h := newHandlersForInboundTest(t, &fakeMessageRepo{}, &fakePartnerRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inbound",
		strings.NewReader("not-json-at-all"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.inboundHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// --- FID-2: duplicate message_id → 409 -----------------------------------

func TestInboundHandler_DuplicateMessageReturns409(t *testing.T) {
	// Repo fails the FIRST insert (the inbound row itself) with the duplicate
	// sentinel — simulating a replay where the row already exists.
	msgRepo := &fakeMessageRepo{
		failWith:    domain.ErrDuplicateMessageID,
		failOnIndex: 0,
	}
	h := newHandlersForInboundTest(t, msgRepo, &fakePartnerRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/inbound",
		bytes.NewReader(envelopeJSON(t, "urn:custom:replay-probe", "msg-replay-001")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.inboundHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"replay of duplicate message_id must return 409 per spec §9.3")

	var body inboundRejection
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, errCodeDuplicateMessage, body.ErrorCode)
	assert.Equal(t, "REJECTED", body.Status)
}
