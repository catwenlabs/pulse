package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"testing"
)

func TestAllowedAddress(t *testing.T) {
	tests := []struct {
		address string
		allowed bool
	}{
		{address: "8.8.8.8", allowed: true},
		{address: "1.1.1.1", allowed: true},
		{address: "127.0.0.1", allowed: false},
		{address: "10.0.0.1", allowed: false},
		{address: "172.16.0.1", allowed: false},
		{address: "192.168.1.1", allowed: false},
		{address: "169.254.169.254", allowed: false},
		{address: "100.64.0.1", allowed: false},
		{address: "::1", allowed: false},
		{address: "fc00::1", allowed: false},
		{address: "fe80::1", allowed: false},
	}

	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			address := netip.MustParseAddr(test.address)
			if got := AllowedAddress(address); got != test.allowed {
				t.Errorf("AllowedAddress(%s) = %v, want %v", address, got, test.allowed)
			}
		})
	}
}

func TestSafeDialerRejectsPrivateDNSResult(t *testing.T) {
	dialer := SafeDialer{
		Lookup: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
		},
		Dial: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("Dial must not be called for blocked address")
			return nil, nil
		},
	}

	_, err := dialer.DialContext(context.Background(), "tcp", "metadata.example:80")
	if !errors.Is(err, ErrAddressBlocked) {
		t.Fatalf("DialContext() error = %v, want ErrAddressBlocked", err)
	}
}

func TestSafeDialerDialsValidatedIPAddress(t *testing.T) {
	var dialed string
	dialer := SafeDialer{
		Lookup: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		Dial: func(_ context.Context, _, address string) (net.Conn, error) {
			dialed = address
			return nil, errors.New("stop after validation")
		},
	}

	_, _ = dialer.DialContext(context.Background(), "tcp", "feed.example:443")
	if dialed != "8.8.8.8:443" {
		t.Errorf("dialed = %q, want validated IP", dialed)
	}
}

func TestSafeDialerRejectsMalformedAddressAndLookupFailure(t *testing.T) {
	dialer := SafeDialer{
		Lookup: func(context.Context, string) ([]netip.Addr, error) {
			return nil, errors.New("DNS failed")
		},
	}
	if _, err := dialer.DialContext(context.Background(), "tcp", "missing-port"); err == nil {
		t.Fatal("malformed address error = nil")
	}
	if _, err := dialer.DialContext(context.Background(), "tcp", "feed.example:80"); err == nil {
		t.Fatal("lookup error = nil")
	}
}

func TestClientRedirectPolicy(t *testing.T) {
	client := New()
	valid, _ := http.NewRequest(http.MethodGet, "https://example.com/feed", nil)
	if err := client.CheckRedirect(valid, nil); err != nil {
		t.Fatalf("valid redirect error = %v", err)
	}

	ftp := &http.Request{URL: &url.URL{Scheme: "ftp", Host: "example.com"}}
	if err := client.CheckRedirect(ftp, nil); err == nil {
		t.Fatal("FTP redirect error = nil")
	}
	withCredentials := &http.Request{URL: &url.URL{
		Scheme: "https", Host: "example.com", User: url.User("secret"),
	}}
	if err := client.CheckRedirect(withCredentials, nil); err == nil {
		t.Fatal("credential redirect error = nil")
	}
	via := make([]*http.Request, 5)
	if err := client.CheckRedirect(valid, via); err == nil {
		t.Fatal("redirect limit error = nil")
	}
}

func TestSafeDialerRejectsPrivateLiteral(t *testing.T) {
	dialer := SafeDialer{
		Dial: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("Dial must not be called")
			return nil, nil
		},
	}
	if _, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:80"); !errors.Is(err, ErrAddressBlocked) {
		t.Fatalf("error = %v, want ErrAddressBlocked", err)
	}
}

func TestAIClientUsesExplicitLocalHostAllowlist(t *testing.T) {
	if !isLocalAIHost("host.docker.internal") || !isLocalAIHost("127.0.0.1") {
		t.Fatal("expected supported local AI hosts to be allowed")
	}
	if isLocalAIHost("169.254.169.254") || isLocalAIHost("private.example") {
		t.Fatal("unexpected local AI host allowlist entry")
	}
	dialer := localAIDialer{}
	if _, err := dialer.DialContext(context.Background(), "tcp", "169.254.169.254:80"); !errors.Is(err, ErrAddressBlocked) {
		t.Fatalf("local AI dialer error = %v, want ErrAddressBlocked", err)
	}
}
