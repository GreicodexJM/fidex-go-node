package api

import (
	"io/ioutil"
	"log"
	"net/http"
)

// serveLoginHandler serves the login page
func serveLoginHandler(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadFile("ui/login.html")
	if err != nil {
		log.Printf("Failed to read login.html: %v", err)
		http.Error(w, "Login page not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// serveDashboardHandler serves the dashboard page (requires auth)
func serveDashboardHandler(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadFile("ui/dist/index.html")
	if err != nil {
		log.Printf("Failed to read dashboard: %v", err)
		http.Error(w, "Dashboard not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// serveStaticAssets serves static files from ui/dist
func serveStaticAssets(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// Remove /dashboard prefix if present (for relative paths)
	if len(path) > 10 && path[:11] == "/dashboard/" {
		path = path[10:]
	}

	// Prevent directory traversal
	if path == "/" || path == "" {
		http.NotFound(w, r)
		return
	}

	fullPath := "ui/dist" + path
	http.ServeFile(w, r, fullPath)
}
