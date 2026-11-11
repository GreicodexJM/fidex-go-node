package dashboard

import (
	"fmt"

	"github.com/skip2/go-qrcode"
)

// GenerateQRCode generates a QR code image containing the discovery URL
// Returns the PNG image as a byte slice
func GenerateQRCode(discoveryURL string, size int) ([]byte, error) {
	if discoveryURL == "" {
		return nil, fmt.Errorf("discovery URL cannot be empty")
	}

	if size <= 0 {
		size = 256 // Default size
	}

	// Generate QR code with medium error correction
	png, err := qrcode.Encode(discoveryURL, qrcode.Medium, size)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	return png, nil
}
