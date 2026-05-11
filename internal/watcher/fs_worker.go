package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fidex-node/internal/domain"
	"fidex-node/internal/logging"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
)

// logger is the package-level structured logger for the file watcher.
var logger = logging.New("watcher")

const (
	outboxDir  = "./fidex/outbox"
	archiveDir = "./fidex/archive"
)

// FileWatcher manages the fsnotify watcher for the outbox directory
type FileWatcher struct {
	watcher     *fsnotify.Watcher
	done        chan bool
	messageRepo domain.MessageRepository
}

// NewFileWatcher creates a new FileWatcher instance with an injected message repository
func NewFileWatcher(messageRepo domain.MessageRepository) (*FileWatcher, error) {
	if messageRepo == nil {
		return nil, fmt.Errorf("messageRepo cannot be nil")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	return &FileWatcher{
		watcher:     watcher,
		done:        make(chan bool),
		messageRepo: messageRepo,
	}, nil
}

// Start initializes the directories and starts watching for file changes
func (fw *FileWatcher) Start() error {
	// Ensure directories exist
	if err := ensureDirectories(); err != nil {
		return fmt.Errorf("failed to ensure directories: %w", err)
	}

	// Add the outbox directory to the watcher
	if err := fw.watcher.Add(outboxDir); err != nil {
		return fmt.Errorf("failed to watch outbox directory: %w", err)
	}

	logger.Info(context.Background(), "File watcher started, monitoring: %s", outboxDir)

	// Start the event loop in a goroutine
	go fw.watchLoop()

	return nil
}

// Stop gracefully stops the file watcher
func (fw *FileWatcher) Stop() error {
	close(fw.done)
	return fw.watcher.Close()
}

// watchLoop is the main event loop that processes file system events
func (fw *FileWatcher) watchLoop() {
	ctx := context.Background()
	// Debouncing map to track recently processed files
	processedFiles := make(map[string]time.Time)
	debounceDelay := 500 * time.Millisecond

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Only process Write and Create events for .json files
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if isJSONFile(event.Name) {
					// Debouncing: avoid processing the same file multiple times rapidly
					if lastProcessed, exists := processedFiles[event.Name]; exists {
						if time.Since(lastProcessed) < debounceDelay {
							continue
						}
					}

					// Update the processed timestamp
					processedFiles[event.Name] = time.Now()

					// Process the file
					if err := fw.processFile(event.Name); err != nil {
						logger.Error(ctx, "Error processing file %s: %v", event.Name, err)
					}

					// Clean up old entries from the debouncing map
					fw.cleanupProcessedFiles(processedFiles)
				}
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			logger.Error(ctx, "File watcher error: %v", err)

		case <-fw.done:
			logger.Info(ctx, "File watcher stopped")
			return
		}
	}
}

// TransmitRequest represents the schema for outbound messages from ERP
type TransmitRequest struct {
	DestinationPartnerID string                 `json:"destination_partner_id"`
	DocumentType         string                 `json:"document_type"`
	ReceiptWebhook       string                 `json:"receipt_webhook,omitempty"`
	Payload              map[string]interface{} `json:"payload"`
}

// processFile reads, validates, and processes a JSON file from the outbox
func (fw *FileWatcher) processFile(filePath string) error {
	ctx := context.Background()
	logger.Info(ctx, "Processing file: %s", filePath)

	// Wait a bit to ensure the file is fully written
	time.Sleep(100 * time.Millisecond)

	// Read the file contents
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Validate that it's valid JSON and conforms to TransmitRequest schema
	var transmitReq TransmitRequest
	if err := json.Unmarshal(fileData, &transmitReq); err != nil {
		return fmt.Errorf("invalid JSON in file: %w", err)
	}

	// Validate required fields
	if err := validateTransmitRequest(&transmitReq); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Generate a new UUID for the FideX message_id
	messageID := uuid.New().String()

	// Create a message record
	msg := &domain.Message{
		MessageID: messageID,
		Direction: domain.DirectionOutbound,
		Status:    domain.StatusQueued,
		Payload:   string(fileData),
		CreatedAt: time.Now(),
	}

	// Save via repository
	if err := fw.messageRepo.Create(ctx, msg); err != nil {
		return fmt.Errorf("failed to insert message into database: %w", err)
	}

	logger.Info(ctx, "Message saved to database with ID: %s", messageID)

	// Move the file to the archive directory
	archivePath := filepath.Join(archiveDir, fmt.Sprintf("%s.json", messageID))
	if err := os.Rename(filePath, archivePath); err != nil {
		// If rename fails, try copy and delete (e.g., across filesystems)
		if err := copyFile(filePath, archivePath); err != nil {
			return fmt.Errorf("failed to archive file: %w", err)
		}
		if err := os.Remove(filePath); err != nil {
			logger.Warn(ctx, "Failed to remove original file %s: %v", filePath, err)
		}
	}

	logger.Info(ctx, "File archived to: %s", archivePath)
	return nil
}

// cleanupProcessedFiles removes old entries from the processed files map
func (fw *FileWatcher) cleanupProcessedFiles(processedFiles map[string]time.Time) {
	cutoff := time.Now().Add(-5 * time.Minute)
	for file, timestamp := range processedFiles {
		if timestamp.Before(cutoff) {
			delete(processedFiles, file)
		}
	}
}

// ensureDirectories creates the outbox and archive directories if they don't exist
func ensureDirectories() error {
	ctx := context.Background()
	directories := []string{outboxDir, archiveDir}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		logger.Info(ctx, "Directory ready: %s", dir)
	}

	return nil
}

// isJSONFile checks if a file has a .json extension
func isJSONFile(filePath string) bool {
	return strings.HasSuffix(strings.ToLower(filePath), ".json")
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceData, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}

	if err := os.WriteFile(dst, sourceData, 0644); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	return nil
}

// validateTransmitRequest validates the required fields of a TransmitRequest
func validateTransmitRequest(req *TransmitRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if req.DestinationPartnerID == "" {
		return fmt.Errorf("destination_partner_id is required")
	}

	if req.DocumentType == "" {
		return fmt.Errorf("document_type is required")
	}

	if req.Payload == nil || len(req.Payload) == 0 {
		return fmt.Errorf("payload is required and cannot be empty")
	}

	return nil
}
