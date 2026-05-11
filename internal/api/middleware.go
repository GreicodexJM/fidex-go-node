package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// IPAllowlistMiddleware restricts access based on allowed IP addresses
func IPAllowlistMiddleware(allowedIPs []string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the client IP
			clientIP := getClientIP(r)

			logger.Debug(r.Context(), "IP Allowlist Check: Client IP=%s", clientIP)

			// Check if IP is in the allowlist
			if !isIPAllowed(clientIP, allowedIPs) {
				logger.Warn(r.Context(), "Access denied: IP %s not in allowlist", clientIP)
				respondWithError(w, http.StatusForbidden, "Access denied", fmt.Errorf("IP address not authorized"))
				return
			}

			// IP is allowed, proceed
			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyMiddleware validates the Authorization header contains a valid Bearer token
func APIKeyMiddleware(expectedKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the Authorization header
			authHeader := r.Header.Get("Authorization")

			if authHeader == "" {
				logger.Warn(r.Context(), "Access denied: Missing Authorization header from %s", r.RemoteAddr)
				respondWithError(w, http.StatusUnauthorized, "Unauthorized", fmt.Errorf("missing authorization header"))
				return
			}

			// Check if it's a Bearer token
			if !strings.HasPrefix(authHeader, "Bearer ") {
				logger.Warn(r.Context(), "Access denied: Invalid authorization format from %s", r.RemoteAddr)
				respondWithError(w, http.StatusUnauthorized, "Unauthorized", fmt.Errorf("invalid authorization format"))
				return
			}

			// Extract the token
			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Validate the token
			if token != expectedKey {
				logger.Warn(r.Context(), "Access denied: Invalid API key from %s", r.RemoteAddr)
				respondWithError(w, http.StatusUnauthorized, "Unauthorized", fmt.Errorf("invalid API key"))
				return
			}

			// Valid API key, proceed
			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the real client IP from the request
// It checks X-Forwarded-For, X-Real-IP headers first, then falls back to RemoteAddr
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (used by proxies)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header (used by some proxies)
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	// RemoteAddr format is "IP:Port", we need to extract just the IP
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If SplitHostPort fails, return RemoteAddr as-is
		return r.RemoteAddr
	}

	return ip
}

// isIPAllowed checks if the client IP is in the allowlist
func isIPAllowed(clientIP string, allowedIPs []string) bool {
	// Parse the client IP
	parsedClientIP := net.ParseIP(clientIP)
	if parsedClientIP == nil {
		logger.Warn(nil, "Could not parse client IP: %s", clientIP)
		return false
	}

	// Check against each allowed IP
	for _, allowedIP := range allowedIPs {
		// Handle CIDR notation (e.g., "192.168.1.0/24")
		if strings.Contains(allowedIP, "/") {
			_, ipNet, err := net.ParseCIDR(allowedIP)
			if err != nil {
				logger.Warn(nil, "Invalid CIDR notation in allowlist: %s", allowedIP)
				continue
			}
			if ipNet.Contains(parsedClientIP) {
				return true
			}
		} else {
			// Direct IP comparison
			parsedAllowedIP := net.ParseIP(allowedIP)
			if parsedAllowedIP == nil {
				logger.Warn(nil, "Invalid IP in allowlist: %s", allowedIP)
				continue
			}
			if parsedClientIP.Equal(parsedAllowedIP) {
				return true
			}
		}
	}

	return false
}

// ParseAllowedIPs parses a JSON array string of allowed IPs
// It also supports comma-separated strings for backward compatibility
func ParseAllowedIPs(input string) ([]string, error) {
	var ips []string

	// Try parsing as JSON first
	if err := json.Unmarshal([]byte(input), &ips); err == nil {
		return ips, nil
	}

	// If JSON parsing fails, assume it's a comma-separated string
	// This handles the case where data was saved incorrectly
	parts := strings.Split(input, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			ips = append(ips, trimmed)
		}
	}

	// If we found IPs via splitting, return them
	if len(ips) > 0 {
		return ips, nil
	}

	// If input was empty, return empty list
	if input == "" {
		return []string{}, nil
	}

	// If we couldn't parse anything meaningful but input wasn't empty, return error
	return nil, fmt.Errorf("failed to parse allowed IPs: invalid format")
}
