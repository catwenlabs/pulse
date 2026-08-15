package document

import (
	"strings"

	"golang.org/x/net/html"
)

// ChapterExcerpt returns the chapter's plain text windowed around the first
// occurrence of the selection (whitespace-insensitive), bounded to maxRunes.
// When the selection is not found, the chapter head is returned instead.
func ChapterExcerpt(contentHTML, selection string, maxRunes int) string {
	text := htmlToText(contentHTML)
	runes := []rune(text)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return text
	}
	needle := strings.TrimSpace(selection)
	offset := -1
	if needle != "" {
		offset = indexRunes(runes, []rune(needle))
	}
	if offset < 0 {
		return ellipsizeSuffix(string(runes[:maxRunes]))
	}
	// Keep a window centered slightly before the selection so the argument
	// leading into it survives.
	half := maxRunes / 2
	start := offset - half
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
		start = end - maxRunes
		if start < 0 {
			start = 0
		}
	}
	window := string(runes[start:end])
	if start > 0 {
		window = "…" + window
	}
	if end < len(runes) {
		window += "…"
	}
	return window
}

// indexRunes finds the first occurrence of needle in haystack in rune space.
// Both sides are already whitespace-normalized by htmlToText.
func indexRunes(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for start := 0; start+len(needle) <= len(haystack); start++ {
		match := true
		for offset, char := range needle {
			if haystack[start+offset] != char {
				match = false
				break
			}
		}
		if match {
			return start
		}
	}
	return -1
}

func ellipsizeSuffix(text string) string {
	return text + "…"
}

// htmlToText strips tags and collapses whitespace, returning visible text.
func htmlToText(contentHTML string) string {
	node, err := html.Parse(strings.NewReader(contentHTML))
	if err != nil {
		return strings.TrimSpace(contentHTML)
	}
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			builder.WriteString(" ")
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}
