package queue

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"fidex-node/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorker_DeliverMessage_NegotiatesOAEP256 wires the FID-4 / ADR-0003
// negotiation through the real worker.deliverMessage path: a partner that
// advertises only RSA-OAEP-256 must result in a JWE protected header with
// alg=RSA-OAEP-256, NOT the legacy SHA-1 default.
func TestWorker_DeliverMessage_NegotiatesOAEP256(t *testing.T) {
	engine, jwks := makeTestEngineAndJWKS(t, "peer-oaep256")

	var (
		capturedAlg string
		capturedMu  sync.Mutex
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var envelope struct {
			Payload string `json:"encrypted_payload"`
		}
		if err := json.NewDecoder(bytes.NewReader(body)).Decode(&envelope); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		parts := strings.Split(envelope.Payload, ".")
		if len(parts) != 5 {
			http.Error(w, "bad JWE", 500)
			return
		}
		header, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		capturedMu.Lock()
		capturedAlg = string(header)
		capturedMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	msgRepo := &mockMessageRepository{}
	partnerRepo := &mockPartnerRepository{
		partners: map[string]*domain.Partner{
			"partner-oaep256": {
				PartnerID:                     "partner-oaep256",
				Name:                          "OAEP-256 Partner",
				MessageEndpoint:               server.URL,
				PublicKeyJWKS:                 jwks,
				SupportedEncryptionAlgorithms: []string{"RSA-OAEP", "RSA-OAEP-256"},
			},
		},
	}

	worker := NewWorker(msgRepo, partnerRepo, engine)
	worker.SetNodeID("urn:test:sender")
	msg := createTestMessage("msg-fid4-256", "partner-oaep256")

	err := worker.deliverMessage(context.Background(), msg)
	require.NoError(t, err)

	capturedMu.Lock()
	defer capturedMu.Unlock()
	assert.Contains(t, capturedAlg, `"alg":"RSA-OAEP-256"`,
		"worker must negotiate RSA-OAEP-256 when both peers advertise it; header=%s", capturedAlg)
}

// TestWorker_DeliverMessage_NegotiatesPlainOAEPForAsymmetricPeer verifies
// the fall-back path: when the peer's advertised list contains only the
// legacy RSA-OAEP entry, the worker must downgrade to that algorithm
// rather than refusing the delivery or silently using SHA-256.
func TestWorker_DeliverMessage_NegotiatesPlainOAEPForAsymmetricPeer(t *testing.T) {
	engine, jwks := makeTestEngineAndJWKS(t, "peer-legacy")

	var capturedAlg string
	var capturedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var envelope struct {
			Payload string `json:"encrypted_payload"`
		}
		_ = json.Unmarshal(body, &envelope)
		parts := strings.Split(envelope.Payload, ".")
		header, _ := base64.RawURLEncoding.DecodeString(parts[0])
		capturedMu.Lock()
		capturedAlg = string(header)
		capturedMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	msgRepo := &mockMessageRepository{}
	partnerRepo := &mockPartnerRepository{
		partners: map[string]*domain.Partner{
			"partner-legacy": {
				PartnerID:                     "partner-legacy",
				Name:                          "Legacy Partner (OAEP-only)",
				MessageEndpoint:               server.URL,
				PublicKeyJWKS:                 jwks,
				SupportedEncryptionAlgorithms: []string{"RSA-OAEP"},
			},
		},
	}

	worker := NewWorker(msgRepo, partnerRepo, engine)
	worker.SetNodeID("urn:test:sender")
	msg := createTestMessage("msg-fid4-legacy", "partner-legacy")

	err := worker.deliverMessage(context.Background(), msg)
	require.NoError(t, err)

	capturedMu.Lock()
	defer capturedMu.Unlock()
	assert.Contains(t, capturedAlg, `"alg":"RSA-OAEP"`)
	assert.NotContains(t, capturedAlg, `"alg":"RSA-OAEP-256"`,
		"worker must NOT pick RSA-OAEP-256 when the peer only advertises RSA-OAEP")
}

// TestWorker_DeliverMessage_NegotiatesPlainOAEPForUnadvertisedLegacyPartner
// covers the pre-FID-4 partner case: the database row was hydrated before
// SupportedEncryptionAlgorithms existed, so the field is nil. The worker
// must treat that as "legacy peer, assume RSA-OAEP" — never refuse the
// delivery.
func TestWorker_DeliverMessage_NegotiatesPlainOAEPForUnadvertisedLegacyPartner(t *testing.T) {
	engine, jwks := makeTestEngineAndJWKS(t, "peer-prefid4")

	var capturedAlg string
	var capturedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var envelope struct {
			Payload string `json:"encrypted_payload"`
		}
		_ = json.Unmarshal(body, &envelope)
		parts := strings.Split(envelope.Payload, ".")
		header, _ := base64.RawURLEncoding.DecodeString(parts[0])
		capturedMu.Lock()
		capturedAlg = string(header)
		capturedMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	msgRepo := &mockMessageRepository{}
	partnerRepo := &mockPartnerRepository{
		partners: map[string]*domain.Partner{
			"partner-prefid4": {
				PartnerID:       "partner-prefid4",
				Name:            "Pre-FID-4 Partner (nil array)",
				MessageEndpoint: server.URL,
				PublicKeyJWKS:   jwks,
				// SupportedEncryptionAlgorithms intentionally nil.
			},
		},
	}

	worker := NewWorker(msgRepo, partnerRepo, engine)
	worker.SetNodeID("urn:test:sender")
	msg := createTestMessage("msg-fid4-prefid4", "partner-prefid4")

	require.NoError(t, worker.deliverMessage(context.Background(), msg))

	capturedMu.Lock()
	defer capturedMu.Unlock()
	assert.Contains(t, capturedAlg, `"alg":"RSA-OAEP"`)
	assert.NotContains(t, capturedAlg, `"alg":"RSA-OAEP-256"`)
}
