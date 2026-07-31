package httpclient

import (
	"net/http"
)

// defaultUserAgent identifies Pulse to feed and content servers. Some WAFs reject
// Go's default "Go-http-client/1.1" with HTTP 403; the "compatible" token keeps it
// honest while matching the Mozilla-prefixed pattern those gateways accept.
const defaultUserAgent = "Mozilla/5.0 (compatible; Pulse/1.0; +https://github.com/catwenlabs/pulse)"

// userAgentTransport sets a default User-Agent on outgoing requests that do not
// already specify one. Drivers that set their own User-Agent (e.g. the push
// snapshot fetcher) are left untouched.
type userAgentTransport struct {
	base http.RoundTripper
}

func (transport userAgentTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if _, ok := request.Header["User-Agent"]; !ok {
		request.Header = request.Header.Clone()
		request.Header.Set("User-Agent", defaultUserAgent)
	}
	return transport.base.RoundTrip(request)
}
