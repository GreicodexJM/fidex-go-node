package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/domain"
	"fidex-node/internal/logging"
)

// logger is the package-level structured logger for discovery.
var logger = logging.New("discovery")

// Protocol constants for partner registration (spec §6.2–§6.4).
const (
	// FidexVersion is the protocol version this node speaks and puts in the
	// registration payload.
	FidexVersion = "1.0"

	// ContentTypeJOSE is the media type of a JWS compact serialization
	// (RFC 7515 §9.2.1). Spec §6.4: the registration body MUST use it.
	ContentTypeJOSE = "application/jose"

	// TimestampWindow is the ±skew a responder accepts on the registration
	// timestamp (spec §6.4).
	TimestampWindow = 15 * time.Minute

	// InvitationTTL is how long a minted registration token stays open
	// (annotated spec §6.3 recommends 30 minutes).
	InvitationTTL = 30 * time.Minute

	// TokenQueryParam is the query parameter that carries the security token
	// in an AS5 configuration URL (annotated spec §6.3: "?token=").
	TokenQueryParam = "token"

	// MaxRegistrationBytes bounds the registration body a responder reads.
	MaxRegistrationBytes = 64 << 10

	// userAgent identifies this node on outbound discovery calls.
	userAgent = "FideX-Node/1.0"
)

// RegistrationClaims is the registration payload the initiator signs (spec
// §6.3 Phase 2; openapi.yaml RegistrationRequest). It travels as the payload
// of a JWS compact serialization, never as raw JSON.
type RegistrationClaims struct {
	FidexVersion          string `json:"fidex_version"`
	InitiatorNodeID       string `json:"initiator_node_id"`
	InitiatorAS5ConfigURL string `json:"initiator_as5_config_url"`
	SecurityToken         string `json:"security_token,omitempty"`
	Timestamp             string `json:"timestamp"`
	ContactEmail          string `json:"contact_email,omitempty"`
}

// RegistrationResponse is the responder's 200 body (openapi.yaml
// RegistrationResponse).
type RegistrationResponse struct {
	Status                string `json:"status"`
	ResponderNodeID       string `json:"responder_node_id"`
	ResponderAS5ConfigURL string `json:"responder_as5_config_url,omitempty"`
	Message               string `json:"message,omitempty"`
	Timestamp             string `json:"timestamp"`
}

// RegistrationStatusRegistered is the only value of RegistrationResponse.Status.
const RegistrationStatusRegistered = "registered"

// ErrorResponse is openapi.yaml's ErrorResponse: {"error":{code,message,timestamp}}.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody is the inner object of ErrorResponse.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	Field     string `json:"field,omitempty"`
}

// RegistrationSigner signs the registration payload with this node's private
// key. *crypto.AS5Engine satisfies it.
type RegistrationSigner interface {
	SignCompactJWS(payload []byte, kid string) (string, error)
}

// DiscoveryService handles partner discovery and registration, both as the
// initiator (InitiatePartnerHandshake) and as the responder
// (HandleRegistration, MintInvitation, InvitationOpen).
type DiscoveryService struct {
	nodeConfig  NodeConfig
	tokenStore  *TokenStore
	partnerRepo domain.PartnerRepository
	signer      RegistrationSigner
	httpClient  *http.Client
	now         func() time.Time
}

// NewDiscoveryService creates a new discovery service. signer may be nil on a
// node that only ever acts as responder; InitiatePartnerHandshake then fails.
func NewDiscoveryService(nodeConfig NodeConfig, tokenStore *TokenStore, partnerRepo domain.PartnerRepository, signer RegistrationSigner) *DiscoveryService {
	return &DiscoveryService{
		nodeConfig:  nodeConfig,
		tokenStore:  tokenStore,
		partnerRepo: partnerRepo,
		signer:      signer,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		now:         time.Now,
	}
}

// OwnAS5ConfigURL is the absolute URL where partners fetch this node's AS5
// configuration. It is what this node sends as initiator_as5_config_url and
// what it encodes (plus a token) in its invitation QR.
func (ds *DiscoveryService) OwnAS5ConfigURL() string {
	return ds.nodeConfig.BaseURL + constants.RouteAS5Configuration
}

// MintInvitation mints a single-use registration token and returns this
// node's AS5 configuration URL carrying it as ?token=, ready to share by QR
// (spec §6.2: the URL "MAY include a single-use security token" and "SHOULD
// be shareable via QR code").
func (ds *DiscoveryService) MintInvitation() (invitationURL string, expiresAt time.Time, err error) {
	token, err := ds.tokenStore.GenerateToken(InvitationTTL)
	if err != nil {
		return "", time.Time{}, err
	}
	u, err := url.Parse(ds.OwnAS5ConfigURL())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid own AS5 configuration URL: %w", err)
	}
	q := u.Query()
	q.Set(TokenQueryParam, token)
	u.RawQuery = q.Encode()
	return u.String(), ds.now().Add(InvitationTTL), nil
}

// InvitationOpen reports whether token is an open (unused, unexpired)
// registration token, without spending it.
func (ds *DiscoveryService) InvitationOpen(token string) bool {
	return ds.tokenStore.IsOpen(token)
}

// TokenFromDiscoveryURL returns the security token carried in an AS5
// configuration URL's query (?token=), or "" when the URL carries none.
func TokenFromDiscoveryURL(discoveryURL string) (string, error) {
	u, err := url.Parse(discoveryURL)
	if err != nil {
		return "", fmt.Errorf("invalid discovery URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("invalid discovery URL: must be an absolute http(s) URL")
	}
	return u.Query().Get(TokenQueryParam), nil
}

// RedactDiscoveryURL returns discoveryURL with the value of its ?token=
// replaced by "REDACTED", for logs. Unparseable input is returned as
// "<invalid discovery URL>".
func RedactDiscoveryURL(discoveryURL string) string {
	u, err := url.Parse(discoveryURL)
	if err != nil {
		return "<invalid discovery URL>"
	}
	q := u.Query()
	if q.Has(TokenQueryParam) {
		q.Set(TokenQueryParam, "REDACTED")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// redactedError hides the invitation token in an error's text while keeping
// the chain for errors.As (e.g. *RegistrationRefusedError).
type redactedError struct {
	err   error
	token string
}

func (e *redactedError) Error() string { return strings.ReplaceAll(e.err.Error(), e.token, "REDACTED") }
func (e *redactedError) Unwrap() error { return e.err }

// RegistrationRefusedError is returned by InitiatePartnerHandshake when the
// responder answers the registration with an ErrorResponse.
type RegistrationRefusedError struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (e *RegistrationRefusedError) Error() string {
	return fmt.Sprintf("registration refused by partner: HTTP %d %s: %s", e.HTTPStatus, e.Code, e.Message)
}

// InitiatePartnerHandshake runs the INITIATOR side of spec §6.3:
//
//	Phase 1: fetch the responder's AS5 configuration (the URL as given, token
//	         included) and its JWKS.
//	Phase 2: build {fidex_version, initiator_node_id, initiator_as5_config_url,
//	         security_token, timestamp}, sign it RS256 with this node's key and
//	         POST the JWS compact serialization as application/jose to the
//	         responder's register endpoint. security_token is the ?token= the
//	         responder put in its URL — the initiator never mints one.
//	Phase 4: on a RegistrationResponse naming the responder's node_id, store
//	         the responder as a partner.
//
// The invitation token is a bearer secret: errors returned from here have it
// replaced by "REDACTED" (net/http errors quote the full request URL).
func (ds *DiscoveryService) InitiatePartnerHandshake(ctx context.Context, discoveryURL string) (*domain.Partner, error) {
	partner, err := ds.initiatePartnerHandshake(ctx, discoveryURL)
	if err != nil {
		if token, _ := TokenFromDiscoveryURL(discoveryURL); token != "" {
			return nil, &redactedError{err: err, token: token}
		}
	}
	return partner, err
}

func (ds *DiscoveryService) initiatePartnerHandshake(ctx context.Context, discoveryURL string) (*domain.Partner, error) {
	if ds.signer == nil {
		return nil, fmt.Errorf("discovery service has no signer: cannot sign a registration request")
	}

	token, err := TokenFromDiscoveryURL(discoveryURL)
	if err != nil {
		return nil, err
	}
	if token == "" {
		logger.Warn(ctx, "Discovery URL carries no ?%s=; registering without security_token (the responder may refuse)", TokenQueryParam)
	}

	// Phase 1
	logger.Info(ctx, "Phase 1: fetching responder AS5 configuration")
	remoteConfig, err := FetchAS5Config(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("phase 1 failed - could not fetch AS5 config: %w", err)
	}
	jwksData, err := FetchAndCachePartnerKeys(remoteConfig.Endpoints.JWKS)
	if err != nil {
		return nil, fmt.Errorf("phase 1 failed - could not fetch JWKS: %w", err)
	}
	logger.Info(ctx, "Phase 1 complete: %s (%s)", remoteConfig.OrganizationName, remoteConfig.NodeID)

	// Phase 2
	claims := RegistrationClaims{
		FidexVersion:          FidexVersion,
		InitiatorNodeID:       ds.nodeConfig.NodeID,
		InitiatorAS5ConfigURL: ds.OwnAS5ConfigURL(),
		SecurityToken:         token,
		Timestamp:             ds.now().UTC().Format(time.RFC3339),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("phase 2 failed - could not marshal registration: %w", err)
	}
	jws, err := ds.signer.SignCompactJWS(payload, ds.nodeConfig.SigningKeyID)
	if err != nil {
		return nil, fmt.Errorf("phase 2 failed - could not sign registration: %w", err)
	}
	logger.Info(ctx, "Phase 2: registering with %s", remoteConfig.Endpoints.Register)
	regResp, err := ds.sendRegistration(ctx, remoteConfig.Endpoints.Register, jws)
	if err != nil {
		return nil, fmt.Errorf("phase 2 failed: %w", err)
	}
	if regResp.ResponderNodeID != remoteConfig.NodeID {
		return nil, fmt.Errorf("phase 2 failed - responder_node_id %q does not match the AS5 configuration's node_id %q",
			regResp.ResponderNodeID, remoteConfig.NodeID)
	}

	// Phase 4
	now := ds.now()
	partner := &domain.Partner{
		PartnerID:          remoteConfig.NodeID,
		Name:               remoteConfig.OrganizationName,
		JWKSUrl:            remoteConfig.Endpoints.JWKS,
		MessageEndpoint:    remoteConfig.Endpoints.ReceiveMessage,
		MDNReceiptEndpoint: remoteConfig.Endpoints.ReceiveReceipt,
		PublicKeyJWKS:      jwksData,
		// ADR-0003 / FID-4: hydrate the partner's advertised encryption
		// capability from the AS5 config (not persisted; see Partner doc).
		SupportedEncryptionAlgorithms: ResolveSupportedEncryptionAlgorithms(remoteConfig),
		LastKeyRefresh:                &now,
	}
	if err := ds.partnerRepo.Upsert(ctx, partner); err != nil {
		return nil, fmt.Errorf("phase 4 failed - could not store partner: %w", err)
	}
	logger.Info(ctx, "✓ Registered with %s (%s)", partner.Name, partner.PartnerID)
	return partner, nil
}

// sendRegistration POSTs the signed registration and decodes the outcome:
// a RegistrationResponse on 200, a *RegistrationRefusedError otherwise.
func (ds *DiscoveryService) sendRegistration(ctx context.Context, registerURL, jws string) (*RegistrationResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, bytes.NewBufferString(jws))
	if err != nil {
		return nil, fmt.Errorf("could not build registration request: %w", err)
	}
	req.Header.Set("Content-Type", ContentTypeJOSE)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := ds.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not send registration request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return nil, fmt.Errorf("could not read registration response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var er ErrorResponse
		if json.Unmarshal(body, &er) == nil && er.Error.Code != "" {
			return nil, &RegistrationRefusedError{HTTPStatus: resp.StatusCode, Code: er.Error.Code, Message: er.Error.Message}
		}
		return nil, &RegistrationRefusedError{HTTPStatus: resp.StatusCode, Message: "response is not an ErrorResponse"}
	}

	var rr RegistrationResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, fmt.Errorf("registration response is not a RegistrationResponse: %w", err)
	}
	if rr.Status != RegistrationStatusRegistered {
		return nil, fmt.Errorf("registration response status %q, want %q", rr.Status, RegistrationStatusRegistered)
	}
	return &rr, nil
}
