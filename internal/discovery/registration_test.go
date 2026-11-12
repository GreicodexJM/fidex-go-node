package discovery

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/crypto"
	"fidex-node/internal/netguard"

	"github.com/go-jose/go-jose/v4"
)

// ── helpers ──────────────────────────────────────────────────────────────

// testPeer is a fake FideX node: an RSA key, and an HTTP server publishing its
// AS5 configuration and JWKS. Requests are counted so tests can assert that a
// responder fetched nothing.
type testPeer struct {
	nodeID   string
	engine   *crypto.AS5Engine
	server   *httptest.Server
	requests atomic.Int32
	// configNodeID overrides the node_id published in the AS5 config.
	configNodeID string
	// register, when set, serves POST /api/v1/register.
	register http.HandlerFunc
	// lastAS5Query records the raw query of the last AS5 config GET.
	lastAS5Query atomic.Value
}

func newTestPeer(t *testing.T, nodeID string) *testPeer {
	t.Helper()
	priv, _, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	engine, err := crypto.NewAS5Engine(priv, nodeID)
	if err != nil {
		t.Fatalf("NewAS5Engine: %v", err)
	}
	p := &testPeer{nodeID: nodeID, engine: engine}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.requests.Add(1)
		switch r.URL.Path {
		case constants.RouteAS5Configuration:
			p.lastAS5Query.Store(r.URL.RawQuery)
			cfg := GenerateAS5Config(NodeConfig{NodeID: p.nodeID, OrganizationName: "Org " + p.nodeID, BaseURL: p.server.URL})
			if p.configNodeID != "" {
				cfg.NodeID = p.configNodeID
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cfg)
		case constants.RouteJWKS:
			jwks, err := p.engine.ExportJWKS(crypto.JWKSKeyIDBase(p.nodeID))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(jwks))
		case constants.RouteRegister:
			if p.register != nil {
				p.register(w, r)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *testPeer) as5URL() string { return p.server.URL + constants.RouteAS5Configuration }

// sign builds the registration JWS this peer would send.
func (p *testPeer) sign(t *testing.T, claims RegistrationClaims, kid string) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jws, err := p.engine.SignCompactJWS(payload, kid)
	if err != nil {
		t.Fatalf("SignCompactJWS: %v", err)
	}
	return jws
}

// claimsFor returns valid registration claims from p, carrying token.
func (p *testPeer) claimsFor(token string, ts time.Time) RegistrationClaims {
	return RegistrationClaims{
		FidexVersion:          FidexVersion,
		InitiatorNodeID:       p.nodeID,
		InitiatorAS5ConfigURL: p.as5URL(),
		SecurityToken:         token,
		Timestamp:             ts.UTC().Format(time.RFC3339),
	}
}

// newResponder builds a responder DiscoveryService over a fresh SQLite DB.
func newResponder(t *testing.T) (*DiscoveryService, *testEnv) {
	t.Helper()
	env := setupTestDB(t)
	t.Cleanup(env.cleanup)
	ds := NewDiscoveryService(NodeConfig{
		NodeID:           "urn:gln:test:responder",
		OrganizationName: "Responder Corp",
		BaseURL:          "https://responder.example.com",
		// The fake peers below are httptest servers on loopback.
		AllowPrivateNetworks: true,
	}, NewTokenStore(), env.partnerRepo, nil)
	return ds, env
}

func mintToken(t *testing.T, ds *DiscoveryService) string {
	t.Helper()
	inv, _, err := ds.MintInvitation()
	if err != nil {
		t.Fatalf("MintInvitation: %v", err)
	}
	tok, err := TokenFromDiscoveryURL(inv)
	if err != nil || tok == "" {
		t.Fatalf("TokenFromDiscoveryURL(%q) = %q, %v", inv, tok, err)
	}
	return tok
}

func wantRefusal(t *testing.T, rerr *RegistrationError, status int, code string) {
	t.Helper()
	if rerr == nil {
		t.Fatalf("expected refusal %d %s, got success", status, code)
	}
	if rerr.HTTPStatus != status || rerr.Code != code {
		t.Fatalf("refusal = %d %s (%v), want %d %s", rerr.HTTPStatus, rerr.Code, rerr, status, code)
	}
}

// ── tokens and invitation URLs ───────────────────────────────────────────

func TestTokenFromDiscoveryURL(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://b.example.com/.well-known/as5-configuration?token=abc_DEF-123", "abc_DEF-123", false},
		{"https://b.example.com/discover/x?foo=1&token=t0k", "t0k", false},
		{"https://b.example.com/.well-known/as5-configuration", "", false},
		{"not a url", "", true},
		{"/relative?token=x", "", true},
		{"ftp://b.example.com/?token=x", "", true},
	}
	for _, c := range cases {
		got, err := TokenFromDiscoveryURL(c.in)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("TokenFromDiscoveryURL(%q) = %q, %v; want %q, err=%v", c.in, got, err, c.want, c.wantErr)
		}
	}
}

func TestMintInvitationCarriesOpenSingleUseToken(t *testing.T) {
	ds, _ := newResponder(t)
	inv, exp, err := ds.MintInvitation()
	if err != nil {
		t.Fatalf("MintInvitation: %v", err)
	}
	if !strings.HasPrefix(inv, "https://responder.example.com"+constants.RouteAS5Configuration+"?token=") {
		t.Fatalf("invitation URL %q is not the AS5 config URL with ?token=", inv)
	}
	if d := time.Until(exp); d < InvitationTTL-time.Minute || d > InvitationTTL+time.Minute {
		t.Errorf("expires_at %v is not ~%v from now", exp, InvitationTTL)
	}
	tok, _ := TokenFromDiscoveryURL(inv)
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw)*8 < 128 {
		t.Fatalf("token %q: want ≥128 bits of unpadded base64url, got %d bits (%v)", tok, len(raw)*8, err)
	}
	if !ds.InvitationOpen(tok) {
		t.Fatal("freshly minted token should be open")
	}
	if !ds.InvitationOpen(tok) {
		t.Fatal("InvitationOpen must not consume the token")
	}
	if !ds.tokenStore.ValidateAndConsume(tok) || ds.InvitationOpen(tok) {
		t.Fatal("token should be spendable once and closed afterwards")
	}
}

// ── initiator side ───────────────────────────────────────────────────────

func TestInitiatorSendsSignedRegistrationWithTokenFromURL(t *testing.T) {
	responder := newTestPeer(t, "urn:gln:test:node-b")
	var gotContentType, gotBody string
	responder.register = func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotContentType, gotBody = r.Header.Get("Content-Type"), string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RegistrationResponse{
			Status: "registered", ResponderNodeID: "urn:gln:test:node-b",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}

	env := setupTestDB(t)
	defer env.cleanup()
	priv, _, _ := crypto.GenerateKeyPair()
	initiatorEngine, _ := crypto.NewAS5Engine(priv, "urn:gln:test:node-a")
	nodeA := NodeConfig{
		NodeID:       "urn:gln:test:node-a",
		BaseURL:      "https://node-a.example.com",
		SigningKeyID: crypto.SigningKeyID("urn:gln:test:node-a"),
		// The fake responder is an httptest server on loopback.
		AllowPrivateNetworks: true,
	}
	ds := NewDiscoveryService(nodeA, NewTokenStore(), env.partnerRepo, initiatorEngine)

	const token = "Zm9vYmFyYmF6cXV4cXV1eGNvcmdlZ3JhdWx0"
	partner, err := ds.InitiatePartnerHandshake(context.Background(), responder.as5URL()+"?token="+token)
	if err != nil {
		t.Fatalf("InitiatePartnerHandshake: %v", err)
	}

	// Phase 1 fetched the AS5 config with the URL as given (token included).
	if q, _ := responder.lastAS5Query.Load().(string); q != "token="+token {
		t.Errorf("AS5 config fetched with query %q, want the invitation's token", q)
	}

	// Phase 2: application/jose, RS256 JWS with kid, signed by the initiator.
	if gotContentType != ContentTypeJOSE {
		t.Errorf("Content-Type = %q, want %q", gotContentType, ContentTypeJOSE)
	}
	if strings.HasPrefix(strings.TrimSpace(gotBody), "{") || strings.Count(gotBody, ".") != 2 {
		t.Fatalf("body is not a JWS compact serialization: %q", gotBody)
	}
	jws, err := jose.ParseSignedCompact(gotBody, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatalf("ParseSignedCompact: %v", err)
	}
	if kid := jws.Signatures[0].Protected.KeyID; kid != nodeA.SigningKeyID {
		t.Errorf("kid = %q, want %q", kid, nodeA.SigningKeyID)
	}
	jwksA, _ := initiatorEngine.ExportJWKS(crypto.JWKSKeyIDBase(nodeA.NodeID))
	pubA, err := crypto.ParsePublicKeyFromJWKSByUse(jwksA, "sig")
	if err != nil {
		t.Fatalf("initiator public key: %v", err)
	}
	payload, err := jws.Verify(pubA)
	if err != nil {
		t.Fatalf("JWS does not verify with the initiator's public key: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("payload: %v", err)
	}
	want := map[string]any{
		"fidex_version":            "1.0",
		"initiator_node_id":        "urn:gln:test:node-a",
		"initiator_as5_config_url": "https://node-a.example.com" + constants.RouteAS5Configuration,
		"security_token":           token,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("payload[%s] = %v, want %v", k, got[k], v)
		}
	}
	for _, legacy := range []string{"node_id", "organization_name", "jwks_uri", "message_endpoint"} {
		if _, ok := got[legacy]; ok {
			t.Errorf("payload still carries legacy field %q", legacy)
		}
	}
	ts, err := time.Parse(time.RFC3339, got["timestamp"].(string))
	if err != nil || time.Since(ts).Abs() > time.Minute {
		t.Errorf("timestamp %v is not a current RFC 3339 time (%v)", got["timestamp"], err)
	}

	// Phase 4: responder stored as partner.
	if partner.PartnerID != "urn:gln:test:node-b" || partner.MessageEndpoint != responder.server.URL+constants.RouteInbound {
		t.Errorf("unexpected partner %+v", partner)
	}
	if _, err := env.partnerRepo.GetByID(context.Background(), "urn:gln:test:node-b"); err != nil {
		t.Errorf("responder not stored: %v", err)
	}
}

func TestInitiatorSurfacesRefusalAndStoresNothing(t *testing.T) {
	responder := newTestPeer(t, "urn:gln:test:node-b")
	responder.register = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(NewErrorResponse(CodeInvalidToken, "Security token is invalid or expired", "", time.Now()))
	}
	env := setupTestDB(t)
	defer env.cleanup()
	priv, _, _ := crypto.GenerateKeyPair()
	engine, _ := crypto.NewAS5Engine(priv, "a")
	ds := NewDiscoveryService(NodeConfig{NodeID: "a", BaseURL: "https://a.example.com", SigningKeyID: crypto.SigningKeyID("a"), AllowPrivateNetworks: true},
		NewTokenStore(), env.partnerRepo, engine)

	_, err := ds.InitiatePartnerHandshake(context.Background(), responder.as5URL()+"?token=x")
	var refused *RegistrationRefusedError
	if err == nil || !errors.As(err, &refused) || refused.Code != CodeInvalidToken || refused.HTTPStatus != 401 {
		t.Fatalf("want RegistrationRefusedError 401 INVALID_TOKEN, got %v", err)
	}
	if n, _ := env.partnerRepo.Count(context.Background()); n != 0 {
		t.Errorf("refused registration stored %d partners", n)
	}
}

func TestInitiatorRejectsResponderNodeIDMismatch(t *testing.T) {
	responder := newTestPeer(t, "urn:gln:test:node-b")
	responder.register = func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(RegistrationResponse{Status: "registered", ResponderNodeID: "urn:gln:test:impostor", Timestamp: "2026-01-01T00:00:00Z"})
	}
	env := setupTestDB(t)
	defer env.cleanup()
	priv, _, _ := crypto.GenerateKeyPair()
	engine, _ := crypto.NewAS5Engine(priv, "a")
	ds := NewDiscoveryService(NodeConfig{NodeID: "a", BaseURL: "https://a.example.com", AllowPrivateNetworks: true}, NewTokenStore(), env.partnerRepo, engine)
	if _, err := ds.InitiatePartnerHandshake(context.Background(), responder.as5URL()+"?token=x"); err == nil || !strings.Contains(err.Error(), "responder_node_id") {
		t.Fatalf("want responder_node_id mismatch error, got %v", err)
	}
}

// ── responder side ───────────────────────────────────────────────────────

func TestResponderRegistersValidSignedRequest(t *testing.T) {
	ds, env := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	token := mintToken(t, ds)

	jws := initiator.sign(t, initiator.claimsFor(token, time.Now()), crypto.SigningKeyID(initiator.nodeID))
	resp, rerr := ds.HandleRegistration(context.Background(), jws)
	if rerr != nil {
		t.Fatalf("HandleRegistration: %v", rerr)
	}
	if resp.Status != "registered" || resp.ResponderNodeID != "urn:gln:test:responder" {
		t.Errorf("unexpected response %+v", resp)
	}
	if _, err := time.Parse(time.RFC3339, resp.Timestamp); err != nil {
		t.Errorf("response timestamp %q not RFC 3339", resp.Timestamp)
	}
	p, err := env.partnerRepo.GetByID(context.Background(), "urn:gln:test:initiator")
	if err != nil {
		t.Fatalf("initiator not stored: %v", err)
	}
	if p.MessageEndpoint != initiator.server.URL+constants.RouteInbound || p.JWKSUrl != initiator.server.URL+constants.RouteJWKS || p.PublicKeyJWKS == "" {
		t.Errorf("stored partner lacks the initiator's published details: %+v", p)
	}
}

func TestResponderRejectsReusedToken(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	token := mintToken(t, ds)
	jws := initiator.sign(t, initiator.claimsFor(token, time.Now()), crypto.SigningKeyID(initiator.nodeID))

	if _, rerr := ds.HandleRegistration(context.Background(), jws); rerr != nil {
		t.Fatalf("first registration: %v", rerr)
	}
	before := initiator.requests.Load()
	_, rerr := ds.HandleRegistration(context.Background(), jws) // exact replay
	wantRefusal(t, rerr, 401, CodeInvalidToken)
	if initiator.requests.Load() != before {
		t.Error("a replay with a spent token made the responder fetch the initiator's resources")
	}
}

func TestResponderValidatesTokenBeforeFetchingAnything(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	for name, token := range map[string]string{"unknown": "never-minted-token-value", "missing": ""} {
		jws := initiator.sign(t, initiator.claimsFor(token, time.Now()), crypto.SigningKeyID(initiator.nodeID))
		_, rerr := ds.HandleRegistration(context.Background(), jws)
		wantRefusal(t, rerr, 401, CodeInvalidToken)
		if n := initiator.requests.Load(); n != 0 {
			t.Errorf("%s token: responder made %d request(s) to the initiator before validating the token", name, n)
		}
	}
}

func TestResponderTimestampWindow(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	kid := crypto.SigningKeyID(initiator.nodeID)
	fixed := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	ds.now = func() time.Time { return fixed }

	for _, off := range []time.Duration{-16 * time.Minute, 16 * time.Minute} {
		token := mintToken(t, ds)
		_, rerr := ds.HandleRegistration(context.Background(), initiator.sign(t, initiator.claimsFor(token, fixed.Add(off)), kid))
		wantRefusal(t, rerr, 401, CodeSignatureInvalid)
		if rerr.Field != "timestamp" {
			t.Errorf("offset %v: field = %q, want timestamp", off, rerr.Field)
		}
		if !ds.InvitationOpen(token) {
			t.Errorf("offset %v: a stale request burned the token", off)
		}
	}
	if n := initiator.requests.Load(); n != 0 {
		t.Errorf("stale requests made %d fetch(es)", n)
	}

	// Inside the window, both directions, boundary included (a distinct
	// initiator each time so the second is not ALREADY_REGISTERED).
	for i, off := range []time.Duration{-15 * time.Minute, 14 * time.Minute} {
		peer := newTestPeer(t, "urn:gln:test:in-window-"+string(rune('a'+i)))
		token := mintToken(t, ds)
		if _, rerr := ds.HandleRegistration(context.Background(), peer.sign(t, peer.claimsFor(token, fixed.Add(off)), crypto.SigningKeyID(peer.nodeID))); rerr != nil {
			t.Errorf("offset %v should be accepted: %v", off, rerr)
		}
	}
}

func TestResponderRejectsNodeIDMismatch(t *testing.T) {
	ds, env := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	initiator.configNodeID = "urn:gln:test:somebody-else"
	token := mintToken(t, ds)

	_, rerr := ds.HandleRegistration(context.Background(), initiator.sign(t, initiator.claimsFor(token, time.Now()), crypto.SigningKeyID(initiator.nodeID)))
	wantRefusal(t, rerr, 401, CodeSignatureInvalid)
	if ds.InvitationOpen(token) {
		t.Error("token should stay spent after a refusal past the token check")
	}
	if n, _ := env.partnerRepo.Count(context.Background()); n != 0 {
		t.Errorf("node_id mismatch stored %d partner(s)", n)
	}
}

func TestResponderRejectsBadSignature(t *testing.T) {
	ds, env := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	impostor := newTestPeer(t, "urn:gln:test:impostor")
	token := mintToken(t, ds)

	// Claims name the initiator, but the impostor's key signed them.
	jws := impostor.sign(t, initiator.claimsFor(token, time.Now()), crypto.SigningKeyID(initiator.nodeID))
	_, rerr := ds.HandleRegistration(context.Background(), jws)
	wantRefusal(t, rerr, 401, CodeSignatureInvalid)
	if n, _ := env.partnerRepo.Count(context.Background()); n != 0 {
		t.Errorf("bad signature stored %d partner(s)", n)
	}
}

func TestResponderTamperedPayload(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	token := mintToken(t, ds)
	jws := initiator.sign(t, initiator.claimsFor(token, time.Now()), crypto.SigningKeyID(initiator.nodeID))

	parts := strings.Split(jws, ".")
	c := initiator.claimsFor(token, time.Now())
	c.ContactEmail = "attacker@example.com"
	b, _ := json.Marshal(c)
	parts[1] = base64.RawURLEncoding.EncodeToString(b)
	_, rerr := ds.HandleRegistration(context.Background(), strings.Join(parts, "."))
	wantRefusal(t, rerr, 401, CodeSignatureInvalid)
}

func TestResponderUnknownKid(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	token := mintToken(t, ds)
	_, rerr := ds.HandleRegistration(context.Background(), initiator.sign(t, initiator.claimsFor(token, time.Now()), "no-such-kid"))
	wantRefusal(t, rerr, 401, CodeUnknownKeyID)
}

func TestResponderAcceptsJWSWithoutKid(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	token := mintToken(t, ds)
	if _, rerr := ds.HandleRegistration(context.Background(), initiator.sign(t, initiator.claimsFor(token, time.Now()), "")); rerr != nil {
		t.Fatalf("a JWS without kid should verify against the initiator's signing keys: %v", rerr)
	}
}

func TestResponderAlreadyRegistered(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	kid := crypto.SigningKeyID(initiator.nodeID)
	if _, rerr := ds.HandleRegistration(context.Background(), initiator.sign(t, initiator.claimsFor(mintToken(t, ds), time.Now()), kid)); rerr != nil {
		t.Fatalf("first registration: %v", rerr)
	}
	_, rerr := ds.HandleRegistration(context.Background(), initiator.sign(t, initiator.claimsFor(mintToken(t, ds), time.Now()), kid))
	wantRefusal(t, rerr, 409, CodeAlreadyRegistered)
}

func TestResponderConfigUnreachable(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	token := mintToken(t, ds)
	c := initiator.claimsFor(token, time.Now())
	c.InitiatorAS5ConfigURL = initiator.server.URL + "/nope"
	_, rerr := ds.HandleRegistration(context.Background(), initiator.sign(t, c, crypto.SigningKeyID(initiator.nodeID)))
	wantRefusal(t, rerr, 400, CodeConfigUnreachable)
}

func TestResponderRejectsMalformedBodies(t *testing.T) {
	ds, _ := newResponder(t)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	token := mintToken(t, ds)
	kid := crypto.SigningKeyID(initiator.nodeID)

	legacyJSON := `{"node_id":"x","organization_name":"y","jwks_uri":"https://x/jwks","security_token":"` + token + `"}`
	noVersion := initiator.claimsFor(token, time.Now())
	noVersion.FidexVersion = ""
	badVersion := initiator.claimsFor(token, time.Now())
	badVersion.FidexVersion = "9.9"
	badURL := initiator.claimsFor(token, time.Now())
	badURL.InitiatorAS5ConfigURL = "not-a-url"
	badTS := initiator.claimsFor(token, time.Now())
	badTS.Timestamp = "11/11/2026 12:00"

	cases := map[string]string{
		"legacy unsigned JSON": legacyJSON,
		"garbage":              "a.b",
		"no fidex_version":     initiator.sign(t, noVersion, kid),
		"bad fidex_version":    initiator.sign(t, badVersion, kid),
		"bad config url":       initiator.sign(t, badURL, kid),
		"bad timestamp":        initiator.sign(t, badTS, kid),
	}
	for name, body := range cases {
		_, rerr := ds.HandleRegistration(context.Background(), body)
		if rerr == nil || rerr.HTTPStatus != 400 {
			t.Errorf("%s: want 400, got %v", name, rerr)
		}
	}
	if !ds.InvitationOpen(token) {
		t.Error("malformed requests must not burn the token")
	}
	if n := initiator.requests.Load(); n != 0 {
		t.Errorf("malformed requests made %d fetch(es)", n)
	}
}

func TestInitiatorErrorsDoNotLeakToken(t *testing.T) {
	env := setupTestDB(t)
	defer env.cleanup()
	priv, _, _ := crypto.GenerateKeyPair()
	engine, _ := crypto.NewAS5Engine(priv, "a")
	ds := NewDiscoveryService(NodeConfig{NodeID: "a", BaseURL: "https://a.example.com", AllowPrivateNetworks: true}, NewTokenStore(), env.partnerRepo, engine)

	const secret = "s3cr3t-invitation-token-value"
	// Port 1 on loopback refuses: net/http's error quotes the full URL.
	_, err := ds.InitiatePartnerHandshake(context.Background(), "http://127.0.0.1:1/.well-known/as5-configuration?token="+secret)
	if err == nil {
		t.Fatal("expected a fetch error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks the token: %v", err)
	}

	responder := newTestPeer(t, "urn:gln:test:node-b")
	responder.register = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(NewErrorResponse(CodeInvalidToken, "nope", "", time.Now()))
	}
	_, err = ds.InitiatePartnerHandshake(context.Background(), responder.as5URL()+"?token="+secret)
	var refused *RegistrationRefusedError
	if !errors.As(err, &refused) || strings.Contains(err.Error(), secret) {
		t.Fatalf("want a redacted RegistrationRefusedError, got %v", err)
	}

	if got := RedactDiscoveryURL("https://b.example.com/x?a=1&token=" + secret); strings.Contains(got, secret) || !strings.Contains(got, "token=REDACTED") {
		t.Fatalf("RedactDiscoveryURL = %q", got)
	}
}

// With private networks NOT allowed (the production default) a responder
// refuses to fetch an initiator that lives on loopback — after spending the
// token, and without storing anything.
func TestResponderGuardRefusesPrivateInitiatorURL(t *testing.T) {
	env := setupTestDB(t)
	defer env.cleanup()
	ds := NewDiscoveryService(NodeConfig{NodeID: "r", BaseURL: "https://r.example.com"}, NewTokenStore(), env.partnerRepo, nil)
	initiator := newTestPeer(t, "urn:gln:test:initiator")
	token := mintToken(t, ds)

	_, rerr := ds.HandleRegistration(context.Background(), initiator.sign(t, initiator.claimsFor(token, time.Now()), crypto.SigningKeyID(initiator.nodeID)))
	wantRefusal(t, rerr, 400, CodeConfigUnreachable)
	if n := initiator.requests.Load(); n != 0 {
		t.Errorf("guarded responder reached a loopback initiator (%d requests)", n)
	}
	if !errors.Is(rerr.Err, netguard.ErrBlockedAddress) {
		t.Errorf("cause = %v, want netguard.ErrBlockedAddress", rerr.Err)
	}
}

func TestValidateAS5ConfigRejectsNonHTTPEndpoints(t *testing.T) {
	cfg := GenerateAS5Config(NodeConfig{NodeID: "x", BaseURL: "https://x.example.com"})
	cfg.Endpoints.ReceiveMessage = "file:///etc/passwd"
	if err := validateAS5Config(cfg); err == nil {
		t.Fatal("a file: endpoint must be refused")
	}
	cfg = GenerateAS5Config(NodeConfig{NodeID: "x", BaseURL: "https://x.example.com"})
	cfg.Endpoints.JWKS = "https://user:pw@x.example.com/jwks"
	if err := validateAS5Config(cfg); err == nil {
		t.Fatal("an endpoint with userinfo must be refused")
	}
}
