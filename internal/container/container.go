package container

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"fidex-node/internal/auth"
	"fidex-node/internal/config"
	"fidex-node/internal/crypto"
	"fidex-node/internal/dashboard"
	"fidex-node/internal/discovery"
	"fidex-node/internal/domain"
	"fidex-node/internal/logging"
	"fidex-node/internal/queue"
	"fidex-node/internal/repository"
)

// logger is the package-level structured logger for container lifecycle events.
var logger = logging.New("container")

// Container holds all application dependencies and provides dependency injection
type Container struct {
	// Configuration
	Config *config.Config

	// Database connection
	DB *sql.DB

	// Repositories
	MessageRepo domain.MessageRepository
	PartnerRepo domain.PartnerRepository
	UserRepo    domain.UserRepository
	SessionRepo domain.SessionRepository

	// Services
	CryptoService    *crypto.AS5Engine
	DiscoveryService *discovery.DiscoveryService
	TokenStore       *discovery.TokenStore
	AuthService      *auth.Service

	// Workers
	QueueWorker  *queue.Worker
	WebSocketHub *dashboard.Hub
}

// NewContainer creates and initializes a new service container with all dependencies
func NewContainer(cfg *config.Config) (*Container, error) {
	container := &Container{
		Config: cfg,
	}

	// Initialize database
	if err := container.initDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize repositories
	if err := container.initRepositories(); err != nil {
		return nil, fmt.Errorf("failed to initialize repositories: %w", err)
	}

	// Initialize crypto service
	if err := container.initCryptoService(); err != nil {
		return nil, fmt.Errorf("failed to initialize crypto service: %w", err)
	}

	// Initialize auth service (depends on repos)
	container.AuthService = auth.NewService(container.SessionRepo, container.UserRepo)

	// Initialize discovery service
	if err := container.initDiscoveryService(); err != nil {
		return nil, fmt.Errorf("failed to initialize discovery service: %w", err)
	}

	// Initialize workers
	if err := container.initWorkers(); err != nil {
		return nil, fmt.Errorf("failed to initialize workers: %w", err)
	}

	logger.Info(context.Background(), "✓ Service container initialized successfully")
	return container, nil
}

// initDatabase initializes the database connection and schema using the
// repository package, with no global state.
func (c *Container) initDatabase() error {
	conn, err := repository.OpenSQLite(c.Config.DatabasePath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err := repository.InitSchema(conn); err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	c.DB = conn
	logger.Info(context.Background(), "✓ Database initialized")
	return nil
}

// initRepositories creates repository instances with proper dependency injection
func (c *Container) initRepositories() error {
	// Use proper repository implementations with injected database connection
	c.MessageRepo = repository.NewSQLiteMessageRepository(c.DB)
	c.PartnerRepo = repository.NewSQLitePartnerRepository(c.DB)
	c.UserRepo = repository.NewSQLiteUserRepository(c.DB)
	c.SessionRepo = repository.NewSQLiteSessionRepository(c.DB)

	logger.Info(context.Background(), "✓ Repositories initialized")
	return nil
}

// initCryptoService initializes the cryptography service
func (c *Container) initCryptoService() error {
	// Load private key
	privateKeyPEM, err := loadPrivateKey(c.Config.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	// Create AS5 engine
	engine, err := crypto.NewAS5Engine(privateKeyPEM, c.Config.NodeID)
	if err != nil {
		return fmt.Errorf("failed to create AS5 engine: %w", err)
	}

	c.CryptoService = engine
	logger.Info(context.Background(), "✓ Crypto service initialized")
	return nil
}

// initDiscoveryService initializes the partner discovery service
func (c *Container) initDiscoveryService() error {
	// Create token store for security tokens
	c.TokenStore = discovery.NewTokenStore()

	// Create node configuration for discovery
	// Build the base URL from the public domain and port
	baseURL := fmt.Sprintf("https://%s:%d", c.Config.PublicDomain, c.Config.PublicAPIPort)
	nodeConfig := discovery.NodeConfig{
		NodeID:                 c.Config.NodeID,
		OrganizationName:       c.Config.OrganizationName,
		BaseURL:                baseURL,
		PublicDomain:           c.Config.PublicDomain,
		SupportedDocumentTypes: []string{},
	}

	// Create discovery service
	c.DiscoveryService = discovery.NewDiscoveryService(nodeConfig, c.TokenStore, c.PartnerRepo)

	logger.Info(context.Background(), "✓ Discovery service initialized")
	return nil
}

// initWorkers initializes background workers
func (c *Container) initWorkers() error {
	// Initialize queue worker with dependencies
	c.QueueWorker = queue.NewWorker(
		c.MessageRepo,
		c.PartnerRepo,
		c.CryptoService,
	)
	c.QueueWorker.SetNodeID(c.Config.NodeID)
	c.QueueWorker.Start()

	// Initialize WebSocket hub
	c.WebSocketHub = dashboard.NewHub()
	go c.WebSocketHub.Run()

	logger.Info(context.Background(), "✓ Workers initialized")
	return nil
}

// Close gracefully shuts down all services and closes connections
func (c *Container) Close() error {
	ctx := context.Background()
	logger.Info(ctx, "Shutting down service container...")

	// Stop queue worker
	if c.QueueWorker != nil {
		logger.Info(ctx, "Stopping queue worker...")
		c.QueueWorker.Stop()
	}

	// Close database connection
	if c.DB != nil {
		logger.Info(ctx, "Closing database connection...")
		if err := c.DB.Close(); err != nil {
			logger.Error(ctx, "Error closing database: %v", err)
			return err
		}
	}

	logger.Info(ctx, "✓ Service container shut down successfully")
	return nil
}

// Helper functions

func loadPrivateKey(path string) (string, error) {
	data, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read private key file: %w", err)
	}
	return string(data), nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
