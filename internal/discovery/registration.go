package discovery

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fidex-node/internal/domain"

	"github.com/go-jose/go-jose/v4"
)

// Error codes (openapi.yaml ErrorResponse.error.code) a responder uses when
// refusing a registration.
const (
	CodeInvalidToken      = "INVALID_TOKEN"
	CodeSignatureInvalid  = "SIGNATURE_INVALID"
	CodeUnknownKeyID      = "UNKNOWN_KEY_ID"
	CodeAlreadyRegistered = "ALREADY_REGISTERED"
	CodeConfigUnreachable = "CONFIG_UNREACHABLE"
	CodeInternalError     = "INTERNAL_ERROR"
)

// minimumRSAKeyBits mirrors the minimum_key_size this node advertises.
const minimumRSAKeyBits = 2048

// RegistrationError is a refusal of an inbound registration. HTTPStatus, Code,
// Message and Field are what the initiator sees (as an ErrorResponse); Err is
// the internal cause, for logs only.
type RegistrationError struct {
	HTTPStatus int
	Code       string
	Message    string
	Field      string
	Err        error
}

func (e *RegistrationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s (HTTP %d): %s: %v", e.Code, e.HTTPStatus, e.Message, e.Err)
	}
	return fmt.Sprintf("%s (HTTP %d): %s", e.Code, e.HTTPStatus, e.Message)
}

func refuse(status int, code, message string, cause error) *RegistrationError {
	return &RegistrationError{HTTPStatus: status, Code: code, Message: message, Err: cause}
}

// malformed is the 400 for a body that is not an acceptable registration JWS.
// The ErrorResponse enum has no generic "bad request" code; SIGNATURE_INVALID
// ("the signed registration object is not acceptable") is the closest fit.
func malformed(field, message string, cause error) *RegistrationError {
	e := refuse(http.StatusBadRequest, CodeSignatureInvalid, message, cause)
	e.Field = field
	return e
}

// peekedRegistration is a registration JWS parsed but NOT yet verified.
// Nothing in it is trusted until verifyRegistration succeeds; before that it
// is only used to find the token and bound the timestamp.
type peekedRegistration struct {
	jws       *jose.JSONWebSignature
	kid       string
	claims    RegistrationClaims
	timestamp time.Time
}

// peekRegistration parses the body as an RS256 JWS compact serialization and
// decodes (without verifying) its payload as RegistrationClaims.
func peekRegistration(body string) (*peekedRegistration, *RegistrationError) {
	compact := strings.TrimSpace(body)
	if len(compact) > MaxRegistrationBytes {
		return nil, malformed("", "registration body too large", nil)
	}
	jws, err := jose.ParseSignedCompact(compact, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, malformed("", "body is not an RS256 JWS compact serialization", err)
	}
	if len(jws.Signatures) != 1 {
		return nil, malformed("", "JWS must carry exactly one signature", nil)
	}

	var c RegistrationClaims
	if err := json.Unmarshal(jws.UnsafePayloadWithoutVerification(), &c); err != nil {
		return nil, malformed("", "JWS payload is not a registration request object", err)
	}
	if c.FidexVersion == "" {
		return nil, malformed("fidex_version", "fidex_version is required", nil)
	}
	if c.FidexVersion != FidexVersion {
		return nil, malformed("fidex_version", fmt.Sprintf("unsupported fidex_version %q (this node supports %q)", c.FidexVersion, FidexVersion), nil)
	}
	if c.InitiatorNodeID == "" {
		return nil, malformed("initiator_node_id", "initiator_node_id is required", nil)
	}
	u, err := url.Parse(c.InitiatorAS5ConfigURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, malformed("initiator_as5_config_url", "initiator_as5_config_url must be an absolute http(s) URL", err)
	}
	if c.Timestamp == "" {
		return nil, malformed("timestamp", "timestamp is required", nil)
	}
	ts, err := time.Parse(time.RFC3339, c.Timestamp)
	if err != nil {
		return nil, malformed("timestamp", "timestamp must be RFC 3339", err)
	}

	return &peekedRegistration{
		jws:       jws,
		kid:       jws.Signatures[0].Protected.KeyID,
		claims:    c,
		timestamp: ts,
	}, nil
}

// signingKeyCandidates returns the RSA public keys of a JWKS that may verify
// an RS256 signature: use absent or "sig", alg absent or "RS256", at least
// minimumRSAKeyBits. When kid is non-empty only keys with that kid qualify
// (spec annotated §6.4 Step 10: "Extract kid from JWS header; lookup public
// key in initiator's JWKS"). An empty kid means "try every signing key".
func signingKeyCandidates(jwksJSON, kid string) ([]*rsa.PublicKey, error) {
	var set jose.JSONWebKeySet
	if err := json.Unmarshal([]byte(jwksJSON), &set); err != nil {
		return nil, fmt.Errorf("JWKS is not valid: %w", err)
	}
	var out []*rsa.PublicKey
	for _, k := range set.Keys {
		if kid != "" && k.KeyID != kid {
			continue
		}
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if k.Algorithm != "" && k.Algorithm != string(jose.RS256) {
			continue
		}
		pub, ok := k.Key.(*rsa.PublicKey)
		if !ok || pub.N.BitLen() < minimumRSAKeyBits {
			continue
		}
		out = append(out, pub)
	}
	return out, nil
}

// HandleRegistration runs the RESPONDER side of spec §6.3 Phase 3 on the raw
// body of POST /api/v1/register. The order is the security property:
//
//  1. Parse the JWS and PEEK its claims (unverified): shape, version, URL,
//     RFC 3339 timestamp. Malformed → 400.
//  2. Refuse a timestamp outside ±15 min (§6.4) BEFORE spending the token, so
//     a replayed or stale request cannot burn somebody's invitation.
//  3. SPEND the security token (single-use, §6.4). Unknown, expired or reused
//     → 401 INVALID_TOKEN. Nothing the initiator names has been fetched yet:
//     without an open token this endpoint makes the node dial nothing.
//  4. Fetch initiator_as5_config_url; its node_id MUST equal
//     initiator_node_id.
//  5. Fetch the initiator's JWKS and verify the JWS (RS256) with its signing
//     key — the kid from the header when present.
//  6. Store the initiator as a partner (409 if it already is one).
//
// After step 3 every refusal leaves the token spent: one token, one attempt.
func (ds *DiscoveryService) HandleRegistration(ctx context.Context, body string) (*RegistrationResponse, *RegistrationError) {
	// 1. Peek.
	reg, rerr := peekRegistration(body)
	if rerr != nil {
		return nil, rerr
	}
	c := reg.claims

	// 2. Timestamp window, boundaries included.
	skew := ds.now().Sub(reg.timestamp)
	if skew < 0 {
		skew = -skew
	}
	if skew > TimestampWindow {
		e := refuse(http.StatusUnauthorized, CodeSignatureInvalid,
			fmt.Sprintf("timestamp is outside ±%v of this node's clock", TimestampWindow), nil)
		e.Field = "timestamp"
		return nil, e
	}

	// 3. Spend the token — before any outbound request.
	if c.SecurityToken == "" {
		return nil, refuse(http.StatusUnauthorized, CodeInvalidToken, "security_token is required by this node", nil)
	}
	if !ds.tokenStore.ValidateAndConsume(c.SecurityToken) {
		return nil, refuse(http.StatusUnauthorized, CodeInvalidToken, "Security token is invalid or expired", nil)
	}

	// 4. Initiator's AS5 configuration; it must name the claimed node.
	cfg, err := FetchAS5Config(c.InitiatorAS5ConfigURL)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, CodeConfigUnreachable, "Unable to fetch initiator AS5 configuration", err)
	}
	if cfg.NodeID != c.InitiatorNodeID {
		return nil, refuse(http.StatusUnauthorized, CodeSignatureInvalid,
			"the AS5 configuration at initiator_as5_config_url does not name initiator_node_id",
			fmt.Errorf("config node_id %q, claimed %q", cfg.NodeID, c.InitiatorNodeID))
	}

	// 5. Initiator's JWKS, then the signature.
	jwksData, err := FetchAndCachePartnerKeys(cfg.Endpoints.JWKS)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, CodeConfigUnreachable, "Unable to fetch initiator JWKS", err)
	}
	keys, err := signingKeyCandidates(jwksData, reg.kid)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, CodeConfigUnreachable, "Initiator JWKS is not valid", err)
	}
	if len(keys) == 0 {
		if reg.kid != "" {
			return nil, refuse(http.StatusUnauthorized, CodeUnknownKeyID,
				fmt.Sprintf("kid %q is not an RS256 signing key in the initiator's JWKS", reg.kid), nil)
		}
		return nil, refuse(http.StatusUnauthorized, CodeSignatureInvalid, "the initiator's JWKS offers no RS256 signing key", nil)
	}
	verified := false
	for _, k := range keys {
		if _, err := reg.jws.Verify(k); err == nil {
			verified = true
			break
		}
	}
	if !verified {
		return nil, refuse(http.StatusUnauthorized, CodeSignatureInvalid, "Registration request signature verification failed", nil)
	}

	// 6. Store the initiator. Checked only now, against a verified identity,
	// so an unauthenticated caller cannot probe who is registered.
	if existing, err := ds.partnerRepo.GetByID(ctx, c.InitiatorNodeID); err == nil && existing != nil {
		return nil, refuse(http.StatusConflict, CodeAlreadyRegistered,
			fmt.Sprintf("Partner %s is already registered", c.InitiatorNodeID), nil)
	}
	now := ds.now()
	partner := &domain.Partner{
		PartnerID:                     cfg.NodeID,
		Name:                          cfg.OrganizationName,
		JWKSUrl:                       cfg.Endpoints.JWKS,
		MessageEndpoint:               cfg.Endpoints.ReceiveMessage,
		MDNReceiptEndpoint:            cfg.Endpoints.ReceiveReceipt,
		PublicKeyJWKS:                 jwksData,
		SupportedEncryptionAlgorithms: ResolveSupportedEncryptionAlgorithms(cfg),
		LastKeyRefresh:                &now,
	}
	if err := ds.partnerRepo.Create(ctx, partner); err != nil {
		return nil, refuse(http.StatusInternalServerError, CodeInternalError, "Failed to store partner", err)
	}
	logger.Info(ctx, "Registered partner %s (%s)", partner.Name, partner.PartnerID)

	name := cfg.OrganizationName
	if name == "" {
		name = cfg.NodeID
	}
	return &RegistrationResponse{
		Status:                RegistrationStatusRegistered,
		ResponderNodeID:       ds.nodeConfig.NodeID,
		ResponderAS5ConfigURL: ds.OwnAS5ConfigURL(),
		Message:               "Successfully registered " + name,
		Timestamp:             now.UTC().Format(time.RFC3339),
	}, nil
}

// NewErrorResponse builds openapi.yaml's ErrorResponse.
func NewErrorResponse(code, message, field string, now time.Time) ErrorResponse {
	return ErrorResponse{Error: ErrorBody{
		Code:      code,
		Message:   message,
		Timestamp: now.UTC().Format(time.RFC3339),
		Field:     field,
	}}
}
