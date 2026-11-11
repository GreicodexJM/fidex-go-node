package api

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"

	"fidex-node/internal/discovery"
)

// as5ConfigHandler handles GET /.well-known/as5-configuration
// Returns the node's AS5 discovery document.
//
// Without ?token= the document is public (spec §6.2: plain discovery). With
// ?token= — the invitation URL this node hands out by QR — the token must be
// open (minted, unused, unexpired), else 401 INVALID_TOKEN (annotated spec
// §6.6.1). Serving the document does NOT spend the token; only a
// registration does (§6.4 "Reject invalid or reused tokens").
func (h *Handlers) as5ConfigHandler(w http.ResponseWriter, r *http.Request) {
	if h.Config == nil {
		logger.Error(r.Context(), "Node config not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	if _, presented := q[discovery.TokenQueryParam]; presented {
		token := q.Get(discovery.TokenQueryParam)
		if h.DiscoveryService == nil || !h.DiscoveryService.InvitationOpen(token) {
			respondWithFidexError(w, http.StatusUnauthorized, discovery.CodeInvalidToken, "Security token is invalid or expired", "")
			return
		}
	}

	discConfig := discovery.NodeConfig{
		NodeID:                 h.Config.NodeID,
		OrganizationName:       h.Config.OrganizationName,
		BaseURL:                getBaseURL(r),
		PublicDomain:           h.Config.PublicDomain,
		SupportedDocumentTypes: []string{},
	}

	as5Config := discovery.GenerateAS5Config(discConfig)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(as5Config)

	logger.Info(r.Context(), "AS5 configuration requested from %s", r.RemoteAddr)
}

// webhookRegistrationHandler handles POST /api/v1/register (spec §6.3 Phase 3,
// openapi.yaml registerPartner). The body is a JWS compact serialization sent
// as application/jose; the answer is a RegistrationResponse (200) or an
// ErrorResponse (400 / 401 / 409 / 500).
func (h *Handlers) webhookRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger.Info(ctx, "Received registration request from %s", r.RemoteAddr)

	if h.DiscoveryService == nil {
		logger.Error(ctx, "Discovery service not initialized")
		respondWithFidexError(w, http.StatusInternalServerError, discovery.CodeInternalError, "Service not available", "")
		return
	}

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != discovery.ContentTypeJOSE {
		respondWithFidexError(w, http.StatusBadRequest, discovery.CodeSignatureInvalid,
			"Content-Type must be application/jose (a JWS compact serialization)", "")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, discovery.MaxRegistrationBytes+1))
	if err != nil {
		respondWithFidexError(w, http.StatusBadRequest, discovery.CodeSignatureInvalid, "Could not read request body", "")
		return
	}

	resp, rerr := h.DiscoveryService.HandleRegistration(ctx, string(body))
	if rerr != nil {
		logger.Warn(ctx, "Registration refused: %v", rerr)
		respondWithFidexError(w, rerr.HTTPStatus, rerr.Code, rerr.Message, rerr.Field)
		return
	}

	respondWithJSON(w, http.StatusOK, resp)
	logger.Info(ctx, "Partner registration successful: %s", resp.Message)
}

// respondWithFidexError writes openapi.yaml's ErrorResponse.
func respondWithFidexError(w http.ResponseWriter, status int, code, message, field string) {
	respondWithJSON(w, status, discovery.NewErrorResponse(code, message, field, time.Now()))
}
