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
	"fidex-node/internal/config"
	"fidex-node/internal/constants"
	"fidex-node/internal/container"
	"fidex-node/internal/crypto"
	"fidex-node/internal/watcher"
)

func main() {
	log.Println("========================================")
	log.Println("Starting FideX Edge Node...")
	log.Println("========================================")

	cfg := mustLoadConfig()
	ensureKeysExist(cfg)
	logAPIKeySecurityWarning()

	appContainer := mustInitContainer(cfg)
	handlers := buildHandlers(appContainer)

	log.Println("Checking for default user...")
	if err := handlers.InitializeDefaultUser(context.Background()); err != nil {
		log.Fatalf("Failed to initialize default user: %v", err)
	}

	fw := mustStartFileWatcher(appContainer)
	startSessionCleanup(appContainer)

	internalServer, publicServer := mustSetupServers(cfg, handlers)

	log.Println("========================================")
	log.Println("✓ FideX Edge Node is running!")
	log.Println("========================================")
	log.Println("Press Ctrl+C to gracefully shutdown...")

	runWithGracefulShutdown(internalServer, publicServer, fw, appContainer)
}

// mustLoadConfig loads node configuration or aborts.
func mustLoadConfig() *config.Config {
	log.Println("Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("✓ Configuration loaded (Internal Port: %d, Public Port: %d)", cfg.InternalAPIPort, cfg.PublicAPIPort)
	return cfg
}

// ensureKeysExist generates the node's RSA key pair on first run.
func ensureKeysExist(cfg *config.Config) {
	if _, err := os.Stat(cfg.PrivateKeyPath); !os.IsNotExist(err) {
		return
	}

	log.Println("No private key found, generating RSA key pair...")
	privateKeyPEM, publicKeyPEM, err := crypto.GenerateKeyPair()
	if err != nil {
		log.Fatalf("Failed to generate key pair: %v", err)
	}

	if err := os.MkdirAll("./keys", 0700); err != nil {
		log.Fatalf("Failed to create keys directory: %v", err)
	}
	if err := os.WriteFile(cfg.PrivateKeyPath, []byte(privateKeyPEM), 0600); err != nil {
		log.Fatalf("Failed to save private key: %v", err)
	}
	if err := os.WriteFile(cfg.PublicKeyPath, []byte(publicKeyPEM), 0644); err != nil {
		log.Fatalf("Failed to save public key: %v", err)
	}

	log.Println("✓ RSA key pair generated and saved")
	log.Println("========================================")
	log.Println("⚠ IMPORTANT: New RSA key pair generated")
	log.Printf("  Public key saved to: %s", cfg.PublicKeyPath)
	log.Printf("  Private key saved to: %s (keep this secure!)", cfg.PrivateKeyPath)
	log.Println("  Share your public key with trading partners via the JWKS endpoint")
	log.Println("========================================")
}

func logAPIKeySecurityWarning() {
	log.Println("========================================")
	log.Println("⚠ SECURITY: Internal API Key")
	log.Println("  Your internal API key is configured and active")
	log.Println("  Access it via: FIDEX_API_KEY environment variable or config file")
	log.Println("  Use it in requests: Authorization: Bearer <your-api-key>")
	log.Println("  NEVER log or expose this key in production!")
	log.Println("========================================")
}

// mustInitContainer wires the service container and aborts on failure.
func mustInitContainer(cfg *config.Config) *container.Container {
	log.Println("Initializing service container...")
	c, err := container.NewContainer(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}
	log.Println("✓ Service container initialized")
	return c
}

// buildHandlers constructs the API Handlers wiring all dependencies from the
// container — replaces the package-level vars used pre-Phase 1.4.
func buildHandlers(c *container.Container) *api.Handlers {
	return &api.Handlers{
		Config:           c.Config,
		DB:               c.DB,
		MessageRepo:      c.MessageRepo,
		PartnerRepo:      c.PartnerRepo,
		UserRepo:         c.UserRepo,
		SessionRepo:      c.SessionRepo,
		AuthService:      c.AuthService,
		DiscoveryService: c.DiscoveryService,
		WebSocketHub:     c.WebSocketHub,
	}
}

// mustStartFileWatcher boots the inbox watcher or aborts.
func mustStartFileWatcher(c *container.Container) *watcher.FileWatcher {
	log.Println("Starting file watcher...")
	fw, err := watcher.NewFileWatcher(c.MessageRepo)
	if err != nil {
		log.Fatalf("Failed to create file watcher: %v", err)
	}
	if err := fw.Start(); err != nil {
		log.Fatalf("Failed to start file watcher: %v", err)
	}
	log.Println("✓ File watcher started")
	return fw
}

// startSessionCleanup launches the background goroutine that prunes expired sessions hourly.
func startSessionCleanup(c *container.Container) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.AuthService.CleanupExpiredSessions(context.Background()); err != nil {
				log.Printf("ERROR: Failed to cleanup expired sessions: %v", err)
			} else {
				log.Println("✓ Expired sessions cleaned up")
			}
		}
	}()
}

// mustSetupServers builds the internal and public HTTP servers, parses the IP
// allowlist, mounts feature routers, and starts both listeners in background.
func mustSetupServers(cfg *config.Config, h *api.Handlers) (*http.Server, *http.Server) {
	allowedIPs, err := api.ParseAllowedIPs(cfg.AllowedIPsString())
	if err != nil {
		log.Fatalf("Failed to parse allowed IPs: %v", err)
	}
	log.Printf("✓ IP Allowlist: %v (Enabled: %v)", cfg.AllowedIPAddresses, cfg.EnableIPAllowlist)

	internalRouter := h.SetupInternalRouter(allowedIPs, cfg.InternalAPIKey, cfg.EnableIPAllowlist)
	publicRouter := h.SetupPublicRouter()

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

	go func() {
		log.Printf("Starting Internal API Server on %s", internalServer.Addr)
		log.Printf("  - POST %s%s (Protected: IP Allowlist + API Key)", constants.APIV1, constants.RouteTransmitRel)
		log.Printf("  - GET  %s (Frontend Constants API)", "/api/constants")
		log.Printf("  - *    %s* (Auth APIs)", constants.APIAuth+"/*")
		log.Printf("  - *    %s* (Dashboard APIs)", constants.APIDashboard+"/*")
		log.Printf("  - *    %s* (Settings APIs)", constants.APISettings+"/*")
		if err := internalServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Internal server error: %v", err)
		}
	}()

	go func() {
		log.Printf("Starting Public API Server on %s", publicServer.Addr)
		log.Printf("  - GET  %s", constants.RouteHealth)
		log.Printf("  - POST %s%s", constants.APIV1, constants.RouteInboundRel)
		log.Printf("  - POST %s%s", constants.APIV1, constants.RouteReceiptRel)
		log.Printf("  - POST %s%s", constants.APIV1, constants.RouteRegisterRel)
		log.Printf("  - GET  %s", constants.RouteJWKS)
		log.Printf("  - GET  %s", constants.RouteAS5Configuration)
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Public server error: %v", err)
		}
	}()

	return internalServer, publicServer
}

// runWithGracefulShutdown blocks on SIGINT/SIGTERM and then tears down resources
// in dependency-safe order: file watcher → container (queue worker + DB) → HTTP servers.
func runWithGracefulShutdown(
	internalServer, publicServer *http.Server,
	fw *watcher.FileWatcher,
	c *container.Container,
) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("\n========================================")
	log.Println("Shutting down FideX Edge Node...")
	log.Println("========================================")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Println("Stopping file watcher...")
	if err := fw.Stop(); err != nil {
		log.Printf("Error stopping file watcher: %v", err)
	} else {
		log.Println("✓ File watcher stopped")
	}

	log.Println("Stopping service container...")
	if err := c.Close(); err != nil {
		log.Printf("Error stopping container: %v", err)
	} else {
		log.Println("✓ Service container stopped")
	}

	log.Println("Stopping internal API server...")
	if err := internalServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down internal server: %v", err)
	} else {
		log.Println("✓ Internal API server stopped")
	}

	log.Println("Stopping public API server...")
	if err := publicServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down public server: %v", err)
	} else {
		log.Println("✓ Public API server stopped")
	}

	log.Println("========================================")
	log.Println("✓ FideX Edge Node shut down successfully")
	log.Println("========================================")
}
