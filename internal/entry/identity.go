package entry

import (
	"fmt"
	"strings"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/source"
)

func Identity(candidate ingestion.Candidate) (string, error) {
	if externalID := strings.TrimSpace(candidate.ExternalID); externalID != "" {
		return "external:" + externalID, nil
	}
	if rawURL := strings.TrimSpace(candidate.URL); rawURL != "" {
		normalized, err := source.NormalizeHTTPURL(rawURL)
		if err != nil {
			return "", fmt.Errorf("identify candidate URL: %w", err)
		}
		return "url:" + normalized, nil
	}

	title := strings.TrimSpace(candidate.Title)
	if title != "" && candidate.PublishedAt != nil {
		return "title-time:" + title + "|" + candidate.PublishedAt.UTC().Format("2006-01-02T15:04:05Z"), nil
	}
	if title != "" {
		return "title:" + title, nil
	}
	return "", fmt.Errorf("identify candidate: external ID, URL, or title is required")
}
