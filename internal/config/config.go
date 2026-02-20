package config

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
)

// Config represents the node's configuration loaded from external sources
type Config struct {
	// Node Identity
	NodeID           string `json:"node_id"`
	OrganizationName string `json:"organization_name"`
	PublicDomain     string `json:"public_domain"`

	// Network Configuration
	InternalAPIPort int `json:"internal_api_port"`
	PublicAPIPort   int `json:"public_api_port"`

	// Security
	InternalAPIKey     string   `json:"internal_api_key"`
	AllowedIPAddresses []string `json:"allowed_ip_addresses"`
	EnableIPAllowlist  bool     `json:"enable_ip_allowlist"`

	// Crypto Keys
	PrivateKeyPath string `json:"private_key_path"`
	PublicKeyPath  string `json:"public_key_path"`

	// Database
	DatabasePath string `json:"database_path"`
}

// Load loads configuration from multiple sources in priority order:
// 1. Command-line flags (highest priority)
// 2. Environment variables
// 3. JSON config file
// 4. Defaults (lowest priority)
func Load() (*Config, error) {
	// Default configuration
	cfg := &Config{
		NodeID:             "fidex-edge-node",
		OrganizationName:   "FideX Edge Node",
		PublicDomain:       "localhost",
		InternalAPIPort:    8080,
		PublicAPIPort:      8443,
		AllowedIPAddresses: []string{"127.0.0.1", "::1"},
		EnableIPAllowlist:  true,
		PrivateKeyPath:     "./keys/private_key.pem",
		PublicKeyPath:      "./keys/public_key.pem",
		DatabasePath:       "./fidex_local.db",
	}

	// Load from JSON config file if specified
	configFile := flag.String("config", "", "Path to JSON config file")
	flag.Parse()

	if *configFile != "" {
		if err := loadFromFile(cfg, *configFile); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Override with environment variables
	loadFromEnv(cfg)

	// Override with command-line flags
	loadFromFlags(cfg)

	// Generate API key if not set
	if cfg.InternalAPIKey == "" {
		cfg.InternalAPIKey = generateAPIKey()
	}

	return cfg, nil
}

// loadFromFile loads configuration from a JSON file
func loadFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv(cfg *Config) {
	if v := os.Getenv("FIDEX_NODE_ID"); v != "" {
		cfg.NodeID = v
	}
	if v := os.Getenv("FIDEX_ORG_NAME"); v != "" {
		cfg.OrganizationName = v
	}
	if v := os.Getenv("FIDEX_PUBLIC_DOMAIN"); v != "" {
		cfg.PublicDomain = v
	}
	if v := os.Getenv("FIDEX_INTERNAL_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.InternalAPIPort = port
		}
	}
	if v := os.Getenv("FIDEX_PUBLIC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.PublicAPIPort = port
		}
	}
	if v := os.Getenv("FIDEX_API_KEY"); v != "" {
		cfg.InternalAPIKey = v
	}
	if v := os.Getenv("FIDEX_ALLOWED_IPS"); v != "" {
		cfg.AllowedIPAddresses = strings.Split(v, ",")
	}
	if v := os.Getenv("FIDEX_ENABLE_IP_ALLOWLIST"); v != "" {
		cfg.EnableIPAllowlist = v == "true" || v == "1"
	}
	if v := os.Getenv("FIDEX_PRIVATE_KEY_PATH"); v != "" {
		cfg.PrivateKeyPath = v
	}
	if v := os.Getenv("FIDEX_PUBLIC_KEY_PATH"); v != "" {
		cfg.PublicKeyPath = v
	}
	if v := os.Getenv("FIDEX_DB_PATH"); v != "" {
		cfg.DatabasePath = v
	}
}

// loadFromFlags loads configuration from command-line flags
func loadFromFlags(cfg *Config) {
	flag.StringVar(&cfg.NodeID, "node-id", cfg.NodeID, "Node ID")
	flag.StringVar(&cfg.OrganizationName, "org-name", cfg.OrganizationName, "Organization name")
	flag.StringVar(&cfg.PublicDomain, "domain", cfg.PublicDomain, "Public domain")
	flag.IntVar(&cfg.InternalAPIPort, "internal-port", cfg.InternalAPIPort, "Internal API port")
	flag.IntVar(&cfg.PublicAPIPort, "public-port", cfg.PublicAPIPort, "Public API port")
	flag.StringVar(&cfg.InternalAPIKey, "api-key", cfg.InternalAPIKey, "Internal API key")
	flag.BoolVar(&cfg.EnableIPAllowlist, "enable-ip-allowlist", cfg.EnableIPAllowlist, "Enable IP allowlist")
	flag.StringVar(&cfg.PrivateKeyPath, "private-key", cfg.PrivateKeyPath, "Private key path")
	flag.StringVar(&cfg.PublicKeyPath, "public-key", cfg.PublicKeyPath, "Public key path")
	flag.StringVar(&cfg.DatabasePath, "db", cfg.DatabasePath, "Database path")

	flag.Parse()
}

// generateAPIKey generates a cryptographically secure random API key
func generateAPIKey() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	key := make([]byte, 32)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := range key {
		// Generate a random index using crypto/rand
		randomIndex, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			// Fallback to a simple incrementing pattern if random generation fails
			// This should never happen in practice
			key[i] = charset[i%len(charset)]
			continue
		}
		key[i] = charset[randomIndex.Int64()]
	}
	return string(key)
}

// Save saves the configuration to a JSON file
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// AllowedIPsString returns the allowed IPs as a JSON string for backward compatibility
func (c *Config) AllowedIPsString() string {
	data, _ := json.Marshal(c.AllowedIPAddresses)
	return string(data)
}
