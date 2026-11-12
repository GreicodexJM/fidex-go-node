package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fidex-node/internal/auth"
	"fidex-node/internal/config"
	"fidex-node/internal/constants"
	"fidex-node/internal/crypto"
	"fidex-node/internal/discovery"
	"fidex-node/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// e2eNode is one FideX node wired like main.go does it: real SQLite partner
// repository, real crypto engine, real discovery service, and the public
// router (register, AS5 config, JWKS) plus the dashboard router served over
// HTTP.
type e2eNode struct {
	h         *Handlers
	public    *httptest.Server
	dashboard *httptest.Server
	session   string // a logged-in admin's session cookie value
}

func newE2ENode(t *testing.T, nodeID string) *e2eNode {
	t.Helper()
	db, err := repository.OpenSQLite(filepath.Join(t.TempDir(), "node.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.InitSchema(db))
	partners := repository.NewSQLitePartnerRepository(db)
	authSvc := auth.NewService(repository.NewSQLiteSessionRepository(db), repository.NewSQLiteUserRepository(db))
	hash, err := auth.HashPassword("e2e-password")
	require.NoError(t, err)
	user, err := authSvc.CreateUser(context.Background(), "admin", hash)
	require.NoError(t, err)
	sess, err := authSvc.CreateSession(context.Background(), user.ID)
	require.NoError(t, err)

	priv, _, err := crypto.GenerateKeyPair()
	require.NoError(t, err)
	engine, err := crypto.NewAS5Engine(priv, nodeID)
	require.NoError(t, err)

	n := &e2eNode{h: &Handlers{
		Config:        &config.Config{NodeID: nodeID, OrganizationName: "Org " + nodeID, PublicDomain: "localhost"},
		PartnerRepo:   partners,
		CryptoService: engine,
		AuthService:   authSvc,
	}, session: sess.SessionID}
	// The public URL is only known once the server listens; route through n.h.
	var publicRouter http.Handler
	n.public = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { publicRouter.ServeHTTP(w, r) }))
	t.Cleanup(n.public.Close)

	n.h.Config.PublicBaseURL = n.public.URL
	n.h.DiscoveryService = discovery.NewDiscoveryService(discovery.NodeConfig{
		NodeID:           nodeID,
		OrganizationName: "Org " + nodeID,
		BaseURL:          n.h.Config.ResolvedPublicBaseURL(),
		SigningKeyID:     crypto.SigningKeyID(nodeID),
		// Both nodes are httptest servers on loopback.
		AllowPrivateNetworks: true,
	}, discovery.NewTokenStore(), partners, engine)
	publicRouter = n.h.SetupPublicRouter()

	n.dashboard = httptest.NewServer(n.h.SetupDashboardRouter())
	t.Cleanup(n.dashboard.Close)
	return n
}

// do sends a dashboard request as the logged-in admin.
func (n *e2eNode) do(t *testing.T, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, n.dashboard.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: n.session})
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func (n *e2eNode) invitation(t *testing.T) string {
	t.Helper()
	resp := n.do(t, http.MethodPost, constants.RouteInvitations, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var inv InvitationResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&inv))
	return inv.DiscoveryURL
}

func (n *e2eNode) discover(t *testing.T, discoveryURL string) (int, string) {
	t.Helper()
	resp := n.do(t, http.MethodPost, constants.RouteDiscover, `{"discovery_url":"`+discoveryURL+`"}`)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func decodeFidexError(t *testing.T, resp *http.Response) discovery.ErrorBody {
	t.Helper()
	var er discovery.ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&er))
	require.NotEmpty(t, er.Error.Timestamp)
	return er.Error
}

// One handshake (spec §6.3) leaves BOTH nodes knowing each other, using the
// responder's minted invitation token; the token is single-use.
func TestE2E_RegistrationByInvitation(t *testing.T) {
	a := newE2ENode(t, "urn:gln:test:node-a")
	b := newE2ENode(t, "urn:gln:test:node-b")
	ctx := context.Background()

	inv := b.invitation(t)
	assert.True(t, strings.HasPrefix(inv, b.public.URL+constants.RouteAS5Configuration+"?token="), inv)

	status, body := a.discover(t, inv)
	require.Equal(t, http.StatusOK, status, body)
	assert.Contains(t, body, "successfully registered")

	pa, err := a.h.PartnerRepo.GetByID(ctx, "urn:gln:test:node-b")
	require.NoError(t, err, "initiator must store the responder")
	assert.Equal(t, b.public.URL+constants.RouteInbound, pa.MessageEndpoint)
	pb, err := b.h.PartnerRepo.GetByID(ctx, "urn:gln:test:node-a")
	require.NoError(t, err, "responder must store the initiator")
	assert.Equal(t, a.public.URL+constants.RouteInbound, pb.MessageEndpoint)
	assert.Equal(t, a.public.URL+constants.RouteJWKS, pb.JWKSUrl)

	// The invitation is spent: fetching the AS5 config with it is now 401,
	// and a third node presenting it cannot register.
	resp, err := http.Get(inv)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, discovery.CodeInvalidToken, decodeFidexError(t, resp).Code)

	c := newE2ENode(t, "urn:gln:test:node-c")
	status, body = c.discover(t, inv)
	assert.NotEqual(t, http.StatusOK, status, body)
	_, err = b.h.PartnerRepo.GetByID(ctx, "urn:gln:test:node-c")
	assert.Error(t, err, "a reused token must not register anyone")
}

// The QR endpoint mints a token: the PNG's URL (also in a header) is open.
func TestE2E_QRCarriesMintedToken(t *testing.T) {
	b := newE2ENode(t, "urn:gln:test:node-b")
	resp := b.do(t, http.MethodGet, constants.RouteDashboardQR, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "image/png", resp.Header.Get("Content-Type"))
	u := resp.Header.Get("X-FideX-Discovery-URL")
	tok, err := discovery.TokenFromDiscoveryURL(u)
	require.NoError(t, err)
	require.NotEmpty(t, tok, "QR URL must carry ?token=")
	assert.True(t, b.h.DiscoveryService.InvitationOpen(tok))

	cfgResp, err := http.Get(u)
	require.NoError(t, err)
	defer cfgResp.Body.Close()
	assert.Equal(t, http.StatusOK, cfgResp.StatusCode, "an open token serves the AS5 config")
	assert.True(t, b.h.DiscoveryService.InvitationOpen(tok), "GET must not spend the token")
}

func TestE2E_AS5ConfigTokenGate(t *testing.T) {
	b := newE2ENode(t, "urn:gln:test:node-b")

	resp, err := http.Get(b.public.URL + constants.RouteAS5Configuration)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "plain discovery stays public")

	resp, err = http.Get(b.public.URL + constants.RouteAS5Configuration + "?token=forged")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, discovery.CodeInvalidToken, decodeFidexError(t, resp).Code)
}

func TestE2E_RegisterRequiresJOSE(t *testing.T) {
	b := newE2ENode(t, "urn:gln:test:node-b")
	legacy := `{"node_id":"x","organization_name":"y","jwks_uri":"https://x/jwks","security_token":"t"}`
	resp, err := http.Post(b.public.URL+constants.RouteRegister, "application/json", strings.NewReader(legacy))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, discovery.CodeSignatureInvalid, decodeFidexError(t, resp).Code)

	resp2, err := http.Post(b.public.URL+constants.RouteRegister, "application/jose", strings.NewReader("not.a.jws"))
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp2.StatusCode)
}

// Token-minting and settings routes need a session: without one they answer
// 401 and mint nothing.
func TestE2E_DashboardAndSettingsRequireSession(t *testing.T) {
	b := newE2ENode(t, "urn:gln:test:node-b")
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, constants.RouteInvitations},
		{http.MethodGet, constants.RouteDashboardQR},
		{http.MethodPost, constants.RouteDiscover},
		{http.MethodGet, constants.RouteDashboardPartners},
	} {
		req, _ := http.NewRequest(c.method, b.dashboard.URL+c.path, strings.NewReader("{}"))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "%s %s without a session", c.method, c.path)

		req, _ = http.NewRequest(c.method, b.dashboard.URL+c.path, strings.NewReader("{}"))
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "forged"})
		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "%s %s with a forged session", c.method, c.path)
	}

	settings := httptest.NewServer(b.h.SetupSettingsRouter())
	defer settings.Close()
	resp, err := http.Get(settings.URL + constants.RouteUsers)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// Without an AuthService the protected routers fail closed.
func TestRequireSessionFailsClosedWithoutAuthService(t *testing.T) {
	h := &Handlers{}
	srv := httptest.NewServer(h.SetupDashboardRouter())
	defer srv.Close()
	resp, err := http.Post(srv.URL+constants.RouteInvitations, "application/json", nil)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
