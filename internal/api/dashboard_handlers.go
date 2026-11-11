package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"fidex-node/internal/constants"
	"fidex-node/internal/dashboard"
	"fidex-node/internal/discovery"
	"fidex-node/internal/domain"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// upgrader is the WebSocket upgrader configuration. It is intentionally
// package-level: it carries no per-request state and is safe for concurrent use.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now (restrict in production)
		return true
	},
}

// DashboardMetrics represents system health and metrics
type DashboardMetrics struct {
	MessagesDelivered24h int     `json:"messages_delivered_24h"`
	MessagesQueued       int     `json:"messages_queued"`
	MessagesFailed       int     `json:"messages_failed"`
	SuccessRate          float64 `json:"success_rate"`
	ActivePartners       int     `json:"active_partners"`
	SystemStatus         string  `json:"system_status"`
}

// MessageListResponse represents paginated message list
type MessageListResponse struct {
	Messages []domain.Message `json:"messages"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PerPage  int              `json:"per_page"`
}

// PartnerDiscoveryRequest represents a request to discover a partner
type PartnerDiscoveryRequest struct {
	DiscoveryURL string `json:"discovery_url"`
}

// PartnerDiscoveryResponse represents the response after discovering a partner
type PartnerDiscoveryResponse struct {
	PartnerID string `json:"partner_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// InvitationResponse is the body of POST /admin/dashboard/invitations.
type InvitationResponse struct {
	DiscoveryURL string    `json:"discovery_url"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// qrCodeHandler handles GET /admin/dashboard/qr
// Mints a single-use registration token and returns a QR code PNG of this
// node's AS5 configuration URL carrying it as ?token= (spec §6.2). Every call
// mints a fresh token; the URL is also returned in the X-FideX-Discovery-URL
// header so the UI can show it as text.
func (h *Handlers) qrCodeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Get size parameter (optional)
	sizeStr := r.URL.Query().Get("size")
	size := 256 // Default
	if sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 1024 {
			size = s
		}
	}

	if h.DiscoveryService == nil {
		http.Error(w, "Discovery service not available", http.StatusServiceUnavailable)
		return
	}
	discoveryURL, _, err := h.DiscoveryService.MintInvitation()
	if err != nil {
		logger.Error(ctx, "Failed to mint invitation token: %v", err)
		http.Error(w, "Failed to mint invitation", http.StatusInternalServerError)
		return
	}

	pngBytes, err := dashboard.GenerateQRCode(discoveryURL, size)
	if err != nil {
		logger.Error(ctx, "Failed to generate QR code: %v", err)
		http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
		return
	}

	// Return PNG image
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pngBytes)))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-FideX-Discovery-URL", discoveryURL)
	w.WriteHeader(http.StatusOK)
	w.Write(pngBytes)

	logger.Info(ctx, "Invitation QR code generated and served to %s", r.RemoteAddr)
}

// invitationHandler handles POST /admin/dashboard/invitations
// Mints a single-use registration token and returns the invitation URL as
// JSON (the same URL the QR encodes), for sharing by email/chat or scripting.
func (h *Handlers) invitationHandler(w http.ResponseWriter, r *http.Request) {
	if h.DiscoveryService == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Discovery service not available", nil)
		return
	}
	discoveryURL, expiresAt, err := h.DiscoveryService.MintInvitation()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to mint invitation", err)
		return
	}
	respondWithJSON(w, http.StatusOK, InvitationResponse{DiscoveryURL: discoveryURL, ExpiresAt: expiresAt})
}

// metricsHandler handles GET /api/dashboard/metrics
// Returns real-time system metrics
func (h *Handlers) metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metrics := DashboardMetrics{SystemStatus: "operational"}

	// Per-status message counts in the last 24 hours.
	counts, err := h.MessageRepo.CountByStatusSince(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		logger.Error(ctx, "Failed to query message metrics: %v", err)
	} else {
		metrics.MessagesDelivered24h = counts[domain.StatusDelivered]
		metrics.MessagesQueued = counts[domain.StatusQueued]
		metrics.MessagesFailed = counts[domain.StatusFailed]

		total := metrics.MessagesDelivered24h + metrics.MessagesQueued + metrics.MessagesFailed
		if total > 0 {
			metrics.SuccessRate = float64(metrics.MessagesDelivered24h) / float64(total)
		}
	}

	// Active partner count.
	if partnerCount, err := h.PartnerRepo.Count(ctx); err != nil {
		logger.Error(ctx, "Failed to count partners: %v", err)
	} else {
		metrics.ActivePartners = partnerCount
	}

	respondWithJSON(w, http.StatusOK, metrics)
}

// messagesHandler handles GET /api/dashboard/messages
// Returns paginated list of messages
func (h *Handlers) messagesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit := 20
	offset := 0
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	var statusFilter *domain.MessageStatus
	if s := r.URL.Query().Get("status"); s != "" {
		status := domain.MessageStatus(s)
		statusFilter = &status
	}

	msgPtrs, total, err := h.MessageRepo.ListPaginated(ctx, statusFilter, limit, offset)
	if err != nil {
		logger.Error(ctx, "Failed to list messages: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to query messages", err)
		return
	}

	messages := make([]domain.Message, 0, len(msgPtrs))
	for _, m := range msgPtrs {
		messages = append(messages, *m)
	}

	respondWithJSON(w, http.StatusOK, MessageListResponse{
		Messages: messages,
		Total:    total,
		Page:     offset/limit + 1,
		PerPage:  limit,
	})
}

// partnersHandler handles GET /api/dashboard/partners
// Returns list of trading partners
func (h *Handlers) partnersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.PartnerRepo.List(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to list partners: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to query partners", err)
		return
	}

	partners := make([]map[string]interface{}, 0, len(rows))
	for _, p := range rows {
		partners = append(partners, map[string]interface{}{
			"id":         p.ID,
			"partner_id": p.PartnerID,
			"name":       p.Name,
			"jwks_url":   p.JWKSUrl,
			"created_at": p.CreatedAt,
			"status":     "connected",
		})
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"partners": partners,
	})
}

// discoverPartnerHandler handles POST /api/dashboard/partners/discover
// Initiates partner auto-discovery from AS5 configuration URL
func (h *Handlers) discoverPartnerHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req PartnerDiscoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if req.DiscoveryURL == "" {
		respondWithError(w, http.StatusBadRequest, "discovery_url is required", nil)
		return
	}

	if h.DiscoveryService == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Discovery service not available", nil)
		return
	}

	logger.Info(ctx, "Registering with partner from: %s", req.DiscoveryURL)

	// Spec §6.3 initiator side: fetch the partner's AS5 config + JWKS, send a
	// signed registration carrying the URL's ?token=, store the partner only
	// once it answers "registered".
	partner, err := h.DiscoveryService.InitiatePartnerHandshake(ctx, req.DiscoveryURL)
	if err != nil {
		logger.Warn(ctx, "Partner registration failed: %v", err)
		var refused *discovery.RegistrationRefusedError
		if errors.As(err, &refused) {
			respondWithError(w, http.StatusBadGateway, "Partner refused the registration", err)
			return
		}
		respondWithError(w, http.StatusBadRequest, "Partner registration failed", err)
		return
	}
	logger.Info(ctx, "Partner registered and saved: %s (%s)", partner.Name, partner.PartnerID)

	response := PartnerDiscoveryResponse{
		PartnerID: partner.PartnerID,
		Name:      partner.Name,
		Status:    "connected",
		Message:   "Partner successfully registered and added",
	}

	respondWithJSON(w, http.StatusOK, response)

	// Broadcast partner_added event to WebSocket clients
	if h.WebSocketHub != nil {
		h.WebSocketHub.Broadcast("partner_added", map[string]interface{}{
			"partner_id": partner.PartnerID,
			"name":       partner.Name,
			"status":     "connected",
		})
	}
}

// websocketHandler handles GET /api/dashboard/ws
// Upgrades the connection to WebSocket for real-time updates
func (h *Handlers) websocketHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Check if WebSocket hub is initialized
	if h.WebSocketHub == nil {
		logger.Error(ctx, "WebSocket hub not initialized")
		http.Error(w, "WebSocket not available", http.StatusServiceUnavailable)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(ctx, "Failed to upgrade to WebSocket: %v", err)
		return
	}

	// Generate client ID from remote address
	clientID := r.RemoteAddr

	// Serve the WebSocket connection
	h.WebSocketHub.ServeWs(conn, clientID)
}

// SetupDashboardRouter configures the dashboard API routes
func (h *Handlers) SetupDashboardRouter() *chi.Mux {
	r := chi.NewRouter()

	// Dashboard API endpoints (all require authentication)
	r.Group(func(r chi.Router) {
		// Apply auth middleware to all dashboard routes
		// r.Use(auth.RequireAuthAPI)  // Uncomment when ready to enable auth

		r.Get("/ws", h.websocketHandler)
		r.Get("/qr", h.qrCodeHandler)
		r.Post(constants.RouteInvitations, h.invitationHandler)
		r.Get("/metrics", h.metricsHandler)
		r.Get("/messages", h.messagesHandler)
		r.Get("/partners", h.partnersHandler)
		r.Post("/partners/discover", h.discoverPartnerHandler)
	})

	return r
}
