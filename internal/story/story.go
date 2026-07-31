package story

import (
	"time"

	"github.com/catwenlabs/pulse/internal/entry"
)

type ID string

type Story struct {
	ID               ID            `json:"id"`
	Representative   entry.Entry   `json:"representative"`
	Entries          []entry.Entry `json:"entries,omitempty"`
	EntryCount       int           `json:"entry_count"`
	SourceCount      int           `json:"source_count"`
	FirstPublishedAt *time.Time    `json:"first_published_at,omitempty"`
	LastPublishedAt  *time.Time    `json:"last_published_at,omitempty"`
	ReadAt           *time.Time    `json:"read_at,omitempty"`
	StarredAt        *time.Time    `json:"starred_at,omitempty"`
	HiddenAt         *time.Time    `json:"hidden_at,omitempty"`
	LaterAt          *time.Time    `json:"later_at,omitempty"`
}

type Patch struct {
	Read    *bool
	Starred *bool
	Hidden  *bool
	Later   *bool
}

type Query struct {
	Limit    int
	Offset   int
	Search   string
	State    string
	Tag      string
	SourceID string
}

type Candidate struct {
	StoryID     ID
	Entry       entry.Entry
	Features    Features
	ClusteredAt *time.Time
}
