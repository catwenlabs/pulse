package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrAddressBlocked = errors.New("network address is blocked")

type LookupFunc func(context.Context, string) ([]netip.Addr, error)
type DialFunc func(context.Context, string, string) (net.Conn, error)

type SafeDialer struct {
	Lookup LookupFunc
	Dial   DialFunc
}

func (dialer SafeDialer) DialContext(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split network address %q: %w", address, err)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return nil, fmt.Errorf("invalid network port %q: %w", port, err)
	}

	addresses, err := dialer.lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	for _, candidate := range addresses {
		candidate = candidate.Unmap()
		if !AllowedAddress(candidate) {
			continue
		}
		conn, dialErr := dialer.dial(ctx, network, net.JoinHostPort(candidate.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		err = dialErr
	}
	if err != nil {
		return nil, fmt.Errorf("dial %q: %w", host, err)
	}
	return nil, fmt.Errorf("%w: %s", ErrAddressBlocked, host)
}

func (dialer SafeDialer) lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{address}, nil
	}
	if dialer.Lookup != nil {
		return dialer.Lookup(ctx, host)
	}
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func (dialer SafeDialer) dial(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	if dialer.Dial != nil {
		return dialer.Dial(ctx, network, address)
	}
	var networkDialer net.Dialer
	return networkDialer.DialContext(ctx, network, address)
}

var blockedRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func AllowedAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() ||
		!address.IsGlobalUnicast() ||
		address.IsPrivate() ||
		address.IsLoopback() ||
		address.IsLinkLocalUnicast() ||
		address.IsMulticast() ||
		address.IsUnspecified() {
		return false
	}
	for _, blocked := range blockedRanges {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

func New() *http.Client {
	return newClient(SafeDialer{}, time.Minute)
}

// NewForAI returns a controlled client for a configured AI endpoint. Public
// endpoints use the normal SSRF-safe dialer. Ollama-style local endpoints are
// allowed only for an explicit host allowlist, so a redirect or DNS result
// cannot turn a local provider into an arbitrary internal request.
func NewForAI(baseURL string, timeout time.Duration) (*http.Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("AI Base URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("AI Base URL must not contain userinfo")
	}
	if timeout <= 0 {
		timeout = time.Minute
	}
	if isLocalAIHost(parsed.Hostname()) {
		return newClient(localAIDialer{}, timeout), nil
	}
	return newClient(SafeDialer{}, timeout), nil
}

func newClient(dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = dialer.DialContext

	return &http.Client{
		Transport: userAgentTransport{base: transport},
		Timeout:   timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
				return fmt.Errorf("redirect scheme %q is not allowed", request.URL.Scheme)
			}
			if request.URL.User != nil {
				return fmt.Errorf("redirect credentials are not allowed")
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

type localAIDialer struct{}

func (localAIDialer) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split network address %q: %w", address, err)
	}
	if !isLocalAIHost(host) {
		return nil, fmt.Errorf("%w: %s", ErrAddressBlocked, host)
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

func isLocalAIHost(host string) bool {
	switch strings.TrimSuffix(strings.ToLower(host), ".") {
	case "localhost", "127.0.0.1", "::1", "host.docker.internal", "host.containers.internal", "gateway.docker.internal", "ollama":
		return true
	default:
		return false
	}
}
