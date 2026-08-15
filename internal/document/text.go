package document

import (
	"html"
	"path/filepath"
	"strings"
)

// Parse converts an uploaded file into a Document by dispatching on the
// filename extension. Unsupported files fail with a ValidationError.
func Parse(filename string, content []byte) (Document, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt":
		return ParseText(filename, content)
	case ".md", ".markdown":
		return ParseMarkdown(filename, content)
	case ".epub":
		return ParseEpub(content)
	default:
		return Document{}, &ValidationError{Field: "filename", Message: "unsupported file type: " + filepath.Ext(filename)}
	}
}

// ParseText converts an uploaded plain-text file into a single-chapter
// Document. The filename without its extension becomes the title; blank-line
// separated blocks become escaped paragraphs.
func ParseText(filename string, content []byte) (Document, error) {
	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	var builder strings.Builder
	for _, block := range strings.Split(string(content), "\n\n") {
		paragraph := strings.Join(strings.Fields(block), " ")
		if paragraph == "" {
			continue
		}
		builder.WriteString("<p>" + html.EscapeString(paragraph) + "</p>")
	}
	return Document{
		Title:    title,
		Chapters: []Chapter{{Index: 0, Title: title, ContentHTML: builder.String()}},
	}, nil
}
