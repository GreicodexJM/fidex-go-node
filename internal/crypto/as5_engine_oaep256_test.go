package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestSignAndEncryptWithAlg_RSAOAEP256_RoundTrip verifies a full
// sign+encrypt → decrypt+verify round-trip using RSA-OAEP-256 for the JWE
// key wrap step (FID-4 / ADR-0003).
func TestSignAndEncryptWithAlg_RSAOAEP256_RoundTrip(t *testing.T) {
	senderPrivPEM, senderPubPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(sender): %v", err)
	}
	receiverPrivPEM, receiverPubPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(receiver): %v", err)
	}

	senderEngine, err := NewAS5Engine(senderPrivPEM, "urn:gln:sender")
	if err != nil {
		t.Fatalf("NewAS5Engine(sender): %v", err)
	}
	receiverEngine, err := NewAS5Engine(receiverPrivPEM, "urn:gln:receiver")
	if err != nil {
		t.Fatalf("NewAS5Engine(receiver): %v", err)
	}

	senderPub, err := ParsePublicKeyFromPEM(senderPubPEM)
	if err != nil {
		t.Fatalf("ParsePublicKeyFromPEM(sender): %v", err)
	}
	receiverPub, err := ParsePublicKeyFromPEM(receiverPubPEM)
	if err != nil {
		t.Fatalf("ParsePublicKeyFromPEM(receiver): %v", err)
	}

	originalPayload := []byte(`{"orderNumber":"FID-4-OAEP256","items":[{"sku":"X","qty":1}]}`)

	jwe, err := senderEngine.SignAndEncryptWithAlg(originalPayload, receiverPub, AlgRSAOAEP256)
	if err != nil {
		t.Fatalf("SignAndEncryptWithAlg(RSA-OAEP-256): %v", err)
	}

	parts := strings.Split(jwe, ".")
	if len(parts) != 5 {
		t.Fatalf("expected 5-part JWE, got %d parts", len(parts))
	}

	// Sanity-check that the JWE protected header advertises the new alg.
	// We don't trust JSON ordering, just substring-match on the b64url
	// header — the test catches any silent fall-back to RSA-OAEP.
	if !strings.Contains(decodeJWEHeader(t, parts[0]), `"alg":"RSA-OAEP-256"`) {
		t.Errorf("JWE header missing alg=RSA-OAEP-256, decoded=%s", decodeJWEHeader(t, parts[0]))
	}

	decrypted, err := receiverEngine.DecryptAndVerify(jwe, senderPub)
	if err != nil {
		t.Fatalf("DecryptAndVerify(round-trip OAEP-256): %v", err)
	}
	if string(decrypted) != string(originalPayload) {
		t.Errorf("payload mismatch.\nwant: %s\n got: %s", originalPayload, decrypted)
	}
}

// TestSignAndEncryptWithAlg_BackCompatRSAOAEP confirms the alg-aware
// variant continues to emit a SHA-1 OAEP envelope when callers pass the
// legacy alg string (or empty string, which routes to the same default).
func TestSignAndEncryptWithAlg_BackCompatRSAOAEP(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	engine, err := NewAS5Engine(priv, "urn:gln:peer")
	if err != nil {
		t.Fatalf("NewAS5Engine: %v", err)
	}
	pk, err := ParsePublicKeyFromPEM(pub)
	if err != nil {
		t.Fatalf("ParsePublicKeyFromPEM: %v", err)
	}

	cases := []struct {
		name string
		alg  string
	}{
		{"explicit RSA-OAEP", AlgRSAOAEP},
		{"empty defaults to RSA-OAEP", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jwe, err := engine.SignAndEncryptWithAlg([]byte(`{"ok":true}`), pk, tc.alg)
			if err != nil {
				t.Fatalf("SignAndEncryptWithAlg(%q): %v", tc.alg, err)
			}
			parts := strings.Split(jwe, ".")
			if len(parts) != 5 {
				t.Fatalf("expected 5-part JWE, got %d", len(parts))
			}
			header := decodeJWEHeader(t, parts[0])
			if !strings.Contains(header, `"alg":"RSA-OAEP"`) {
				t.Errorf("expected alg=RSA-OAEP in JWE header, got %s", header)
			}
			if strings.Contains(header, `"alg":"RSA-OAEP-256"`) {
				t.Errorf("unexpected RSA-OAEP-256 in header; back-compat path must stay SHA-1")
			}
		})
	}
}

// TestSignAndEncryptWithAlg_Unknown rejects unknown algorithm names so a
// silent downgrade never happens.
func TestSignAndEncryptWithAlg_Unknown(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	engine, _ := NewAS5Engine(priv, "urn:gln:peer")
	pk, _ := ParsePublicKeyFromPEM(pub)

	_, err = engine.SignAndEncryptWithAlg([]byte("x"), pk, "RSA-NONSENSE")
	if err == nil {
		t.Fatal("expected error on unknown alg, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported encryption algorithm") {
		t.Errorf("expected 'unsupported encryption algorithm' in error, got %v", err)
	}
}

// TestSignAndEncrypt_LegacyEntryPointUnchanged guards that the original
// no-alg-argument entry point still emits an RSA-OAEP envelope. This is
// the explicit back-compat contract for callers that pre-date FID-4.
func TestSignAndEncrypt_LegacyEntryPointUnchanged(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	engine, _ := NewAS5Engine(priv, "urn:gln:peer")
	pk, _ := ParsePublicKeyFromPEM(pub)

	jwe, err := engine.SignAndEncrypt([]byte(`{"a":1}`), pk)
	if err != nil {
		t.Fatalf("SignAndEncrypt: %v", err)
	}
	parts := strings.Split(jwe, ".")
	header := decodeJWEHeader(t, parts[0])
	if !strings.Contains(header, `"alg":"RSA-OAEP"`) {
		t.Errorf("legacy SignAndEncrypt must keep alg=RSA-OAEP, got header=%s", header)
	}
}

// TestNegotiateEncryptionAlgorithm covers the negotiation matrix described
// in ADR-0003 §Decision.
func TestNegotiateEncryptionAlgorithm(t *testing.T) {
	local := LocalSupportedEncryptionAlgorithms()

	cases := []struct {
		name    string
		ours    []string
		theirs  []string
		want    string
		wantErr bool
	}{
		{
			name:   "both support both -> prefer 256",
			ours:   local,
			theirs: []string{AlgRSAOAEP, AlgRSAOAEP256},
			want:   AlgRSAOAEP256,
		},
		{
			name:   "both support both, peer lists 256 first",
			ours:   local,
			theirs: []string{AlgRSAOAEP256, AlgRSAOAEP},
			want:   AlgRSAOAEP256,
		},
		{
			name:   "peer only supports plain OAEP -> fall back",
			ours:   local,
			theirs: []string{AlgRSAOAEP},
			want:   AlgRSAOAEP,
		},
		{
			name:   "we only support plain OAEP, peer supports both",
			ours:   []string{AlgRSAOAEP},
			theirs: []string{AlgRSAOAEP, AlgRSAOAEP256},
			want:   AlgRSAOAEP,
		},
		{
			name:   "legacy peer with empty list -> fall back to RSA-OAEP",
			ours:   local,
			theirs: nil,
			want:   AlgRSAOAEP,
		},
		{
			name:   "peer only supports 256, we don't -> no overlap",
			ours:   []string{AlgRSAOAEP},
			theirs: []string{AlgRSAOAEP256},
			wantErr: true,
		},
		{
			name:   "peer advertises only unknown alg -> no overlap",
			ours:   local,
			theirs: []string{"RSA-OAEP-512"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NegotiateEncryptionAlgorithm(tc.ours, tc.theirs)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got alg=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("want alg=%q, got %q", tc.want, got)
			}
		})
	}
}

// decodeJWEHeader is a tiny test helper that base64url-decodes the JWE
// protected header (first compact-form segment) without pulling in
// go-jose's parser, so the assertion is decoupled from the library's
// internal canonicalisation.
func decodeJWEHeader(t *testing.T, segment string) string {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("decode JWE header segment: %v", err)
	}
	return string(decoded)
}
