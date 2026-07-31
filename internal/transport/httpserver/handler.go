package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/catwenlabs/pulse/internal/annotation"
	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/opml"
	"github.com/catwenlabs/pulse/internal/organization"
	"github.com/catwenlabs/pulse/internal/preview"
	"github.com/catwenlabs/pulse/internal/rule"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/story"
)

type Backend interface {
	CreateSource(context.Context, source.Spec) (source.Source, error)
	ListSources(context.Context) ([]source.Source, error)
	GetSource(context.Context, source.ID) (source.Source, error)
	UpdateSource(context.Context, source.ID, string, string) (source.Source, error)
	SetSourceEnabled(context.Context, source.ID, bool) (source.Source, error)
	ArchiveSource(context.Context, source.ID) error
	SetSourceSecret(context.Context, source.ID, string) error
	GetSourceHealth(context.Context, source.ID) (source.Health, error)
	CreateFolder(context.Context, string) (organization.Folder, error)
	ListFolders(context.Context) ([]organization.Folder, error)
	DeleteFolder(context.Context, string) error
	AddSourceToFolder(context.Context, string, source.ID) error
	RemoveSourceFromFolder(context.Context, string, source.ID) error
	CreateView(context.Context, organization.View) (organization.View, error)
	UpdateView(context.Context, organization.View) (organization.View, error)
	ListViews(context.Context) ([]organization.View, error)
	DeleteView(context.Context, string) error
	Enqueue(context.Context, ingestion.EnqueueRequest) (ingestion.Acquisition, error)
	ListEntries(context.Context, int) ([]entry.Entry, error)
	SearchEntries(context.Context, entry.Query) ([]entry.Entry, error)
	GetEntry(context.Context, entry.ID) (entry.Entry, error)
	UpdateEntry(context.Context, entry.ID, entry.Patch) (entry.Entry, error)
	MarkEntriesRead(context.Context, source.ID) (int64, error)
	AddEntryTag(context.Context, entry.ID, string) (entry.Tag, error)
	RemoveEntryTag(context.Context, entry.ID, string) error
	ListStories(context.Context, story.Query) ([]story.Story, error)
	GetStory(context.Context, story.ID) (story.Story, error)
	UpdateStory(context.Context, story.ID, story.Patch) (story.Story, error)
	MarkStoriesRead(context.Context, string) (int64, error)
	MergeStories(context.Context, story.ID, story.ID) (story.Story, error)
	SplitStory(context.Context, story.ID, entry.ID) (story.Story, error)
	Recluster(context.Context) (int, error)
	ImportOPML(context.Context, []opml.Subscription) (opml.ImportResult, error)
	ExportOPML(context.Context) ([]opml.Subscription, error)
	PreviewSource(context.Context, source.Spec) (preview.Result, error)
	CreateRule(context.Context, rule.Rule) (rule.Rule, error)
	ListRules(context.Context) ([]rule.Rule, error)
	GetRule(context.Context, string) (rule.Rule, error)
	UpdateRule(context.Context, rule.Rule) (rule.Rule, error)
	DeleteRule(context.Context, string) error
	PreviewRule(context.Context, string) (rule.PreviewResult, error)
	ReplayRule(context.Context, string, bool) (rule.ReplayResult, error)
}

func NewHandler(backends ...Backend) http.Handler {
	var backend Backend
	if len(backends) > 0 {
		backend = backends[0]
	}
	return newHandler(backend, nil)
}

func NewHandlerWithWeb(backend Backend, web fs.FS) http.Handler {
	return newHandler(backend, web)
}

func newHandler(backend Backend, web fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	if backend == nil {
		registerWeb(mux, web)
		return mux
	}
	mux.HandleFunc("POST /api/v1/sources", createSource(backend))
	mux.HandleFunc("POST /api/v1/sources/preview", previewSource(backend))
	mux.HandleFunc("GET /api/v1/sources", listSources(backend))
	mux.HandleFunc("GET /api/v1/sources/{id}", getSource(backend))
	mux.HandleFunc("PATCH /api/v1/sources/{id}", updateSource(backend))
	mux.HandleFunc("DELETE /api/v1/sources/{id}", archiveSource(backend))
	mux.HandleFunc("POST /api/v1/sources/{id}/runs", runSource(backend))
	mux.HandleFunc("POST /api/v1/sources/{id}/entries", createManualEntry(backend))
	mux.HandleFunc("POST /api/v1/sources/{id}/annotations", importAnnotations(backend))
	mux.HandleFunc("POST /api/v1/sources/{id}/secret", rotateSourceSecret(backend))
	mux.HandleFunc("GET /api/v1/sources/{id}/health", getSourceHealth(backend))
	mux.HandleFunc("POST /api/v1/webhooks/{id}", receiveWebhook(backend))
	mux.HandleFunc("GET /api/v1/entries", listEntries(backend))
	mux.HandleFunc("PATCH /api/v1/entries", markEntriesRead(backend))
	mux.HandleFunc("GET /api/v1/entries/{id}", getEntry(backend))
	mux.HandleFunc("PATCH /api/v1/entries/{id}", updateEntry(backend))
	mux.HandleFunc("POST /api/v1/entries/{id}/tags", addEntryTag(backend))
	mux.HandleFunc("DELETE /api/v1/entries/{id}/tags/{tagID}", removeEntryTag(backend))
	mux.HandleFunc("GET /api/v1/entries/{id}/export.md", exportEntryMarkdown(backend))
	mux.HandleFunc("GET /api/v1/stories", listStories(backend))
	mux.HandleFunc("PATCH /api/v1/stories", markStoriesRead(backend))
	mux.HandleFunc("GET /api/v1/stories/{id}", getStory(backend))
	mux.HandleFunc("PATCH /api/v1/stories/{id}", updateStory(backend))
	mux.HandleFunc("POST /api/v1/stories/{id}/merge", mergeStory(backend))
	mux.HandleFunc("POST /api/v1/stories/{id}/split", splitStory(backend))
	mux.HandleFunc("POST /api/v1/stories/recluster", reclusterStories(backend))
	mux.HandleFunc("POST /api/v1/opml/import", importOPML(backend))
	mux.HandleFunc("GET /api/v1/opml/export", exportOPML(backend))
	mux.HandleFunc("GET /api/v1/folders", listFolders(backend))
	mux.HandleFunc("POST /api/v1/folders", createFolder(backend))
	mux.HandleFunc("DELETE /api/v1/folders/{id}", deleteFolder(backend))
	mux.HandleFunc("PUT /api/v1/folders/{id}/sources/{sourceID}", addSourceToFolder(backend))
	mux.HandleFunc("DELETE /api/v1/folders/{id}/sources/{sourceID}", removeSourceFromFolder(backend))
	mux.HandleFunc("GET /api/v1/views", listViews(backend))
	mux.HandleFunc("POST /api/v1/views", createView(backend))
	mux.HandleFunc("PUT /api/v1/views/{id}", updateView(backend))
	mux.HandleFunc("DELETE /api/v1/views/{id}", deleteView(backend))
	mux.HandleFunc("GET /api/v1/rules", listRules(backend))
	mux.HandleFunc("POST /api/v1/rules", createRule(backend))
	mux.HandleFunc("GET /api/v1/rules/{id}", getRule(backend))
	mux.HandleFunc("PUT /api/v1/rules/{id}", updateRule(backend))
	mux.HandleFunc("DELETE /api/v1/rules/{id}", deleteRule(backend))
	mux.HandleFunc("POST /api/v1/rules/{id}/preview", previewRule(backend))
	mux.HandleFunc("POST /api/v1/rules/{id}/replay", replayRule(backend))
	mux.HandleFunc("GET /api/v1/export/config", exportConfig(backend))
	registerWeb(mux, web)
	return mux
}

func listStories(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		limit, offset, ok := pagination(w, request)
		if !ok {
			return
		}
		items, err := backend.ListStories(request.Context(), story.Query{
			Limit:    limit,
			Offset:   offset,
			Search:   request.URL.Query().Get("q"),
			State:    request.URL.Query().Get("state"),
			Tag:      request.URL.Query().Get("tag"),
			SourceID: request.URL.Query().Get("source_id"),
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilSlice(items))
	}
}

func getStory(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		item, err := backend.GetStory(request.Context(), story.ID(request.PathValue("id")))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func updateStory(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			Read    *bool `json:"read"`
			Starred *bool `json:"starred"`
			Hidden  *bool `json:"hidden"`
			Later   *bool `json:"later"`
		}
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		updated, err := backend.UpdateStory(
			request.Context(),
			story.ID(request.PathValue("id")),
			story.Patch{
				Read: body.Read, Starred: body.Starred,
				Hidden: body.Hidden, Later: body.Later,
			},
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func mergeStory(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			Into string `json:"into"`
		}
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		if body.Into == "" {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "into is required", "into")
			return
		}
		merged, err := backend.MergeStories(
			request.Context(),
			story.ID(request.PathValue("id")),
			story.ID(body.Into),
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, merged)
	}
}

func splitStory(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			EntryID string `json:"entry_id"`
		}
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		if body.EntryID == "" {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "entry_id is required", "entry_id")
			return
		}
		split, err := backend.SplitStory(
			request.Context(),
			story.ID(request.PathValue("id")),
			entry.ID(body.EntryID),
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, split)
	}
}

func reclusterStories(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		processed, err := backend.Recluster(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"processed": processed})
	}
}

func markStoriesRead(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			Read bool `json:"read"`
		}
		if err := decodeJSONBody(w, request, &body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		if !body.Read {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "read must be true", "read")
			return
		}
		count, err := backend.MarkStoriesRead(
			request.Context(),
			request.URL.Query().Get("source_id"),
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"updated_count": count})
	}
}

func pagination(w http.ResponseWriter, request *http.Request) (int, int, bool) {
	limit := 50
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 200 {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 200", "limit")
			return 0, 0, false
		}
		limit = parsed
	}
	offset := 0
	if value := request.URL.Query().Get("offset"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "offset must be zero or greater", "offset")
			return 0, 0, false
		}
		offset = parsed
	}
	return limit, offset, true
}

func exportConfig(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		sources, err := backend.ListSources(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		rules, err := backend.ListRules(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		views, err := backend.ListViews(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="pulse-config.json"`)
		writeJSON(w, http.StatusOK, map[string]any{
			"version": 1,
			"sources": nonNilSlice(sources),
			"rules":   nonNilSlice(rules),
			"views":   nonNilSlice(views),
		})
	}
}

func exportEntryMarkdown(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		item, err := backend.GetEntry(request.Context(), entry.ID(request.PathValue("id")))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		title := item.DisplayTitle
		if title == "" {
			title = item.SourceTitle
		}
		if title == "" {
			title = "Untitled"
		}
		body := item.ContentHTML
		if body == "" {
			body = item.Summary
		}
		var metadata strings.Builder
		metadata.WriteString("# ")
		metadata.WriteString(title)
		metadata.WriteString("\n\n")
		if item.Author != "" {
			metadata.WriteString("Author: ")
			metadata.WriteString(item.Author)
			metadata.WriteString("\n\n")
		}
		if item.CanonicalURL != "" {
			metadata.WriteString("Source: ")
			metadata.WriteString(item.CanonicalURL)
			metadata.WriteString("\n\n")
		}
		metadata.WriteString(htmlText(body))
		metadata.WriteString("\n")
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set(
			"Content-Disposition",
			fmt.Sprintf(`attachment; filename="%s.md"`, item.ID),
		)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, metadata.String())
	}
}

func htmlText(value string) string {
	var result strings.Builder
	inTag := false
	space := false
	for _, character := range value {
		switch character {
		case '<':
			inTag = true
			space = true
		case '>':
			inTag = false
		default:
			if inTag {
				continue
			}
			if unicode.IsSpace(character) {
				space = true
				continue
			}
			if space && result.Len() > 0 {
				result.WriteByte(' ')
			}
			space = false
			result.WriteRune(character)
		}
	}
	return strings.TrimSpace(result.String())
}

func listRules(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		items, err := backend.ListRules(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilSlice(items))
	}
}

func createRule(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var definition rule.Rule
		if err := decodeJSONBody(w, request, &definition); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		created, err := backend.CreateRule(request.Context(), definition)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func getRule(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		item, err := backend.GetRule(request.Context(), request.PathValue("id"))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func updateRule(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var definition rule.Rule
		if err := decodeJSONBody(w, request, &definition); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		definition.ID = request.PathValue("id")
		updated, err := backend.UpdateRule(request.Context(), definition)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func deleteRule(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := backend.DeleteRule(request.Context(), request.PathValue("id")); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func previewRule(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		result, err := backend.PreviewRule(request.Context(), request.PathValue("id"))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func replayRule(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			EffectsEnabled bool `json:"effects_enabled"`
		}
		if request.ContentLength != 0 {
			if err := decodeJSONBody(w, request, &body); err != nil {
				writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
				return
			}
		}
		result, err := backend.ReplayRule(
			request.Context(), request.PathValue("id"), body.EffectsEnabled,
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func decodeJSONBody(w http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(w, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func listFolders(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		items, err := backend.ListFolders(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilSlice(items))
	}
}

func createFolder(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		item, err := backend.CreateFolder(request.Context(), body.Name)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func deleteFolder(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := backend.DeleteFolder(request.Context(), request.PathValue("id")); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func addSourceToFolder(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := backend.AddSourceToFolder(request.Context(), request.PathValue("id"), source.ID(request.PathValue("sourceID"))); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func removeSourceFromFolder(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := backend.RemoveSourceFromFolder(request.Context(), request.PathValue("id"), source.ID(request.PathValue("sourceID"))); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func listViews(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		items, err := backend.ListViews(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilSlice(items))
	}
}

func createView(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var input organization.View
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		item, err := backend.CreateView(request.Context(), input)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func updateView(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var input organization.View
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		input.ID = request.PathValue("id")
		item, err := backend.UpdateView(request.Context(), input)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func deleteView(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := backend.DeleteView(request.Context(), request.PathValue("id")); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getSourceHealth(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		health, err := backend.GetSourceHealth(
			request.Context(), source.ID(request.PathValue("id")),
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, health)
	}
}

func receiveWebhook(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		src, err := backend.GetSource(request.Context(), source.ID(request.PathValue("id")))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		if src.Kind != source.KindWebhook {
			writeProblem(w, http.StatusNotFound, "webhook_not_found", "webhook source not found", "")
			return
		}
		if !src.Enabled {
			writeProblem(w, http.StatusConflict, "source_paused", "source is paused", "")
			return
		}
		provided := request.Header.Get("X-Pulse-Webhook-Secret")
		if !webhookSecretMatches(provided, src.SecretRef) {
			writeProblem(w, http.StatusUnauthorized, "invalid_webhook_secret", "invalid webhook secret", "")
			return
		}
		payload, err := readJSONPayload(w, request)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		key := request.Header.Get("Idempotency-Key")
		if strings.TrimSpace(key) == "" {
			digest := sha256.Sum256(payload)
			key = hex.EncodeToString(digest[:])
		}
		acquisition, err := backend.Enqueue(request.Context(), ingestion.EnqueueRequest{
			SourceID: src.ID, Trigger: ingestion.TriggerWebhook, Payload: payload,
			IdempotencyKey: key, Priority: 100,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, acquisition)
	}
}

func createManualEntry(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		src, err := backend.GetSource(request.Context(), source.ID(request.PathValue("id")))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		if src.Kind != source.KindManual {
			writeProblem(w, http.StatusUnprocessableEntity, "wrong_source_kind", "source is not manual", "")
			return
		}
		if !src.Enabled {
			writeProblem(w, http.StatusConflict, "source_paused", "source is paused", "")
			return
		}
		payload, err := readJSONPayload(w, request)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		if err := validateManualEntryURL(payload); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "url")
			return
		}
		key := request.Header.Get("Idempotency-Key")
		if strings.TrimSpace(key) == "" {
			digest := sha256.Sum256(payload)
			key = hex.EncodeToString(digest[:])
		}
		acquisition, err := backend.Enqueue(request.Context(), ingestion.EnqueueRequest{
			SourceID: src.ID, Trigger: ingestion.TriggerManual, Payload: payload,
			IdempotencyKey: key, Priority: 100,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, acquisition)
	}
}

func validateManualEntryURL(payload []byte) error {
	var value struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return fmt.Errorf("decode manual entry: %w", err)
	}
	if strings.TrimSpace(value.URL) == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(value.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("url must be an HTTP or HTTPS page without embedded credentials")
	}
	return nil
}

func importAnnotations(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		src, err := backend.GetSource(request.Context(), source.ID(request.PathValue("id")))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		if src.Kind != source.KindAnnotations {
			writeProblem(w, http.StatusUnprocessableEntity, "wrong_source_kind", "source is not annotations", "")
			return
		}
		if !src.Enabled {
			writeProblem(w, http.StatusConflict, "source_paused", "source is paused", "")
			return
		}
		payload, err := readJSONPayload(w, request)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		if _, err := annotation.DecodeBatch(payload); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "annotations")
			return
		}
		key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
		if key == "" {
			digest := sha256.Sum256(payload)
			key = hex.EncodeToString(digest[:])
		}
		acquisition, err := backend.Enqueue(request.Context(), ingestion.EnqueueRequest{
			SourceID: src.ID, Trigger: ingestion.TriggerImport, Payload: payload,
			IdempotencyKey: key, Priority: 100,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, acquisition)
	}
}

func rotateSourceSecret(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		id := source.ID(request.PathValue("id"))
		if _, err := backend.GetSource(request.Context(), id); err != nil {
			writeDomainError(w, err)
			return
		}
		secret, err := randomKey()
		if err != nil {
			writeDomainError(w, err)
			return
		}
		digest := sha256.Sum256([]byte(secret))
		stored := "sha256:" + hex.EncodeToString(digest[:])
		if err := backend.SetSourceSecret(request.Context(), id, stored); err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"secret": secret})
	}
}

func webhookSecretMatches(provided, stored string) bool {
	if provided == "" || stored == "" {
		return false
	}
	if strings.HasPrefix(stored, "sha256:") {
		digest := sha256.Sum256([]byte(provided))
		actual := "sha256:" + hex.EncodeToString(digest[:])
		return subtle.ConstantTimeCompare([]byte(actual), []byte(stored)) == 1
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(stored)) == 1
}

func readJSONPayload(w http.ResponseWriter, request *http.Request) (json.RawMessage, error) {
	request.Body = http.MaxBytesReader(w, request.Body, 1<<20)
	payload, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("read JSON payload: %w", err)
	}
	if len(payload) == 0 || !json.Valid(payload) {
		return nil, fmt.Errorf("body must be valid JSON")
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return nil, fmt.Errorf("body must be a JSON object")
	}
	return json.RawMessage(payload), nil
}

func previewSource(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(w, request.Body, 1<<20)
		var body struct {
			Name      string          `json:"name"`
			Kind      source.Kind     `json:"kind"`
			Locator   string          `json:"locator"`
			Config    json.RawMessage `json:"config"`
			SecretRef string          `json:"secret_ref"`
		}
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		result, err := backend.PreviewSource(request.Context(), source.Spec{
			Name: body.Name, Kind: body.Kind, Locator: body.Locator,
			Config: body.Config, SecretRef: body.SecretRef,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func updateSource(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(w, request.Body, 1<<20)
		var body struct {
			Enabled *bool   `json:"enabled"`
			Name    *string `json:"name"`
			Locator *string `json:"locator"`
		}
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		id := source.ID(request.PathValue("id"))
		var (
			updated source.Source
			err     error
		)
		switch {
		case body.Name != nil && body.Locator != nil && body.Enabled == nil:
			updated, err = backend.UpdateSource(request.Context(), id, *body.Name, *body.Locator)
		case body.Enabled != nil && body.Name == nil && body.Locator == nil:
			updated, err = backend.SetSourceEnabled(request.Context(), id, *body.Enabled)
		default:
			writeProblem(w, http.StatusBadRequest, "invalid_request", "provide enabled, or both name and locator", "")
			return
		}
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func archiveSource(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := backend.ArchiveSource(
			request.Context(),
			source.ID(request.PathValue("id")),
		); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func registerWeb(mux *http.ServeMux, web fs.FS) {
	if web == nil {
		return
	}
	files := http.FileServerFS(web)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			http.NotFound(w, request)
			return
		}
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if name == "." {
			name = "index.html"
		}
		if info, err := fs.Stat(web, name); err == nil && !info.IsDir() {
			files.ServeHTTP(w, request)
			return
		}
		index, err := fs.ReadFile(web, "index.html")
		if err != nil {
			http.NotFound(w, request)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}

func importOPML(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(w, request.Body, 8<<20)
		subscriptions, err := opml.Import(request.Body)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_opml", err.Error(), "")
			return
		}
		result, err := backend.ImportOPML(request.Context(), subscriptions)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func exportOPML(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		subscriptions, err := backend.ExportOPML(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		data, err := opml.Export("Pulse subscriptions", subscriptions)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/x-opml; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="pulse-subscriptions.opml"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

func createSource(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(w, request.Body, 1<<20)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()

		var body struct {
			Name      string          `json:"name"`
			Kind      source.Kind     `json:"kind"`
			Locator   string          `json:"locator"`
			Config    json.RawMessage `json:"config"`
			SecretRef string          `json:"secret_ref"`
		}
		if err := decoder.Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		created, err := backend.CreateSource(request.Context(), source.Spec{
			Name:      body.Name,
			Kind:      body.Kind,
			Locator:   body.Locator,
			Config:    body.Config,
			SecretRef: body.SecretRef,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func listSources(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		sources, err := backend.ListSources(request.Context())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilSlice(sources))
	}
}

func getSource(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		got, err := backend.GetSource(request.Context(), source.ID(request.PathValue("id")))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, got)
	}
}

func runSource(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		sourceID := source.ID(request.PathValue("id"))
		if _, err := backend.GetSource(request.Context(), sourceID); err != nil {
			writeDomainError(w, err)
			return
		}
		idempotencyKey := request.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			var err error
			idempotencyKey, err = randomKey()
			if err != nil {
				writeDomainError(w, err)
				return
			}
		}
		acquisition, err := backend.Enqueue(request.Context(), ingestion.EnqueueRequest{
			SourceID:       sourceID,
			Trigger:        ingestion.TriggerManual,
			IdempotencyKey: idempotencyKey,
			Priority:       100,
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, acquisition)
	}
}

func listEntries(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		limit := 50
		if value := request.URL.Query().Get("limit"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 || parsed > 200 {
				writeProblem(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 200", "limit")
				return
			}
			limit = parsed
		}
		offset := 0
		if value := request.URL.Query().Get("offset"); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				writeProblem(w, http.StatusBadRequest, "invalid_request", "offset must be zero or greater", "offset")
				return
			}
			offset = parsed
		}
		entries, err := backend.SearchEntries(request.Context(), entry.Query{
			Limit:    limit,
			Offset:   offset,
			Search:   request.URL.Query().Get("q"),
			State:    request.URL.Query().Get("state"),
			Tag:      request.URL.Query().Get("tag"),
			SourceID: source.ID(request.URL.Query().Get("source_id")),
		})
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nonNilSlice(entries))
	}
}

func getEntry(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		item, err := backend.GetEntry(request.Context(), entry.ID(request.PathValue("id")))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}

func updateEntry(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(w, request.Body, 1<<20)
		var body struct {
			Read         *bool   `json:"read"`
			Starred      *bool   `json:"starred"`
			Hidden       *bool   `json:"hidden"`
			Later        *bool   `json:"later"`
			DisplayTitle *string `json:"display_title"`
			Note         *string `json:"note"`
		}
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		updated, err := backend.UpdateEntry(
			request.Context(),
			entry.ID(request.PathValue("id")),
			entry.Patch{
				Read: body.Read, Starred: body.Starred, Hidden: body.Hidden, Later: body.Later,
				DisplayTitle: body.DisplayTitle, Note: body.Note,
			},
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func markEntriesRead(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(w, request.Body, 1<<10)
		var body struct {
			Read bool `json:"read"`
		}
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		if !body.Read {
			writeProblem(w, http.StatusBadRequest, "invalid_request", "read must be true", "read")
			return
		}
		updated, err := backend.MarkEntriesRead(
			request.Context(),
			source.ID(strings.TrimSpace(request.URL.Query().Get("source_id"))),
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"updated_count": updated})
	}
}

func addEntryTag(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "")
			return
		}
		tag, err := backend.AddEntryTag(
			request.Context(), entry.ID(request.PathValue("id")), body.Name,
		)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, tag)
	}
}

func removeEntryTag(backend Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if err := backend.RemoveEntryTag(
			request.Context(),
			entry.ID(request.PathValue("id")),
			request.PathValue("tagID"),
		); err != nil {
			writeDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeDomainError(w http.ResponseWriter, err error) {
	var validationErr *source.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeProblem(w, http.StatusUnprocessableEntity, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, source.ErrDuplicate):
		writeProblem(w, http.StatusConflict, "source_exists", err.Error(), "")
	case errors.Is(err, source.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "source_not_found", err.Error(), "")
	case errors.Is(err, entry.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "entry_not_found", err.Error(), "")
	case errors.Is(err, story.ErrSelfMerge):
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error(), "into")
	case errors.Is(err, story.ErrReclusterUnavailable):
		writeProblem(w, http.StatusServiceUnavailable, "recluster_unavailable", err.Error(), "")
	case errors.Is(err, ingestion.ErrFetch):
		writeProblem(w, http.StatusBadGateway, "source_fetch_failed", err.Error(), "")
	case errors.Is(err, ingestion.ErrParse):
		writeProblem(w, http.StatusUnprocessableEntity, "source_parse_failed", err.Error(), "")
	default:
		slog.Error("unhandled domain error", "error", err)
		writeProblem(w, http.StatusInternalServerError, "internal_error", "internal server error", "")
	}
}

func writeProblem(w http.ResponseWriter, status int, code, detail, field string) {
	writeJSON(w, status, map[string]string{
		"code":   code,
		"detail": detail,
		"field":  field,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func nonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func randomKey() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate idempotency key: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
