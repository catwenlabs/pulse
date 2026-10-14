package httpserver

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// JSON list payloads (Stories, Entries, Digests) carry article HTML and grow
// into megabytes at scale. Browsers transparently decompress gzip responses,
// so compressing API and web responses cuts transfer size by roughly an order
// of magnitude without touching any handler.
//
// Exclusions:
//   - text/event-stream responses rely on unbuffered flushing and must pass
//     through untouched;
//   - already-compressed media (images, video, audio, archives) would grow;
//   - bodyless statuses (204, 304, 1xx) must not receive a gzip trailer;
//   - Range requests interact with byte offsets and are served plain.

var gzipWriterPool = sync.Pool{
	New: func() any {
		return gzip.NewWriter(nil)
	},
}

func acceptsGzip(header http.Header) bool {
	for _, value := range header.Values("Accept-Encoding") {
		for _, part := range strings.Split(value, ",") {
			name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
			if name != "gzip" {
				continue
			}
			// An explicit q=0 marks gzip as not acceptable for this request.
			quality := 1.0
			if params = strings.TrimSpace(params); strings.HasPrefix(params, "q=") {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(params, "q=")), 64); err == nil {
					quality = parsed
				}
			}
			if quality > 0 {
				return true
			}
		}
	}
	return false
}

func gzipExcluded(contentType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch {
	case normalized == "text/event-stream":
		return true
	case strings.HasPrefix(normalized, "image/"),
		strings.HasPrefix(normalized, "video/"),
		strings.HasPrefix(normalized, "audio/"),
		strings.HasPrefix(normalized, "multipart/"):
		return true
	case normalized == "application/zip",
		normalized == "application/gzip",
		normalized == "application/x-gzip",
		normalized == "application/octet-stream":
		return true
	}
	return false
}

func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead ||
			request.Header.Get("Range") != "" ||
			!acceptsGzip(request.Header) {
			next.ServeHTTP(w, request)
			return
		}
		writer := &gzipResponseWriter{ResponseWriter: w}
		defer writer.close()
		next.ServeHTTP(writer, request)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	wroteHeader bool
	compressing bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.wroteHeader = true
	if !gzipBodylessStatus(status) {
		w.decideForContentType(w.Header().Get("Content-Type"))
		if w.compressing {
			header := w.Header()
			header.Del("Content-Length")
			header.Set("Content-Encoding", "gzip")
			header.Add("Vary", "Accept-Encoding")
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		// net/http would sniff the same way before committing the response;
		// materializing it in the header keeps the compression decision
		// consistent for handlers that never set a content type.
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(body))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.compressing {
		return w.writer.Write(body)
	}
	return w.ResponseWriter.Write(body)
}

// WriteString shadows any StringWriter promoted from the wrapped writer;
// io.WriteString would otherwise bypass Write and reach the client raw.
func (w *gzipResponseWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

// Flush keeps SSE handlers working behind the wrapper: compressed responses
// flush the gzip stream first so buffered bytes still reach the client.
func (w *gzipResponseWriter) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	if w.compressing && w.wroteHeader {
		_ = w.writer.Flush()
	}
	flusher.Flush()
}

func gzipBodylessStatus(status int) bool {
	return (status >= 100 && status < 200) ||
		status == http.StatusNoContent ||
		status == http.StatusNotModified
}

func (w *gzipResponseWriter) decideForContentType(contentType string) {
	w.compressing = contentType != "" && !gzipExcluded(contentType)
	if !w.compressing {
		return
	}
	w.writer = gzipWriterPool.Get().(*gzip.Writer)
	w.writer.Reset(w.ResponseWriter)
}

// release returns the pooled writer without emitting a gzip trailer, used
// when no compressed bytes may reach the wire.
func (w *gzipResponseWriter) release() {
	if w.writer == nil {
		return
	}
	w.writer.Reset(io.Discard)
	gzipWriterPool.Put(w.writer)
	w.writer = nil
	w.compressing = false
}

func (w *gzipResponseWriter) close() {
	if !w.wroteHeader || !w.compressing {
		w.release()
		return
	}
	// Close flushes the gzip trailer; only then is the response complete.
	_ = w.writer.Close()
	w.release()
}
