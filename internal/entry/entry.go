package entry

import (
	"errors"
	"time"

	"github.com/catwenlabs/pulse/internal/annotation"
	"github.com/catwenlabs/pulse/internal/source"
)

var ErrNotFound = errors.New("entry not found")

type ID string

type Entry struct {
	ID           ID                 `json:"id"`
	SourceID     source.ID          `json:"source_id"`
	IdentityKey  string             `json:"identity_key"`
	ExternalID   string             `json:"external_id"`
	CanonicalURL string             `json:"canonical_url"`
	SourceTitle  string             `json:"source_title"`
	DisplayTitle string             `json:"display_title"`
	Author       string             `json:"author"`
	Summary      string             `json:"summary"`
	ContentHTML  string             `json:"content_html"`
	PublishedAt  *time.Time         `json:"published_at,omitempty"`
	DiscoveredAt time.Time          `json:"discovered_at"`
	ReadAt       *time.Time         `json:"read_at,omitempty"`
	StarredAt    *time.Time         `json:"starred_at,omitempty"`
	HiddenAt     *time.Time         `json:"hidden_at,omitempty"`
	LaterAt      *time.Time         `json:"later_at,omitempty"`
	Note         string             `json:"note"`
	Annotation   *annotation.Detail `json:"annotation,omitempty"`
}

type Query struct {
	Limit    int       `json:"limit,omitempty"`
	Offset   int       `json:"offset,omitempty"`
	Search   string    `json:"search,omitempty"`
	State    string    `json:"state,omitempty"`
	Tag      string    `json:"tag,omitempty"`
	SourceID source.ID `json:"source_id,omitempty"`
}

type Patch struct {
	Read         *bool
	Starred      *bool
	Hidden       *bool
	Later        *bool
	DisplayTitle *string
	Note         *string
}

type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
