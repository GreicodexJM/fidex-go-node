package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

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

// qrCodeHandler handles GET /api/dashboard/qr
// Generates and returns a QR code PNG containing the node's discovery URL
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

	// Get base URL from request
	baseURL := getBaseURL(r)
	discoveryURL := dashboard.GetDiscoveryURL(baseURL)

	// Generate QR code
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
	w.WriteHeader(http.StatusOK)
	w.Write(pngBytes)

	logger.Info(ctx, "QR code generated and served to %s", r.RemoteAddr)
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

	logger.Info(ctx, "Discovering partner from: %s", req.DiscoveryURL)

	// Fetch AS5 configuration
	as5Config, err := discovery.FetchAS5Config(req.DiscoveryURL)
	if err != nil {
		logger.Warn(ctx, "Failed to fetch AS5 config: %v", err)
		respondWithError(w, http.StatusBadRequest, "Failed to fetch partner configuration", err)
		return
	}

	// Fetch partner's JWKS
	jwks, err := discovery.FetchAndCachePartnerKeys(as5Config.JWKSUri)
	if err != nil {
		logger.Warn(ctx, "Failed to fetch partner JWKS: %v", err)
		respondWithError(w, http.StatusBadRequest, "Failed to fetch partner keys", err)
		return
	}

	// Save partner to database via repository upsert.
	now := time.Now()
	partner := &domain.Partner{
		PartnerID:      as5Config.Issuer,
		Name:           as5Config.OrganizationName,
		JWKSUrl:        as5Config.JWKSUri,
		PublicKeyJWKS:  jwks,
		LastKeyRefresh: &now,
	}
	if err := h.PartnerRepo.Upsert(ctx, partner); err != nil {
		logger.Error(ctx, "Failed to save partner: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save partner", err)
		return
	}

	logger.Info(ctx, "Partner discovered and saved: %s (%s)", as5Config.OrganizationName, as5Config.Issuer)

	response := PartnerDiscoveryResponse{
		PartnerID: as5Config.Issuer,
		Name:      as5Config.OrganizationName,
		Status:    "connected",
		Message:   "Partner successfully discovered and added",
	}

	respondWithJSON(w, http.StatusOK, response)

	// Broadcast partner_added event to WebSocket clients
	if h.WebSocketHub != nil {
		h.WebSocketHub.Broadcast("partner_added", map[string]interface{}{
			"partner_id": as5Config.Issuer,
			"name":       as5Config.OrganizationName,
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
		r.Get("/metrics", h.metricsHandler)
		r.Get("/messages", h.messagesHandler)
		r.Get("/partners", h.partnersHandler)
		r.Post("/partners/discover", h.discoverPartnerHandler)
	})

	return r
}
