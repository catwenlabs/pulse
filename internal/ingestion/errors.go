package ingestion

import "errors"

// ErrFetch indicates a transient failure retrieving a Source's content (network,
// TLS, DNS, unexpected HTTP status, or size limit). The HTTP layer maps it to 502
// so the user sees the real cause instead of a generic internal error.
var ErrFetch = errors.New("source fetch failed")

// ErrParse indicates that retrieved content could not be parsed as a feed. The HTTP
// layer maps it to 422.
var ErrParse = errors.New("source parse failed")
