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
}

func newE2ENode(t *testing.T, nodeID string) *e2eNode {
	t.Helper()
	db, err := repository.OpenSQLite(filepath.Join(t.TempDir(), "node.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, repository.InitSchema(db))
	partners := repository.NewSQLitePartnerRepository(db)

	priv, _, err := crypto.GenerateKeyPair()
	require.NoError(t, err)
	engine, err := crypto.NewAS5Engine(priv, nodeID)
	require.NoError(t, err)

	n := &e2eNode{h: &Handlers{
		Config:        &config.Config{NodeID: nodeID, OrganizationName: "Org " + nodeID, PublicDomain: "localhost"},
		PartnerRepo:   partners,
		CryptoService: engine,
	}}
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
	}, discovery.NewTokenStore(), partners, engine)
	publicRouter = n.h.SetupPublicRouter()

	n.dashboard = httptest.NewServer(n.h.SetupDashboardRouter())
	t.Cleanup(n.dashboard.Close)
	return n
}

func (n *e2eNode) invitation(t *testing.T) string {
	t.Helper()
	resp, err := http.Post(n.dashboard.URL+constants.RouteInvitations, "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var inv InvitationResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&inv))
	return inv.DiscoveryURL
}

func (n *e2eNode) discover(t *testing.T, discoveryURL string) (int, string) {
	t.Helper()
	body := `{"discovery_url":"` + discoveryURL + `"}`
	resp, err := http.Post(n.dashboard.URL+constants.RouteDiscover, "application/json", strings.NewReader(body))
	require.NoError(t, err)
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
	resp, err := http.Get(b.dashboard.URL + constants.RouteDashboardQR)
	require.NoError(t, err)
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
