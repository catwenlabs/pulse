package document

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ParseMarkdown converts an uploaded Markdown file into a single-chapter
// Document. The first level-1 heading becomes the title and is removed from
// the body; without one the filename without its extension is used. Raw HTML
// in the source is escaped by the renderer, never passed through.
func ParseMarkdown(filename string, content []byte) (Document, error) {
	// The default renderer escapes raw HTML in the source; html.WithUnsafe
	// must never be enabled for untrusted documents.
	parser := goldmark.New().Parser()
	document := parser.Parse(text.NewReader(content))

	title := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	for heading := document.FirstChild(); heading != nil; heading = heading.NextSibling() {
		if kind, ok := heading.(*ast.Heading); ok && kind.Level == 1 {
			title = strings.TrimSpace(string(kind.Text(content)))
			document.RemoveChild(document, heading)
			break
		}
	}

	var rendered bytes.Buffer
	if err := goldmark.New().Renderer().Render(&rendered, content, document); err != nil {
		return Document{}, fmt.Errorf("render markdown: %w", err)
	}

	return Document{
		Title:    title,
		Chapters: []Chapter{{Index: 0, Title: title, ContentHTML: rendered.String()}},
	}, nil
}
