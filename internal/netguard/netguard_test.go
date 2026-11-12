package netguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestAddressAllowed(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "10.1.2.3", "172.17.0.2", "192.168.1.1", "169.254.169.254",
		"0.0.0.0", "100.64.0.1", "fd00::1", "fe80::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"224.0.0.1", "255.255.255.255", "::",
	}
	for _, s := range blocked {
		if AddressAllowed(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !AddressAllowed(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestGuardedClientRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	if _, err := NewClient(5*time.Second, false).Get(srv.URL); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("guarded client reached loopback: err=%v", err)
	}
	resp, err := NewClient(5*time.Second, true).Get(srv.URL)
	if err != nil {
		t.Fatalf("allowPrivate client should reach loopback: %v", err)
	}
	resp.Body.Close()
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect was followed")
	}))
	defer target.Close()
	redir := httptest.NewServer(http.RedirectHandler(target.URL, http.StatusFound))
	defer redir.Close()

	resp, err := NewClient(5*time.Second, true).Get(redir.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want the 302 itself", resp.StatusCode)
	}
}
