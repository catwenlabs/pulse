package document

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// NoteImport is one entry of the neutral note-import contract. It carries no
// channel/provider vocabulary: whoever prepares the file maps their source to
// this shape themselves.
type NoteImport struct {
	// timeError records an unparsable highlighted_at so validation can
	// report it against the field instead of a generic body error.
	timeError error

	BookIdentifier string     `json:"book_identifier,omitempty"`
	BookTitle      string     `json:"book_title"`
	BookAuthor     string     `json:"book_author,omitempty"`
	ChapterIndex   int        `json:"chapter_index,omitempty"`
	Location       string     `json:"location,omitempty"`
	Highlight      string     `json:"highlight"`
	Note           string     `json:"note,omitempty"`
	Color          string     `json:"highlight_color,omitempty"`
	HighlightedAt  *time.Time `json:"-"`
}

// UnmarshalJSON accepts highlighted_at as an RFC 3339 string and defers a
// parse failure to validation.
func (entry *NoteImport) UnmarshalJSON(data []byte) error {
	type plain NoteImport
	var raw struct {
		plain
		HighlightedAt string `json:"highlighted_at,omitempty"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	*entry = NoteImport(raw.plain)
	if strings.TrimSpace(raw.HighlightedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.HighlightedAt))
		if err != nil {
			entry.timeError = err
		} else {
			entry.HighlightedAt = &parsed
		}
	}
	return nil
}

// NoteImportFile is the top-level import document.
type NoteImportFile struct {
	Notes []NoteImport `json:"notes"`
}

// ParseNoteImport decodes and validates an import file. Every entry must
// identify its book (identifier, or title) and carry a non-empty highlight.
func ParseNoteImport(data []byte) (NoteImportFile, error) {
	var file NoteImportFile
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return NoteImportFile{}, &ValidationError{Field: "body", Message: err.Error()}
	}
	if err := ensureJSONTail(decoder); err != nil {
		return NoteImportFile{}, err
	}
	if len(file.Notes) == 0 {
		return NoteImportFile{}, &ValidationError{Field: "notes", Message: "must not be empty"}
	}
	for index := range file.Notes {
		if err := file.Notes[index].validate(index); err != nil {
			return NoteImportFile{}, err
		}
	}
	return file, nil
}

func (entry NoteImport) validate(index int) error {
	field := func(name string) string {
		return "notes[" + strconv.Itoa(index) + "]." + name
	}
	if strings.TrimSpace(entry.BookIdentifier) == "" && strings.TrimSpace(entry.BookTitle) == "" {
		return &ValidationError{Field: field("book_title"), Message: "book must have an identifier or a title"}
	}
	highlight := strings.TrimSpace(entry.Highlight)
	if highlight == "" {
		return &ValidationError{Field: field("highlight"), Message: "must not be empty"}
	}
	if len([]rune(highlight)) > MaxNoteHighlightCharacters {
		return &ValidationError{Field: field("highlight"), Message: "must not exceed " + strconv.Itoa(MaxNoteHighlightCharacters) + " characters"}
	}
	if entry.ChapterIndex < 0 {
		return &ValidationError{Field: field("chapter_index"), Message: "must not be negative"}
	}
	if entry.timeError != nil {
		return &ValidationError{Field: field("highlighted_at"), Message: "must be RFC 3339"}
	}
	return nil
}

// ensureJSONTail rejects trailing content after the top-level object.
func ensureJSONTail(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return &ValidationError{Field: "body", Message: "unexpected trailing content"}
		}
		return &ValidationError{Field: "body", Message: fmt.Sprintf("invalid JSON: %v", err)}
	}
	return nil
}
