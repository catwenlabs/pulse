package story

import (
	"time"

	"github.com/catwenlabs/pulse/internal/entry"
)

type ID string

type Story struct {
	ID               ID            `json:"id"`
	SortTime         *time.Time    `json:"-"`
	DisplayTitle     string        `json:"display_title,omitempty"`
	Note             string        `json:"note,omitempty"`
	Tags             []entry.Tag   `json:"tags,omitempty"`
	MatchedEntry     *entry.Entry  `json:"matched_entry,omitempty"`
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

// SourceEntry is the explicit response shape used when browsing a Source. The
// Entry remains the source-provided content record; all Reader state and user
// metadata are projected from its owning Story.
type SourceEntry struct {
	Entry entry.Entry `json:"entry"`
	Story StoryRef    `json:"story"`
}

type ReaderCounts struct {
	InboxStories   int `json:"inbox_stories"`
	UnreadStories  int `json:"unread_stories"`
	StarredStories int `json:"starred_stories"`
	LaterStories   int `json:"later_stories"`
	HiddenStories  int `json:"hidden_stories"`
}

type Page struct {
	Stories      []Story      `json:"stories"`
	TotalStories int          `json:"total_stories"`
	ReaderCounts ReaderCounts `json:"reader_counts"`
	NextCursor   string       `json:"next_cursor,omitempty"`
}

type SourceEntryPage struct {
	Entries      []SourceEntry `json:"entries"`
	TotalEntries int           `json:"total_entries"`
	ReaderCounts ReaderCounts  `json:"reader_counts"`
	NextCursor   string        `json:"next_cursor,omitempty"`
}

type StoryRef struct {
	ID           ID          `json:"id"`
	EntryCount   int         `json:"entry_count"`
	SourceCount  int         `json:"source_count"`
	DisplayTitle string      `json:"display_title,omitempty"`
	Note         string      `json:"note,omitempty"`
	Tags         []entry.Tag `json:"tags,omitempty"`
	ReadAt       *time.Time  `json:"read_at,omitempty"`
	StarredAt    *time.Time  `json:"starred_at,omitempty"`
	HiddenAt     *time.Time  `json:"hidden_at,omitempty"`
	LaterAt      *time.Time  `json:"later_at,omitempty"`
}

type Patch struct {
	Read         *bool
	Starred      *bool
	Hidden       *bool
	Later        *bool
	DisplayTitle *string
	Note         *string
}

type SplitOptions struct {
	CopyDisplayTitle bool
	MoveDisplayTitle bool
	CopyNote         bool
	MoveNote         bool
	CopyTags         bool
	MoveTags         bool
}

type MergeOptions struct {
	DisplayTitle *string
	Note         *string
}

type Query struct {
	Limit    int
	Offset   int
	Cursor   string
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
	EntryCount  int
}
