// Package document holds the imported long-form reading domain. Documents are
// parallel to Entry: they never bind to a Source, never enter the Entry
// pipeline, and never join Stories. A Document is "a long article with
// chapters", not an e-book library.
package document

import "errors"

var (
	// ErrNotFound is returned when a Document does not exist.
	ErrNotFound = errors.New("document not found")
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
