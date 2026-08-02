package entry

import (
	"errors"
	"time"

	"github.com/catwenlabs/pulse/internal/annotation"
	"github.com/catwenlabs/pulse/internal/source"
)

var (
	ErrNotFound             = errors.New("entry not found")
	ErrDeletionConfirmation = errors.New("deleting the final Entry requires confirmation")
)

type DeletionConfirmationError struct {
	StoryID      string
	DisplayTitle string
	Note         string
	EntryCount   int
}

func (err *DeletionConfirmationError) Error() string {
	return "deleting the final Entry would also remove its Story metadata"
}

func (err *DeletionConfirmationError) Unwrap() error {
	return ErrDeletionConfirmation
}

type ID string

type Entry struct {
	ID           ID                 `json:"id"`
	SourceID     source.ID          `json:"source_id"`
	IdentityKey  string             `json:"identity_key"`
	ExternalID   string             `json:"external_id"`
	CanonicalURL string             `json:"canonical_url"`
	SourceTitle  string             `json:"source_title"`
	Author       string             `json:"author"`
	Summary      string             `json:"summary"`
	ContentHTML  string             `json:"content_html"`
	PublishedAt  *time.Time         `json:"published_at,omitempty"`
	DiscoveredAt time.Time          `json:"discovered_at"`
	Annotation   *annotation.Detail `json:"annotation,omitempty"`
}

type Query struct {
	Limit    int       `json:"limit,omitempty"`
	Offset   int       `json:"offset,omitempty"`
	Cursor   string    `json:"cursor,omitempty"`
	Search   string    `json:"search,omitempty"`
	State    string    `json:"state,omitempty"`
	Tag      string    `json:"tag,omitempty"`
	SourceID source.ID `json:"source_id,omitempty"`
}

type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
