package feed

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

const defaultMaxBytes int64 = 4 << 20
const parserVersion = 2

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Driver struct {
	client HTTPClient
}

type checkpoint struct {
	ETag          string `json:"etag,omitempty"`
	LastModified  string `json:"last_modified,omitempty"`
	ParserVersion int    `json:"parser_version,omitempty"`
}

func New(client HTTPClient) *Driver {
	return &Driver{client: client}
}

func (driver *Driver) Kind() source.Kind {
	return source.KindRSS
}

func (driver *Driver) Validate(_ context.Context, spec source.Spec) (source.ValidatedSpec, error) {
	if spec.Kind != source.KindRSS {
		return source.ValidatedSpec{}, &source.ValidationError{
			Field: "kind", Message: "feed driver requires rss kind",
		}
	}
	return spec.Validate()
}

func (driver *Driver) Acquire(
	ctx context.Context,
	acquireRequest ingestion.AcquireRequest,
) (ingestion.AcquisitionBatch, error) {
	var previous checkpoint
	if len(acquireRequest.Checkpoint) > 0 {
		if err := json.Unmarshal(acquireRequest.Checkpoint, &previous); err != nil {
			return ingestion.AcquisitionBatch{}, fmt.Errorf("decode feed checkpoint: %w", err)
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, acquireRequest.Source.Locator, nil)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("create feed request: %w", err)
	}
	request.Header.Set("Accept", "application/atom+xml, application/rss+xml, application/feed+json, application/json;q=0.9, application/xml;q=0.8")
	if previous.ParserVersion >= parserVersion {
		if previous.ETag != "" {
			request.Header.Set("If-None-Match", previous.ETag)
		}
		if previous.LastModified != "" {
			request.Header.Set("If-Modified-Since", previous.LastModified)
		}
	}

	response, err := driver.client.Do(request)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("fetch feed: %w", err)
	}
	defer response.Body.Close()

	next := checkpoint{
		ETag:          response.Header.Get("ETag"),
		LastModified:  response.Header.Get("Last-Modified"),
		ParserVersion: parserVersion,
	}
	if next.ETag == "" {
		next.ETag = previous.ETag
	}
	if next.LastModified == "" {
		next.LastModified = previous.LastModified
	}
	nextCheckpoint, err := json.Marshal(next)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("encode feed checkpoint: %w", err)
	}

	if response.StatusCode == http.StatusNotModified {
		return ingestion.AcquisitionBatch{
			NextCheckpoint: nextCheckpoint,
			Diagnostics:    ingestion.Diagnostics{Status: "not_modified"},
		}, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("fetch feed: HTTP status %d", response.StatusCode)
	}

	maxBytes := acquireRequest.Limits.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read feed: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read feed: response exceeds %d bytes", maxBytes)
	}

	candidates, err := parse(body)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("parse feed: %w", err)
	}
	return ingestion.AcquisitionBatch{
		Candidates:     candidates,
		NextCheckpoint: nextCheckpoint,
		Diagnostics: ingestion.Diagnostics{
			Status:         "ok",
			CandidateCount: len(candidates),
		},
	}, nil
}

func parse(body []byte) ([]ingestion.Candidate, error) {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") {
		return parseJSONFeed(body)
	}

	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	switch strings.ToLower(root.XMLName.Local) {
	case "rss", "rdf":
		return parseRSS(body)
	case "feed":
		return parseAtom(body)
	default:
		return nil, fmt.Errorf("unsupported XML root %q", root.XMLName.Local)
	}
}

type rssDocument struct {
	Channel struct {
		Items []struct {
			GUID        string `xml:"guid"`
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Content     string `xml:"encoded"`
			Author      string `xml:"author"`
			PubDate     string `xml:"pubDate"`
			Enclosure   struct {
				URL  string `xml:"url,attr"`
				Type string `xml:"type,attr"`
			} `xml:"enclosure"`
			Thumbnail struct {
				URL string `xml:"url,attr"`
			} `xml:"thumbnail"`
			Media struct {
				URL    string `xml:"url,attr"`
				Type   string `xml:"type,attr"`
				Medium string `xml:"medium,attr"`
			} `xml:"content"`
		} `xml:"item"`
	} `xml:"channel"`
	Items []struct {
		GUID        string `xml:"guid"`
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		Content     string `xml:"encoded"`
		Author      string `xml:"author"`
		PubDate     string `xml:"date"`
		Enclosure   struct {
			URL  string `xml:"url,attr"`
			Type string `xml:"type,attr"`
		} `xml:"enclosure"`
		Thumbnail struct {
			URL string `xml:"url,attr"`
		} `xml:"thumbnail"`
		Media struct {
			URL    string `xml:"url,attr"`
			Type   string `xml:"type,attr"`
			Medium string `xml:"medium,attr"`
		} `xml:"content"`
	} `xml:"item"`
}

func parseRSS(body []byte) ([]ingestion.Candidate, error) {
	var document rssDocument
	if err := xml.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	items := document.Channel.Items
	if len(items) == 0 && len(document.Items) > 0 {
		result := make([]ingestion.Candidate, 0, len(document.Items))
		for _, item := range document.Items {
			result = append(result, ingestion.Candidate{
				ExternalID:  strings.TrimSpace(item.GUID),
				URL:         strings.TrimSpace(item.Link),
				Title:       strings.TrimSpace(item.Title),
				Author:      strings.TrimSpace(item.Author),
				Summary:     item.Description,
				ContentHTML: richRSSContent(item.Description, item.Content, item.Thumbnail.URL, mediaImage(item.Media.URL, item.Media.Type, item.Media.Medium), imageEnclosure(item.Enclosure.URL, item.Enclosure.Type)),
				PublishedAt: parseTime(item.PubDate),
			})
		}
		return result, nil
	}
	result := make([]ingestion.Candidate, 0, len(items))
	for _, item := range items {
		result = append(result, ingestion.Candidate{
			ExternalID:  strings.TrimSpace(item.GUID),
			URL:         strings.TrimSpace(item.Link),
			Title:       strings.TrimSpace(item.Title),
			Author:      strings.TrimSpace(item.Author),
			Summary:     item.Description,
			ContentHTML: richRSSContent(item.Description, item.Content, item.Thumbnail.URL, mediaImage(item.Media.URL, item.Media.Type, item.Media.Medium), imageEnclosure(item.Enclosure.URL, item.Enclosure.Type)),
			PublishedAt: parseTime(item.PubDate),
		})
	}
	return result, nil
}

func richRSSContent(description string, encoded string, imageURLs ...string) string {
	content := strings.TrimSpace(encoded)
	if content == "" {
		content = strings.TrimSpace(description)
	}
	if strings.Contains(strings.ToLower(content), "<img") {
		return content
	}
	for _, rawURL := range imageURLs {
		if imageURL := strings.TrimSpace(rawURL); imageURL != "" {
			return `<figure><img src="` + html.EscapeString(imageURL) + `" alt="" loading="lazy"></figure>` + content
		}
	}
	return content
}

func imageEnclosure(url string, mediaType string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "image/") {
		return url
	}
	return ""
}

func mediaImage(url string, mediaType string, medium string) string {
	if strings.EqualFold(strings.TrimSpace(medium), "image") {
		return url
	}
	return imageEnclosure(url, mediaType)
}

type atomDocument struct {
	Entries []struct {
		ID        string `xml:"id"`
		Title     string `xml:"title"`
		Summary   string `xml:"summary"`
		Content   string `xml:"content"`
		Updated   string `xml:"updated"`
		Published string `xml:"published"`
		Links     []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
		Author struct {
			Name string `xml:"name"`
		} `xml:"author"`
	} `xml:"entry"`
}

func parseAtom(body []byte) ([]ingestion.Candidate, error) {
	var document atomDocument
	if err := xml.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	result := make([]ingestion.Candidate, 0, len(document.Entries))
	for _, item := range document.Entries {
		link := ""
		for _, candidate := range item.Links {
			if candidate.Rel == "" || candidate.Rel == "alternate" {
				link = candidate.Href
				break
			}
		}
		content := item.Content
		if content == "" {
			content = item.Summary
		}
		published := item.Published
		if published == "" {
			published = item.Updated
		}
		result = append(result, ingestion.Candidate{
			ExternalID:  strings.TrimSpace(item.ID),
			URL:         strings.TrimSpace(link),
			Title:       strings.TrimSpace(item.Title),
			Author:      strings.TrimSpace(item.Author.Name),
			Summary:     item.Summary,
			ContentHTML: content,
			PublishedAt: parseTime(published),
		})
	}
	return result, nil
}

type jsonFeedDocument struct {
	Items []struct {
		ID            string `json:"id"`
		URL           string `json:"url"`
		Title         string `json:"title"`
		Summary       string `json:"summary"`
		ContentHTML   string `json:"content_html"`
		ContentText   string `json:"content_text"`
		DatePublished string `json:"date_published"`
		Authors       []struct {
			Name string `json:"name"`
		} `json:"authors"`
	} `json:"items"`
}

func parseJSONFeed(body []byte) ([]ingestion.Candidate, error) {
	var document jsonFeedDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	result := make([]ingestion.Candidate, 0, len(document.Items))
	for _, item := range document.Items {
		content := item.ContentHTML
		if content == "" {
			content = item.ContentText
		}
		author := ""
		if len(item.Authors) > 0 {
			author = item.Authors[0].Name
		}
		result = append(result, ingestion.Candidate{
			ExternalID:  strings.TrimSpace(item.ID),
			URL:         strings.TrimSpace(item.URL),
			Title:       strings.TrimSpace(item.Title),
			Author:      strings.TrimSpace(author),
			Summary:     item.Summary,
			ContentHTML: content,
			PublishedAt: parseTime(item.DatePublished),
		})
	}
	return result, nil
}

func parseTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC3339, time.RFC1123Z, time.RFC1123} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return &parsed
		}
	}
	return nil
}
