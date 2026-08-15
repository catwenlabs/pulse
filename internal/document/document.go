// Package document holds the imported long-form reading domain. Documents are
// parallel to Entry: they never bind to a Source, never enter the Entry
// pipeline, and never join Stories. A Document is "a long article with
// chapters", not an e-book library.
package document

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrNotFound is returned when a Document does not exist.
	ErrNotFound = errors.New("document not found")
	// ErrDuplicate is returned when an import matches an existing document
	// by identifier or by title and author.
	ErrDuplicate = errors.New("document already imported")
	// ErrUnavailable is returned when no document store is wired in.
	ErrUnavailable = errors.New("documents are unavailable")
)

// ValidationError reports an invalid document import payload.
type ValidationError struct {
	Field   string
	Message string
}

func (err *ValidationError) Error() string {
	return err.Field + ": " + err.Message
}

// ID identifies a Document.
type ID string

// Document is one imported long-form reading object.
type Document struct {
	ID         ID        `json:"id"`
	Identifier string    `json:"identifier,omitempty"`
	Title      string    `json:"title"`
	Author     string    `json:"author,omitempty"`
	Chapters   []Chapter `json:"chapters"`
	// Progress is the reading position when reading has started, nil before.
	Progress *Progress `json:"progress,omitempty"`
}

// Chapter is one sanitized reading unit of a Document.
type Chapter struct {
	Index       int    `json:"index"`
	Title       string `json:"title"`
	ContentHTML string `json:"content_html"`
}

// Summary is a Document without chapter bodies, used in list views.
type Summary struct {
	ID           ID     `json:"id"`
	Identifier   string `json:"identifier,omitempty"`
	Title        string `json:"title"`
	Author       string `json:"author,omitempty"`
	ChapterCount int    `json:"chapter_count"`
}

// ImportRequest carries one uploaded file before format-specific parsing.
type ImportRequest struct {
	Filename string
	Content  []byte
}

// Progress records where reading stopped: the chapter plus the scrolled
// fraction within it (0 <= ScrollRatio <= 1).
type Progress struct {
	ChapterIndex int     `json:"chapter_index"`
	ScrollRatio  float64 `json:"scroll_ratio"`
}

// Validate checks the progress bounds.
func (progress Progress) Validate() error {
	if progress.ChapterIndex < 0 {
		return &ValidationError{Field: "chapter_index", Message: "must not be negative"}
	}
	if progress.ScrollRatio < 0 || progress.ScrollRatio > 1 {
		return &ValidationError{Field: "scroll_ratio", Message: "must be between 0 and 1"}
	}
	return nil
}

// MaxNoteHighlightCharacters bounds the highlighted text of one note.
const MaxNoteHighlightCharacters = 10000

// NoteInput carries one highlight written while reading in Pulse.
type NoteInput struct {
	ChapterIndex int    `json:"chapter_index"`
	Highlight    string `json:"highlight"`
	Text         string `json:"note,omitempty"`
	Color        string `json:"highlight_color,omitempty"`
}

// Validate checks the note bounds.
func (input NoteInput) Validate() error {
	if input.ChapterIndex < 0 {
		return &ValidationError{Field: "chapter_index", Message: "must not be negative"}
	}
	highlight := strings.TrimSpace(input.Highlight)
	if highlight == "" {
		return &ValidationError{Field: "highlight", Message: "must not be empty"}
	}
	if len([]rune(highlight)) > MaxNoteHighlightCharacters {
		return &ValidationError{Field: "highlight", Message: "must not exceed " + strconv.Itoa(MaxNoteHighlightCharacters) + " characters"}
	}
	return nil
}

// Note is one highlight (with optional personal note) on a document.
type Note struct {
	ID            string     `json:"id"`
	DocumentID    ID         `json:"document_id"`
	ChapterIndex  int        `json:"chapter_index"`
	Highlight     string     `json:"highlight"`
	Text          string     `json:"note,omitempty"`
	Color         string     `json:"highlight_color,omitempty"`
	HighlightedAt *time.Time `json:"highlighted_at,omitempty"`
}
