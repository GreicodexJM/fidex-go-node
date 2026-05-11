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
	"fidex-node/internal/logging"
	"fidex-node/internal/watcher"
)

// mainLogger is the structured logger used by the boot sequence and graceful
// shutdown. must* helpers still call log.Fatalf to abort the process, per the
// project convention "only main.go can panic".
var mainLogger = logging.New("main")

func main() {
	ctx := context.Background()
	mainLogger.Info(ctx, "========================================")
	mainLogger.Info(ctx, "Starting FideX Edge Node...")
	mainLogger.Info(ctx, "========================================")

	cfg := mustLoadConfig()
	ensureKeysExist(cfg)
	logAPIKeySecurityWarning()

	appContainer := mustInitContainer(cfg)
	handlers := buildHandlers(appContainer)

	mainLogger.Info(ctx, "Checking for default user...")
	if err := handlers.InitializeDefaultUser(ctx); err != nil {
		log.Fatalf("Failed to initialize default user: %v", err)
	}

	fw := mustStartFileWatcher(appContainer)
	startSessionCleanup(appContainer)

	internalServer, publicServer := mustSetupServers(cfg, handlers)

	mainLogger.Info(ctx, "========================================")
	mainLogger.Info(ctx, "✓ FideX Edge Node is running!")
	mainLogger.Info(ctx, "========================================")
	mainLogger.Info(ctx, "Press Ctrl+C to gracefully shutdown...")

	runWithGracefulShutdown(internalServer, publicServer, fw, appContainer)
}

// mustLoadConfig loads node configuration or aborts.
func mustLoadConfig() *config.Config {
	ctx := context.Background()
	mainLogger.Info(ctx, "Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	mainLogger.Info(ctx, "✓ Configuration loaded (Internal Port: %d, Public Port: %d)", cfg.InternalAPIPort, cfg.PublicAPIPort)
	return cfg
}

// ensureKeysExist generates the node's RSA key pair on first run.
func ensureKeysExist(cfg *config.Config) {
	if _, err := os.Stat(cfg.PrivateKeyPath); !os.IsNotExist(err) {
		return
	}

	ctx := context.Background()
	mainLogger.Info(ctx, "No private key found, generating RSA key pair...")
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

	mainLogger.Info(ctx, "✓ RSA key pair generated and saved")
	mainLogger.Warn(ctx, "New RSA key pair generated. Public key: %s. Private key: %s (keep secure). Share public key via JWKS endpoint.",
		cfg.PublicKeyPath, cfg.PrivateKeyPath)
}

func logAPIKeySecurityWarning() {
	mainLogger.Warn(context.Background(),
		"Internal API key is active. Configure via FIDEX_API_KEY env or config file. "+
			"Use Authorization: Bearer <key>. Never log this value in production.")
}

// mustInitContainer wires the service container and aborts on failure.
func mustInitContainer(cfg *config.Config) *container.Container {
	ctx := context.Background()
	mainLogger.Info(ctx, "Initializing service container...")
	c, err := container.NewContainer(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}
	mainLogger.Info(ctx, "✓ Service container initialized")
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
	ctx := context.Background()
	mainLogger.Info(ctx, "Starting file watcher...")
	fw, err := watcher.NewFileWatcher(c.MessageRepo)
	if err != nil {
		log.Fatalf("Failed to create file watcher: %v", err)
	}
	if err := fw.Start(); err != nil {
		log.Fatalf("Failed to start file watcher: %v", err)
	}
	mainLogger.Info(ctx, "✓ File watcher started")
	return fw
}

// startSessionCleanup launches the background goroutine that prunes expired sessions hourly.
func startSessionCleanup(c *container.Container) {
	go func() {
		ctx := context.Background()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.AuthService.CleanupExpiredSessions(ctx); err != nil {
				mainLogger.Error(ctx, "Failed to cleanup expired sessions: %v", err)
			} else {
				mainLogger.Info(ctx, "✓ Expired sessions cleaned up")
			}
		}
	}()
}

// mustSetupServers builds the internal and public HTTP servers, parses the IP
// allowlist, mounts feature routers, and starts both listeners in background.
func mustSetupServers(cfg *config.Config, h *api.Handlers) (*http.Server, *http.Server) {
	ctx := context.Background()
	allowedIPs, err := api.ParseAllowedIPs(cfg.AllowedIPsString())
	if err != nil {
		log.Fatalf("Failed to parse allowed IPs: %v", err)
	}
	mainLogger.Info(ctx, "✓ IP Allowlist: %v (Enabled: %v)", cfg.AllowedIPAddresses, cfg.EnableIPAllowlist)

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
		ctx := context.Background()
		mainLogger.Info(ctx, "Starting Internal API Server on %s", internalServer.Addr)
		mainLogger.Info(ctx, "  - POST %s%s (Protected: IP Allowlist + API Key)", constants.APIV1, constants.RouteTransmitRel)
		mainLogger.Info(ctx, "  - GET  %s (Frontend Constants API)", "/api/constants")
		mainLogger.Info(ctx, "  - *    %s* (Auth APIs)", constants.APIAuth+"/*")
		mainLogger.Info(ctx, "  - *    %s* (Dashboard APIs)", constants.APIDashboard+"/*")
		mainLogger.Info(ctx, "  - *    %s* (Settings APIs)", constants.APISettings+"/*")
		if err := internalServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Internal server error: %v", err)
		}
	}()

	go func() {
		ctx := context.Background()
		mainLogger.Info(ctx, "Starting Public API Server on %s", publicServer.Addr)
		mainLogger.Info(ctx, "  - GET  %s", constants.RouteHealth)
		mainLogger.Info(ctx, "  - POST %s%s", constants.APIV1, constants.RouteInboundRel)
		mainLogger.Info(ctx, "  - POST %s%s", constants.APIV1, constants.RouteReceiptRel)
		mainLogger.Info(ctx, "  - POST %s%s", constants.APIV1, constants.RouteRegisterRel)
		mainLogger.Info(ctx, "  - GET  %s", constants.RouteJWKS)
		mainLogger.Info(ctx, "  - GET  %s", constants.RouteAS5Configuration)
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mainLogger.Info(ctx, "========================================")
	mainLogger.Info(ctx, "Shutting down FideX Edge Node...")
	mainLogger.Info(ctx, "========================================")

	mainLogger.Info(ctx, "Stopping file watcher...")
	if err := fw.Stop(); err != nil {
		mainLogger.Error(ctx, "Error stopping file watcher: %v", err)
	} else {
		mainLogger.Info(ctx, "✓ File watcher stopped")
	}

	mainLogger.Info(ctx, "Stopping service container...")
	if err := c.Close(); err != nil {
		mainLogger.Error(ctx, "Error stopping container: %v", err)
	} else {
		mainLogger.Info(ctx, "✓ Service container stopped")
	}

	mainLogger.Info(ctx, "Stopping internal API server...")
	if err := internalServer.Shutdown(ctx); err != nil {
		mainLogger.Error(ctx, "Error shutting down internal server: %v", err)
	} else {
		mainLogger.Info(ctx, "✓ Internal API server stopped")
	}

	mainLogger.Info(ctx, "Stopping public API server...")
	if err := publicServer.Shutdown(ctx); err != nil {
		mainLogger.Error(ctx, "Error shutting down public server: %v", err)
	} else {
		mainLogger.Info(ctx, "✓ Public API server stopped")
	}

	mainLogger.Info(ctx, "========================================")
	mainLogger.Info(ctx, "✓ FideX Edge Node shut down successfully")
	mainLogger.Info(ctx, "========================================")
}
