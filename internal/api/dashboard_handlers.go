package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"fidex-node/internal/dashboard"
	"fidex-node/internal/db"
	"fidex-node/internal/discovery"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var (
	// Global WebSocket hub
	wsHub *dashboard.Hub

	// WebSocket upgrader
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			// Allow all origins for now (restrict in production)
			return true
		},
	}
)

// InitializeWebSocketHub initializes the global WebSocket hub
func InitializeWebSocketHub() *dashboard.Hub {
	wsHub = dashboard.NewHub()
	go wsHub.Run()
	return wsHub
}

// GetWebSocketHub returns the global WebSocket hub
func GetWebSocketHub() *dashboard.Hub {
	return wsHub
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
	Messages []db.Message `json:"messages"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PerPage  int          `json:"per_page"`
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
func qrCodeHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Failed to generate QR code: %v", err)
		http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
		return
	}

	// Return PNG image
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pngBytes)))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	w.Write(pngBytes)

	log.Printf("QR code generated and served to %s", r.RemoteAddr)
}

// metricsHandler handles GET /api/dashboard/metrics
// Returns real-time system metrics
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	// Calculate metrics from database
	metrics := DashboardMetrics{
		SystemStatus: "operational",
	}

	// Count messages in last 24 hours by status
	cutoff := time.Now().Add(-24 * time.Hour)

	rows, err := db.DB.Query(`
		SELECT status, COUNT(*) 
		FROM messages 
		WHERE created_at >= ? 
		GROUP BY status
	`, cutoff)

	if err != nil {
		log.Printf("Failed to query message metrics: %v", err)
	} else {
		defer rows.Close()

		var total int
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err != nil {
				continue
			}
			total += count

			switch status {
			case string(db.StatusDelivered):
				metrics.MessagesDelivered24h = count
			case string(db.StatusQueued):
				metrics.MessagesQueued = count
			case string(db.StatusFailed):
				metrics.MessagesFailed = count
			}
		}

		// Calculate success rate
		if total > 0 {
			metrics.SuccessRate = float64(metrics.MessagesDelivered24h) / float64(total)
		}
	}

	// Count active partners
	err = db.DB.QueryRow(`SELECT COUNT(*) FROM trading_partners`).Scan(&metrics.ActivePartners)
	if err != nil {
		log.Printf("Failed to count partners: %v", err)
	}

	respondWithJSON(w, http.StatusOK, metrics)
}

// messagesHandler handles GET /api/dashboard/messages
// Returns paginated list of messages
func messagesHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	statusFilter := r.URL.Query().Get("status")

	limit := 20
	offset := 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Build query
	query := `SELECT id, message_id, direction, status, payload, created_at FROM messages`
	countQuery := `SELECT COUNT(*) FROM messages`
	args := []interface{}{}

	if statusFilter != "" {
		query += ` WHERE status = ?`
		countQuery += ` WHERE status = ?`
		args = append(args, statusFilter)
	}

	query += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	queryArgs := append(args, limit, offset)

	// Get total count
	var total int
	err := db.DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		log.Printf("Failed to count messages: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to count messages", err)
		return
	}

	// Get messages
	rows, err := db.DB.Query(query, queryArgs...)
	if err != nil {
		log.Printf("Failed to query messages: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to query messages", err)
		return
	}
	defer rows.Close()

	messages := []db.Message{}
	for rows.Next() {
		var msg db.Message
		if err := rows.Scan(&msg.ID, &msg.MessageID, &msg.Direction, &msg.Status, &msg.Payload, &msg.CreatedAt); err != nil {
			log.Printf("Failed to scan message: %v", err)
			continue
		}
		messages = append(messages, msg)
	}

	response := MessageListResponse{
		Messages: messages,
		Total:    total,
		Page:     offset/limit + 1,
		PerPage:  limit,
	}

	respondWithJSON(w, http.StatusOK, response)
}

// partnersHandler handles GET /api/dashboard/partners
// Returns list of trading partners
func partnersHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(`
		SELECT id, partner_id, name, jwks_url, created_at 
		FROM trading_partners 
		ORDER BY created_at DESC
	`)
	if err != nil {
		log.Printf("Failed to query partners: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to query partners", err)
		return
	}
	defer rows.Close()

	partners := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var partnerID, name, jwksURL string
		var createdAt time.Time

		if err := rows.Scan(&id, &partnerID, &name, &jwksURL, &createdAt); err != nil {
			log.Printf("Failed to scan partner: %v", err)
			continue
		}

		partners = append(partners, map[string]interface{}{
			"id":         id,
			"partner_id": partnerID,
			"name":       name,
			"jwks_url":   jwksURL,
			"created_at": createdAt,
			"status":     "connected",
		})
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"partners": partners,
	})
}

// discoverPartnerHandler handles POST /api/dashboard/partners/discover
// Initiates partner auto-discovery from AS5 configuration URL
func discoverPartnerHandler(w http.ResponseWriter, r *http.Request) {
	var req PartnerDiscoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	if req.DiscoveryURL == "" {
		respondWithError(w, http.StatusBadRequest, "discovery_url is required", nil)
		return
	}

	log.Printf("Discovering partner from: %s", req.DiscoveryURL)

	// Fetch AS5 configuration
	as5Config, err := discovery.FetchAS5Config(req.DiscoveryURL)
	if err != nil {
		log.Printf("Failed to fetch AS5 config: %v", err)
		respondWithError(w, http.StatusBadRequest, "Failed to fetch partner configuration", err)
		return
	}

	// Fetch partner's JWKS
	jwks, err := discovery.FetchAndCachePartnerKeys(as5Config.JWKSUri)
	if err != nil {
		log.Printf("Failed to fetch partner JWKS: %v", err)
		respondWithError(w, http.StatusBadRequest, "Failed to fetch partner keys", err)
		return
	}

	// Save partner to database
	_, err = db.DB.Exec(`
		INSERT INTO trading_partners (partner_id, name, jwks_url, public_key_jwks, last_key_refresh, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(partner_id) DO UPDATE SET
			name = excluded.name,
			jwks_url = excluded.jwks_url,
			public_key_jwks = excluded.public_key_jwks,
			last_key_refresh = excluded.last_key_refresh,
			updated_at = excluded.updated_at
	`, as5Config.Issuer, as5Config.OrganizationName, as5Config.JWKSUri, jwks, time.Now(), time.Now(), time.Now())

	if err != nil {
		log.Printf("Failed to save partner: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to save partner", err)
		return
	}

	log.Printf("Partner discovered and saved: %s (%s)", as5Config.OrganizationName, as5Config.Issuer)

	response := PartnerDiscoveryResponse{
		PartnerID: as5Config.Issuer,
		Name:      as5Config.OrganizationName,
		Status:    "connected",
		Message:   "Partner successfully discovered and added",
	}

	respondWithJSON(w, http.StatusOK, response)

	// Broadcast partner_added event to WebSocket clients
	if wsHub != nil {
		wsHub.Broadcast("partner_added", map[string]interface{}{
			"partner_id": as5Config.Issuer,
			"name":       as5Config.OrganizationName,
			"status":     "connected",
		})
	}
}

// websocketHandler handles GET /api/dashboard/ws
// Upgrades the connection to WebSocket for real-time updates
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// Check if WebSocket hub is initialized
	if wsHub == nil {
		log.Printf("WebSocket hub not initialized")
		http.Error(w, "WebSocket not available", http.StatusServiceUnavailable)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}

	// Generate client ID from remote address
	clientID := r.RemoteAddr

	// Serve the WebSocket connection
	wsHub.ServeWs(conn, clientID)
}

// SetupDashboardRouter configures the dashboard API routes
func SetupDashboardRouter() *chi.Mux {
	r := chi.NewRouter()

	// Dashboard API endpoints (all require authentication)
	r.Group(func(r chi.Router) {
		// Apply auth middleware to all dashboard routes
		// r.Use(auth.RequireAuthAPI)  // Uncomment when ready to enable auth

		r.Get("/ws", websocketHandler)
		r.Get("/qr", qrCodeHandler)
		r.Get("/metrics", metricsHandler)
		r.Get("/messages", messagesHandler)
		r.Get("/partners", partnersHandler)
		r.Post("/partners/discover", discoverPartnerHandler)
	})

	return r
}
