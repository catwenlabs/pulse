package annotation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const MaxBatchEntries = 500

type Batch struct {
	Annotations []Input `json:"annotations"`
}

type Input struct {
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	BookIdentity   string `json:"book_identity"`
	BookTitle      string `json:"book_title"`
	BookAuthor     string `json:"book_author"`
	Chapter        string `json:"chapter"`
	Location       string `json:"location"`
	HighlightColor string `json:"highlight_color"`
	Highlight      string `json:"highlight"`
	Note           string `json:"note"`
	HighlightedAt  string `json:"highlighted_at"`
}

func DecodeBatch(data []byte) (Batch, error) {
	var batch Batch
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&batch); err != nil {
		return Batch{}, fmt.Errorf("decode annotations: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("unexpected trailing JSON value")
		}
		return Batch{}, fmt.Errorf("decode annotations: %w", err)
	}
	if len(batch.Annotations) == 0 {
		return Batch{}, fmt.Errorf("annotations must contain at least one entry")
	}
	if len(batch.Annotations) > MaxBatchEntries {
		return Batch{}, fmt.Errorf("annotations must contain at most %d entries", MaxBatchEntries)
	}
	for index := range batch.Annotations {
		if err := batch.Annotations[index].normalizeAndValidate(); err != nil {
			return Batch{}, fmt.Errorf("annotations[%d].%w", index, err)
		}
	}
	return batch, nil
}

func (input *Input) normalizeAndValidate() error {
	input.ID = strings.TrimSpace(input.ID)
	input.Provider = strings.TrimSpace(input.Provider)
	input.BookIdentity = strings.TrimSpace(input.BookIdentity)
	input.BookTitle = strings.TrimSpace(input.BookTitle)
	input.BookAuthor = strings.TrimSpace(input.BookAuthor)
	input.Chapter = strings.TrimSpace(input.Chapter)
	input.Location = strings.TrimSpace(input.Location)
	input.HighlightColor = strings.TrimSpace(input.HighlightColor)
	input.Highlight = strings.TrimSpace(input.Highlight)
	input.Note = strings.TrimSpace(input.Note)
	input.HighlightedAt = strings.TrimSpace(input.HighlightedAt)
	for _, field := range []struct {
		name  string
		value string
		max   int
	}{
		{"id", input.ID, 512},
		{"provider", input.Provider, 64},
		{"book_identity", input.BookIdentity, 512},
		{"book_title", input.BookTitle, 2048},
		{"book_author", input.BookAuthor, 512},
		{"chapter", input.Chapter, 1024},
		{"location", input.Location, 256},
		{"highlight_color", input.HighlightColor, 32},
		{"highlight", input.Highlight, 256 << 10},
		{"note", input.Note, 256 << 10},
		{"highlighted_at", input.HighlightedAt, 64},
	} {
		if len(field.value) > field.max {
			return fmt.Errorf("%s: exceeds %d bytes", field.name, field.max)
		}
	}
	if input.Provider == "" {
		return fmt.Errorf("provider: must not be empty")
	}
	if input.BookTitle == "" {
		return fmt.Errorf("book_title: must not be empty")
	}
	if input.Highlight == "" {
		return fmt.Errorf("highlight: must not be empty")
	}
	if input.HighlightedAt != "" {
		if _, err := time.Parse(time.RFC3339, input.HighlightedAt); err != nil {
			return fmt.Errorf("highlighted_at: must be RFC3339: %w", err)
		}
	}
	return nil
}

// Detail contains the source-owned fields for an Entry created from a book annotation.
// User-authored Pulse notes remain on entry.Entry and are never overwritten by imports.
type Detail struct {
	Provider       string     `json:"provider"`
	BookIdentity   string     `json:"book_identity"`
	BookTitle      string     `json:"book_title"`
	BookAuthor     string     `json:"book_author"`
	Chapter        string     `json:"chapter"`
	Location       string     `json:"location"`
	HighlightColor string     `json:"highlight_color"`
	AnnotationNote string     `json:"annotation_note"`
	HighlightedAt  *time.Time `json:"highlighted_at,omitempty"`
}
