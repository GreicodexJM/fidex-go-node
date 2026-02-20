package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fidex-node/internal/api"
	"fidex-node/internal/auth"
	"fidex-node/internal/config"
	"fidex-node/internal/crypto"
	"fidex-node/internal/db"
	"fidex-node/internal/queue"
	"fidex-node/internal/watcher"
)

// Global config accessible to all packages
var AppConfig *config.Config

func main() {
	log.Println("========================================")
	log.Println("Starting FideX Edge Node...")
	log.Println("========================================")

	// 1. Load Configuration (from JSON file, env vars, or CLI args)
	log.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	AppConfig = cfg
	api.NodeConfig = cfg // Set config for API handlers
	log.Printf("✓ Configuration loaded (Internal Port: %d, Public Port: %d)", cfg.InternalAPIPort, cfg.PublicAPIPort)

	// 2. Initialize SQLite Database
	log.Println("Initializing database...")
	if err := db.InitDB(cfg.DatabasePath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("✓ Database initialized successfully")

	// 3. Initialize default admin user
	log.Println("Checking for default user...")
	if err := api.InitializeDefaultUser(); err != nil {
		log.Fatalf("Failed to initialize default user: %v", err)
	}

	// 4. Generate node keys if they don't exist
	if _, err := os.Stat(cfg.PrivateKeyPath); os.IsNotExist(err) {
		log.Println("No private key found, generating RSA key pair...")
		privateKeyPEM, publicKeyPEM, err := crypto.GenerateKeyPair()
		if err != nil {
			log.Fatalf("Failed to generate key pair: %v", err)
		}

		// Create keys directory if it doesn't exist
		if err := os.MkdirAll("./keys", 0700); err != nil {
			log.Fatalf("Failed to create keys directory: %v", err)
		}

		// Save private key
		if err := os.WriteFile(cfg.PrivateKeyPath, []byte(privateKeyPEM), 0600); err != nil {
			log.Fatalf("Failed to save private key: %v", err)
		}

		// Save public key
		if err := os.WriteFile(cfg.PublicKeyPath, []byte(publicKeyPEM), 0644); err != nil {
			log.Fatalf("Failed to save public key: %v", err)
		}

		log.Println("✓ RSA key pair generated and saved")
		log.Println("========================================")
		log.Println("Public Key (share with trading partners):")
		log.Println(publicKeyPEM)
		log.Println("========================================")
	}

	// Display the generated API key on first run
	if cfg.InternalAPIKey != "" {
		log.Println("========================================")
		log.Println("IMPORTANT: Internal API Key (save this!)")
		log.Printf("API Key: %s", cfg.InternalAPIKey)
		log.Println("Use this key in the Authorization header:")
		log.Printf("  Authorization: Bearer %s", cfg.InternalAPIKey)
		log.Println("========================================")
	}

	// 5. Initialize File Watcher
	log.Println("Starting file watcher...")
	fw, err := watcher.NewFileWatcher()
	if err != nil {
		log.Fatalf("Failed to create file watcher: %v", err)
	}
	if err := fw.Start(); err != nil {
		log.Fatalf("Failed to start file watcher: %v", err)
	}
	log.Println("✓ File watcher started")

	// 4. Initialize WebSocket Hub
	log.Println("Initializing WebSocket hub...")
	api.InitializeWebSocketHub()
	log.Println("✓ WebSocket hub initialized")

	// 4.2. Initialize Message Queue Worker
	log.Println("Starting message queue worker...")
	queueWorker := queue.NewWorker()
	queueWorker.Start()
	log.Println("✓ Message queue worker started")

	// 4.5. Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			auth.CleanupExpiredSessions()
		}
	}()

	// 6. Parse allowed IPs
	allowedIPs, err := api.ParseAllowedIPs(cfg.AllowedIPsString())
	if err != nil {
		log.Fatalf("Failed to parse allowed IPs: %v", err)
	}
	log.Printf("✓ IP Allowlist: %v (Enabled: %v)", cfg.AllowedIPAddresses, cfg.EnableIPAllowlist)

	// 7. Setup HTTP Routers
	internalRouter := api.SetupInternalRouter(allowedIPs, cfg.InternalAPIKey, cfg.EnableIPAllowlist)
	publicRouter := api.SetupPublicRouter()

	// Mount auth routes on internal router
	internalRouter.Mount("/api/auth", api.SetupAuthRouter())

	// Mount dashboard routes on internal router
	internalRouter.Mount("/api/dashboard", api.SetupDashboardRouter())

	// Mount settings routes on internal router
	internalRouter.Mount("/api/settings", api.SetupSettingsRouter())

	// 8. Create HTTP Servers
	internalServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.InternalAPIPort),
		Handler:      internalRouter,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	publicServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.PublicAPIPort),
		Handler:      publicRouter,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 7. Start HTTP Servers in goroutines
	go func() {
		log.Printf("Starting Internal API Server on %s", internalServer.Addr)
		log.Println("  - POST /api/v1/transmit (Protected: IP Allowlist + API Key)")
		if err := internalServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Internal server error: %v", err)
		}
	}()

	go func() {
		log.Printf("Starting Public API Server on %s", publicServer.Addr)
		log.Println("  - GET  /health")
		log.Println("  - POST /api/v1/inbound")
		log.Println("  - POST /api/v1/receipt")
		log.Println("  - GET  /.well-known/jwks.json")
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Public server error: %v", err)
		}
	}()

	log.Println("========================================")
	log.Println("✓ FideX Edge Node is running!")
	log.Println("========================================")
	log.Println("Press Ctrl+C to gracefully shutdown...")

	// 8. Setup graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Wait for interrupt signal
	<-quit
	log.Println("\n========================================")
	log.Println("Shutting down FideX Edge Node...")
	log.Println("========================================")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown file watcher
	log.Println("Stopping file watcher...")
	if err := fw.Stop(); err != nil {
		log.Printf("Error stopping file watcher: %v", err)
	} else {
		log.Println("✓ File watcher stopped")
	}

	// Shutdown queue worker
	log.Println("Stopping queue worker...")
	queueWorker.Stop()
	log.Println("✓ Queue worker stopped")

	// Shutdown internal server
	log.Println("Stopping internal API server...")
	if err := internalServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down internal server: %v", err)
	} else {
		log.Println("✓ Internal API server stopped")
	}

	// Shutdown public server
	log.Println("Stopping public API server...")
	if err := publicServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down public server: %v", err)
	} else {
		log.Println("✓ Public API server stopped")
	}

	// Close database connection
	log.Println("Closing database connection...")
	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	} else {
		log.Println("✓ Database closed")
	}

	log.Println("========================================")
	log.Println("✓ FideX Edge Node shut down successfully")
	log.Println("========================================")
}
