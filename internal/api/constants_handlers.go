package api

import (
	"encoding/json"
	"fidex-node/internal/constants"
	"net/http"
)

// APIRoutes represents the frontend-accessible route constants
type APIRoutes struct {
	// Authentication routes
	AuthLogin  string `json:"authLogin"`
	AuthLogout string `json:"authLogout"`
	AuthMe     string `json:"authMe"`

	// Dashboard routes
	DashboardWS       string `json:"dashboardWS"`
	DashboardQR       string `json:"dashboardQR"`
	DashboardMetrics  string `json:"dashboardMetrics"`
	DashboardMessages string `json:"dashboardMessages"`
	DashboardPartners string `json:"dashboardPartners"`
	DiscoverPartner   string `json:"discoverPartner"`

	// Settings routes
	SettingsConfig      string `json:"settingsConfig"`
	SettingsRotateKeys  string `json:"settingsRotateKeys"`
	SettingsUsers       string `json:"settingsUsers"`
	SettingsUserByID    string `json:"settingsUserById"` // Template: replace {id}
	SettingsPassword    string `json:"settingsPassword"`
	SettingsPartners    string `json:"settingsPartners"`
	SettingsPartnerByID string `json:"settingsPartnerById"` // Template: replace {id}

	// Page routes
	Login     string `json:"login"`
	Dashboard string `json:"dashboard"`
}

// constantsHandler serves the route constants as JSON for frontend consumption
func constantsHandler(w http.ResponseWriter, r *http.Request) {
	routes := APIRoutes{
		// Authentication routes (full paths)
		AuthLogin:  constants.APIAuth + constants.RouteAuthLogin,
		AuthLogout: constants.APIAuth + constants.RouteAuthLogout,
		AuthMe:     constants.APIAuth + constants.RouteAuthMe,

		// Dashboard routes (full paths)
		DashboardWS:       constants.APIDashboard + constants.RouteDashboardWS,
		DashboardQR:       constants.APIDashboard + constants.RouteDashboardQR,
		DashboardMetrics:  constants.APIDashboard + constants.RouteDashboardMetrics,
		DashboardMessages: constants.APIDashboard + constants.RouteDashboardMessages,
		DashboardPartners: constants.APIDashboard + constants.RouteDashboardPartners,
		DiscoverPartner:   constants.APIDashboard + constants.RouteDiscover,

		// Settings routes (full paths)
		SettingsConfig:      constants.APISettings + constants.RouteConfig,
		SettingsRotateKeys:  constants.APISettings + constants.RouteRotateKeys,
		SettingsUsers:       constants.APISettings + constants.RouteUsers,
		SettingsUserByID:    constants.APISettings + constants.RouteUserByID,
		SettingsPassword:    constants.APISettings + constants.RoutePassword,
		SettingsPartners:    constants.APISettings + constants.RoutePartners,
		SettingsPartnerByID: constants.APISettings + constants.RoutePartnerByID,

		// Page routes
		Login:     constants.RouteLogin,
		Dashboard: constants.RouteDashboard,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
	json.NewEncoder(w).Encode(routes)
}
