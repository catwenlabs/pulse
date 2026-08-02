package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/catwenlabs/pulse/internal/events"
)

const (
	libraryChangeEventName = "library-change"
	defaultHeartbeat       = 15 * time.Second
)

func streamLibraryChanges(hub *events.LibraryChangeHub) http.HandlerFunc {
	return streamLibraryChangesWithHeartbeat(hub, defaultHeartbeat)
}

func streamLibraryChangesWithHeartbeat(hub *events.LibraryChangeHub, heartbeat time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming is not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("X-Accel-Buffering", "no")

		stream := hub.Subscribe(request.Context())
		_, _ = io.WriteString(w, "retry: 3000\n\n")
		flusher.Flush()

		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-request.Context().Done():
				return
			case change, ok := <-stream:
				if !ok {
					return
				}
				if err := writeLibraryChange(w, change); err != nil {
					return
				}
				flusher.Flush()
			case <-ticker.C:
				if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func writeLibraryChange(w io.Writer, change events.LibraryChange) error {
	payload, err := json.Marshal(change)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		w,
		"event: %s\nid: %d\ndata: %s\n\n",
		libraryChangeEventName,
		change.ID,
		payload,
	)
	return err
}
