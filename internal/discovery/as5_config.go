package discovery

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"fidex-node/internal/constants"
	"fidex-node/internal/crypto"
)

// AS5Configuration is the discovery document a node publishes so partners can
// onboard automatically. Field names and structure match
// fidex-protocol-specification.md §6.2 exactly.
type AS5Configuration struct {
	FidexVersion           string            `json:"fidex_version"`
	SupportedVersions      []string          `json:"supported_versions"`
	ConformanceProfile     string            `json:"conformance_profile,omitempty"`
	NodeID                 string            `json:"node_id"`
	OrganizationName       string            `json:"organization_name"`
	PublicDomain           string            `json:"public_domain"`
	SupportedDocumentTypes []string          `json:"supported_document_types,omitempty"`
	Endpoints              AS5Endpoints      `json:"endpoints"`
	Security               AS5SecurityConfig `json:"security"`
}

// AS5Endpoints groups the per-operation URLs the peer should call.
// Each value is an absolute URL; paths MAY differ between implementations
// (the spec leaves them implementation-defined). See spec §6.2.
type AS5Endpoints struct {
	ReceiveMessage string `json:"receive_message"`
	ReceiveReceipt string `json:"receive_receipt"`
	Register       string `json:"register"`
	JWKS           string `json:"jwks"`
}

// AS5SecurityConfig declares the crypto algorithms this node supports.
//
// EncryptionAlgorithm is the historical single-value field per spec §6.2 and
// remains the back-compat anchor: partners that only know about a single
// algorithm read this field. SupportedEncryptionAlgorithms is the newer,
// additive advertisement introduced for FID-4 (ADR-0003) so that peers
// negotiating capabilities can pick the strongest mutually-supported alg
// (e.g. RSA-OAEP-256). Older peers ignore the array. See spec §5.2 and JWA
// RFC 7518 §4.2 / §4.3 for algorithm semantics.
type AS5SecurityConfig struct {
	SignatureAlgorithm            string   `json:"signature_algorithm"`
	EncryptionAlgorithm           string   `json:"encryption_algorithm"`
	SupportedEncryptionAlgorithms []string `json:"supported_encryption_algorithms,omitempty"`
	ContentEncryption             string   `json:"content_encryption"`
	MinimumKeySize                int      `json:"minimum_key_size"`
}

// NodeConfig holds the node's configuration for discovery.
// PublicDomain feeds the public_domain field in AS5 config.
//
// BaseURL is this node's PUBLIC base URL (scheme://host[:port]) as partners
// reach it; the node's own AS5 configuration URL is BaseURL +
// constants.RouteAS5Configuration. SigningKeyID is the kid of the "sig" key
// in this node's JWKS, put in the header of the registration JWS.
type NodeConfig struct {
	NodeID                 string
	OrganizationName       string
	BaseURL                string
	PublicDomain           string
	SupportedDocumentTypes []string
	SigningKeyID           string
	// AllowPrivateNetworks lets partner URLs resolve to loopback, private
	// and link-local addresses (FIDEX_ALLOW_PRIVATE_PEERS). Off in
	// production: see internal/netguard.
	AllowPrivateNetworks bool
}

// maxFetchBytes bounds every document read from a partner (AS5
// configuration, JWKS, registration response).
const maxFetchBytes = 1 << 20

// GenerateAS5Config builds this node's spec-conformant discovery document.
func GenerateAS5Config(config NodeConfig) *AS5Configuration {
	docTypes := config.SupportedDocumentTypes
	if docTypes == nil {
		docTypes = []string{}
	}
	publicDomain := config.PublicDomain
	if publicDomain == "" {
		publicDomain = config.BaseURL
	}

	return &AS5Configuration{
		FidexVersion:           "1.0",
		SupportedVersions:      []string{"1.0"},
		ConformanceProfile:     "core",
		NodeID:                 config.NodeID,
		OrganizationName:       config.OrganizationName,
		PublicDomain:           publicDomain,
		SupportedDocumentTypes: docTypes,
		Endpoints: AS5Endpoints{
			ReceiveMessage: config.BaseURL + constants.RouteInbound,
			ReceiveReceipt: config.BaseURL + constants.RouteReceipt,
			Register:       config.BaseURL + constants.RouteRegister,
			JWKS:           config.BaseURL + constants.RouteJWKS,
		},
		Security: AS5SecurityConfig{
			SignatureAlgorithm:  "RS256",
			EncryptionAlgorithm: "RSA-OAEP",
			// Advertise dual support (ADR-0003 / FID-4). The single
			// EncryptionAlgorithm field stays "RSA-OAEP" so legacy peers
			// that only read that field keep working unchanged.
			SupportedEncryptionAlgorithms: []string{"RSA-OAEP", "RSA-OAEP-256"},
			ContentEncryption:             "A256GCM",
			MinimumKeySize:                2048,
		},
	}
}

// FetchAS5Config fetches and validates an AS5 configuration from a remote URL.
// client should come from netguard.NewClient: the URL is partner-chosen.
func FetchAS5Config(client *http.Client, discoveryURL string) (*AS5Configuration, error) {
	resp, err := client.Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch AS5 config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch AS5 config: HTTP %d", resp.StatusCode)
	}

	var config AS5Configuration
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxFetchBytes)).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse AS5 config: %w", err)
	}

	if err := validateAS5Config(&config); err != nil {
		return nil, fmt.Errorf("invalid AS5 config: %w", err)
	}

	return &config, nil
}

// validateAS5Config ensures the required fields per spec §6.2 are present.
func validateAS5Config(config *AS5Configuration) error {
	if config.NodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	if config.FidexVersion == "" {
		return fmt.Errorf("fidex_version is required")
	}
	if len(config.SupportedVersions) == 0 {
		return fmt.Errorf("supported_versions is required and must be non-empty")
	}
	for name, v := range map[string]string{
		"endpoints.jwks":            config.Endpoints.JWKS,
		"endpoints.receive_message": config.Endpoints.ReceiveMessage,
		"endpoints.receive_receipt": config.Endpoints.ReceiveReceipt,
		"endpoints.register":        config.Endpoints.Register,
	} {
		if v == "" {
			return fmt.Errorf("%s is required", name)
		}
		if !isAbsoluteHTTPURL(v) {
			return fmt.Errorf("%s must be an absolute http(s) URL", name)
		}
	}
	return nil
}

// isAbsoluteHTTPURL reports whether s is an http or https URL with a host
// and no userinfo.
func isAbsoluteHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}

// ResolveSupportedEncryptionAlgorithms returns the list of encryption
// algorithms a partner supports, honouring the back-compat rules in
// ADR-0003 (FID-4):
//
//   - If the newer supported_encryption_algorithms array is present, it is
//     used verbatim (the source of truth).
//   - Otherwise the single legacy encryption_algorithm string is wrapped
//     into a one-element slice.
//   - If neither is set the function returns an empty slice — callers
//     should treat that as "peer didn't tell us, assume RSA-OAEP".
//
// This helper is the canonical way to read an AS5 partner's encryption
// capability before negotiating an algorithm.
func ResolveSupportedEncryptionAlgorithms(cfg *AS5Configuration) []string {
	if cfg == nil {
		return nil
	}
	if len(cfg.Security.SupportedEncryptionAlgorithms) > 0 {
		// Defensive copy so callers can mutate without aliasing the cfg.
		out := make([]string, len(cfg.Security.SupportedEncryptionAlgorithms))
		copy(out, cfg.Security.SupportedEncryptionAlgorithms)
		return out
	}
	if cfg.Security.EncryptionAlgorithm != "" {
		return []string{cfg.Security.EncryptionAlgorithm}
	}
	return nil
}

// FetchAndCachePartnerKeys fetches a partner's JWKS and returns it as a string.
// client should come from netguard.NewClient: the URL is partner-chosen.
func FetchAndCachePartnerKeys(client *http.Client, jwksURL string) (string, error) {
	resp, err := client.Get(jwksURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch JWKS: HTTP %d", resp.StatusCode)
	}

	var jwksData interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxFetchBytes)).Decode(&jwksData); err != nil {
		return "", fmt.Errorf("invalid JWKS JSON: %w", err)
	}

	jwksBytes, err := json.Marshal(jwksData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWKS: %w", err)
	}

	if _, err := crypto.ParsePublicKeyFromJWKS(string(jwksBytes)); err != nil {
		return "", fmt.Errorf("invalid JWKS format: %w", err)
	}

	return string(jwksBytes), nil
}
