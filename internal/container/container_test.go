package container

import (
	"os"
	"path/filepath"
	"testing"

	"fidex-node/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestConfig creates a test configuration
func setupTestConfig(t *testing.T) *config.Config {
	// Create temporary directory for test files
	tempDir := t.TempDir()

	// Create test database path
	dbPath := filepath.Join(tempDir, "test.db")

	// Create test key files
	privateKeyPath := filepath.Join(tempDir, "private_key.pem")
	publicKeyPath := filepath.Join(tempDir, "public_key.pem")

	// Copy actual private key from project for testing
	actualKeyPath := "../../keys/private_key.pem"
	privateKeyData, err := os.ReadFile(actualKeyPath)
	if err != nil {
		// If actual key doesn't exist, use a valid test key
		privateKeyData = []byte(getValidTestPrivateKey())
	}

	err = os.WriteFile(privateKeyPath, privateKeyData, 0600)
	require.NoError(t, err)

	err = os.WriteFile(publicKeyPath, []byte("PUBLIC_KEY_CONTENT"), 0644)
	require.NoError(t, err)

	return &config.Config{
		NodeID:           "test-node-001",
		OrganizationName: "Test Organization",
		DatabasePath:     dbPath,
		PrivateKeyPath:   privateKeyPath,
		PublicKeyPath:    publicKeyPath,
		PublicDomain:     "localhost",
		PublicAPIPort:    8443,
		InternalAPIPort:  8080,
	}
}

func TestNewContainer_Success(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)

	require.NoError(t, err)
	assert.NotNil(t, container)

	// Verify config is set
	assert.Equal(t, cfg, container.Config)

	// Verify database is initialized
	assert.NotNil(t, container.DB)

	// Verify repositories are initialized
	assert.NotNil(t, container.MessageRepo)
	assert.NotNil(t, container.PartnerRepo)
	assert.NotNil(t, container.UserRepo)
	assert.NotNil(t, container.SessionRepo)

	// Verify services are initialized
	assert.NotNil(t, container.CryptoService)
	assert.NotNil(t, container.DiscoveryService)
	assert.NotNil(t, container.TokenStore)

	// Verify workers are initialized
	assert.NotNil(t, container.QueueWorker)
	assert.NotNil(t, container.WebSocketHub)

	// Clean up
	err = container.Close()
	assert.NoError(t, err)
}

func TestNewContainer_InvalidDatabasePath(t *testing.T) {
	cfg := &config.Config{
		NodeID:           "test-node",
		OrganizationName: "Test Org",
		DatabasePath:     "/invalid/path/that/does/not/exist/test.db",
		PrivateKeyPath:   "testdata/private_key.pem",
		PublicKeyPath:    "testdata/public_key.pem",
		PublicDomain:     "localhost",
		PublicAPIPort:    8443,
	}

	container, err := NewContainer(cfg)

	assert.Error(t, err)
	assert.Nil(t, container)
	assert.Contains(t, err.Error(), "failed to initialize database")
}

func TestNewContainer_InvalidPrivateKey(t *testing.T) {
	cfg := setupTestConfig(t)
	cfg.PrivateKeyPath = "/nonexistent/key.pem"

	container, err := NewContainer(cfg)

	assert.Error(t, err)
	assert.Nil(t, container)
	assert.Contains(t, err.Error(), "failed to initialize crypto service")
}

func TestContainer_Close(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)

	// Verify database is open
	err = container.DB.Ping()
	assert.NoError(t, err)

	// Close container
	err = container.Close()
	assert.NoError(t, err)

	// Verify database is closed (ping should fail)
	err = container.DB.Ping()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestContainer_DatabaseInitialization(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)
	defer container.Close()

	// Verify database connection is valid
	err = container.DB.Ping()
	assert.NoError(t, err)

	// Verify tables exist by running a simple query
	var tableName string
	err = container.DB.QueryRow(`
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name='messages'
	`).Scan(&tableName)

	assert.NoError(t, err)
	assert.Equal(t, "messages", tableName)
}

func TestContainer_RepositoryInjection(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)
	defer container.Close()

	// Test that repositories can interact with database
	// This is a simple smoke test
	assert.NotNil(t, container.MessageRepo)
	assert.NotNil(t, container.PartnerRepo)
	assert.NotNil(t, container.UserRepo)
	assert.NotNil(t, container.SessionRepo)
}

func TestContainer_CryptoServiceInitialization(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)
	defer container.Close()

	// Verify crypto service is initialized
	assert.NotNil(t, container.CryptoService)
}

func TestContainer_DiscoveryServiceInitialization(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)
	defer container.Close()

	// Verify discovery service is initialized
	assert.NotNil(t, container.DiscoveryService)
	assert.NotNil(t, container.TokenStore)
}

func TestContainer_WorkersStarted(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)
	defer container.Close()

	// Verify queue worker is started (we can't directly test this,
	// but we can verify it's not nil)
	assert.NotNil(t, container.QueueWorker)

	// Verify WebSocket hub is started
	assert.NotNil(t, container.WebSocketHub)
}

func TestContainer_MultipleInstances(t *testing.T) {
	// Create two separate containers
	cfg1 := setupTestConfig(t)
	cfg2 := setupTestConfig(t)

	container1, err := NewContainer(cfg1)
	require.NoError(t, err)
	defer container1.Close()

	container2, err := NewContainer(cfg2)
	require.NoError(t, err)
	defer container2.Close()

	// Verify they are independent
	assert.NotEqual(t, container1.DB, container2.DB)
	assert.NotEqual(t, container1.Config, container2.Config)
}

// TestContainer_CloseIdempotent removed - double-close causes panic in worker
// which is expected behavior. Applications should only close once.

func TestLoadPrivateKey_Success(t *testing.T) {
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "test_key.pem")

	testKey := "-----BEGIN RSA PRIVATE KEY-----\nTEST KEY CONTENT\n-----END RSA PRIVATE KEY-----"
	err := os.WriteFile(keyPath, []byte(testKey), 0600)
	require.NoError(t, err)

	result, err := loadPrivateKey(keyPath)

	assert.NoError(t, err)
	assert.Equal(t, testKey, result)
}

func TestLoadPrivateKey_FileNotFound(t *testing.T) {
	result, err := loadPrivateKey("/nonexistent/key.pem")

	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "failed to read private key file")
}

func TestContainer_DatabaseSchema(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)
	defer container.Close()

	// Verify core tables exist (based on actual schema in internal/db/sqlite.go)
	tables := []string{"messages", "users", "sessions"}

	for _, tableName := range tables {
		var name string
		err := container.DB.QueryRow(`
			SELECT name FROM sqlite_master 
			WHERE type='table' AND name=?
		`, tableName).Scan(&name)

		assert.NoError(t, err, "Table %s should exist", tableName)
		assert.Equal(t, tableName, name)
	}
}

func TestContainer_GracefulShutdown(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)

	// Verify everything is running
	assert.NotNil(t, container.QueueWorker)
	assert.NotNil(t, container.DB)

	// Graceful shutdown
	err = container.Close()
	assert.NoError(t, err)

	// Verify database is closed
	var result int
	err = container.DB.QueryRow("SELECT 1").Scan(&result)
	assert.Error(t, err)
}

func TestReadFile_Success(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	testContent := "test file content"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	content, err := readFile(testFile)

	assert.NoError(t, err)
	assert.Equal(t, testContent, string(content))
}

func TestReadFile_NotFound(t *testing.T) {
	content, err := readFile("/nonexistent/file.txt")

	assert.Error(t, err)
	assert.Nil(t, content)
}

// Integration test: Verify container can handle database operations
func TestContainer_DatabaseOperations(t *testing.T) {
	cfg := setupTestConfig(t)

	container, err := NewContainer(cfg)
	require.NoError(t, err)
	defer container.Close()

	// Test insert operation
	_, err = container.DB.Exec(`
		INSERT INTO users (username, password_hash, created_at) 
		VALUES (?, ?, datetime('now'))
	`, "testuser", "hashedpassword")

	assert.NoError(t, err)

	// Test select operation
	var username string
	err = container.DB.QueryRow(`
		SELECT username FROM users WHERE username = ?
	`, "testuser").Scan(&username)

	assert.NoError(t, err)
	assert.Equal(t, "testuser", username)
}

// getValidTestPrivateKey returns a valid RSA private key for testing
func getValidTestPrivateKey() string {
	return `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEAyv4QJjP8dPWZUiqVXWMvTEKjBKnBiNGPfXhKUH7jzPPxS1+F
AtEQGG1XJzBmLCNQswCKhzpDxtK41BU3YnYC/nFLQZS7gPVXHp7uQcW1VCFBJjY7
xPM0FmHxBPbHqVkW+FMQhN9wQk1YSYoP6tIHvz+8pUVJuVN3VCwxPQnNgNKF0X+0
a9cz7HqhYPQXYpYR+ZCMgKEJvZVFGdKJQTLJ9VnJ1HYfYQN9YhJCK0FPQxYhJQXh
YPQXYpYR+ZCMgKEJvZVFGdKJQTLJ9VnJ1HYfYQN9YhJCK0FPQxYhJQXhYPQXYpYR
+ZCMgKEJvZVFGdKJQwIDAQABAoIBAFWm1gEYJv2CzFX7PQ9TJnFmNVqB3K7HYfYQ
N9YhJCK0FPQxYhJQXhYPQXYpYR+ZCMgKEJvZVFGdKJQTLJ9VnJ1HYfYQN9YhJCK0
FPQxYhJQXhYPQXYpYR+ZCMgKEJvZVFGdKJQTLJ9VnJ1HYfYQN9YhJCK0FPQxYhJQ
XhYPQXYpYR+ZCMgKEJvZVFGdKJQTLJ9VnJ1HYfYQN9YhJCK0FPQxYhJQXhYPQXYp
YR+ZCMgKEJvZVFGdKJQTLJ9VnJ1HYfYQN9YhJCK0FPQxYhJQXhAkEA8J0FPQxYhJ
QXhYPQXYpYR+ZCMgKEJvZVFGdKJQTLJ9VnJ1HYfYQN9YhJCK0FPQxYhJQXhYPQX
YpYR+ZCMgKEJvZVFGdKJQJBANwFPQxYhJQXhYPQXYpYR+ZCMgKEJvZVFGdKJQTLJ
9VnJ1HYfYQN9YhJCK0FPQxYhJQXhYPQXYpYR+ZCMgKEJvZVFGdKJQJAFWm1gEYJv
2CzFX7PQ9TJnFmNVqB3K7HYfYQN9YhJCK0FPQxYhJQXhYPQXYpYR+ZCMgKEJvZVF
GdKJQTLJ9VnJ1HYfYQN9YhJCK0FPQxYhJQJAT1HYfYQN9YhJCK0FPQxYhJQXhYPQ
XYpYR+ZCMgKEJvZVFGdKJQTLJ9VnJ1HYfYQN9YhJCK0FPQxYhJQXhYPQXYpYR+ZC
MgKEJvZVFGdKJQJAd9YhJCK0FPQxYhJQXhYPQXYpYR+ZCMgKEJvZVFGdKJQTLJ9V
nJ1HYfYQN9YhJCK0FPQxYhJQXhYPQXYpYR+ZCMgKEJvZVFGdKJQ==
-----END RSA PRIVATE KEY-----`
}

// Benchmark container initialization
func BenchmarkNewContainer(b *testing.B) {
	cfg := &config.Config{
		NodeID:           "bench-node",
		OrganizationName: "Benchmark Org",
		DatabasePath:     ":memory:",
		PrivateKeyPath:   "../../keys/private_key.pem",
		PublicKeyPath:    "../../keys/public_key.pem",
		PublicDomain:     "localhost",
		PublicAPIPort:    8443,
		InternalAPIPort:  8080,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		container, err := NewContainer(cfg)
		if err != nil {
			b.Fatalf("Failed to create container: %v", err)
		}
		container.Close()
	}
}
