// Package netguard builds HTTP clients for requests whose destination a
// trading partner chose: its AS5 configuration, its JWKS, its register,
// receive_message and receive_receipt endpoints.
//
// Those URLs are partner-written text. Without a guard, a partner (or anybody
// holding an invitation token) could point this node at its own loopback
// interface, the cloud metadata service, or the private network behind it
// (SSRF). The guard runs at DIAL time on the resolved address, so it also
// covers DNS names that resolve to private addresses and DNS rebinding, and
// redirects are not followed at all.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned (wrapped) when a dial targets an address the
// guard refuses.
var ErrBlockedAddress = errors.New("destination address not allowed")

// blockedPrefixes are refused in addition to what netip.Addr's predicates
// cover (loopback, private, link-local, multicast, unspecified).
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),  // carrier-grade NAT (RFC 6598)
	netip.MustParsePrefix("192.0.0.0/24"),   // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),  // benchmarking (RFC 2544)
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved, incl. broadcast
	netip.MustParsePrefix("64:ff9b:1::/48"), // local-use NAT64 (RFC 8215)
	netip.MustParsePrefix("2001:db8::/32"),  // documentation
	netip.MustParsePrefix("fec0::/10"),      // deprecated site-local
}

// AddressAllowed reports whether a dial to addr is allowed when private
// networks are not.
func AddressAllowed(addr netip.Addr) bool {
	addr = addr.Unmap() // ::ffff:127.0.0.1 is 127.0.0.1
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// control is a net.Dialer Control hook: address is the resolved "ip:port".
func control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrBlockedAddress, address)
	}
	if !AddressAllowed(ap.Addr()) {
		return fmt.Errorf("%w: %s is loopback, private, link-local or reserved", ErrBlockedAddress, ap.Addr())
	}
	return nil
}

// NewClient returns an HTTP client for partner-chosen URLs.
//
// Redirects are never followed (a 3xx is returned to the caller, which treats
// any non-200 as failure). With allowPrivate=false every dial is checked
// against AddressAllowed and the environment's HTTP proxy is NOT used (a
// proxy would hide the real destination from the dial-time check).
// allowPrivate=true is for development and test networks (docker bridge,
// localhost) where partners legitimately live on private addresses.
func NewClient(timeout time.Duration, allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if !allowPrivate {
		dialer.Control = control
		transport.Proxy = nil
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
