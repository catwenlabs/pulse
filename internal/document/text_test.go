package document

import (
	"strings"
	"testing"
)

func TestParseTextBuildsSingleEscapedChapter(t *testing.T) {
	parsed, err := ParseText("reading-notes.txt", []byte("第一段内容。\n\n第二段 <b>加粗</b> 内容。"))
	if err != nil {
		t.Fatalf("ParseText() error = %v", err)
	}
	if parsed.Title != "reading-notes" {
		t.Errorf("Title = %q, want reading-notes", parsed.Title)
	}
	if len(parsed.Chapters) != 1 {
		t.Fatalf("chapters = %d, want 1", len(parsed.Chapters))
	}
	chapter := parsed.Chapters[0]
	if chapter.Index != 0 {
		t.Errorf("Index = %d, want 0", chapter.Index)
	}
	if chapter.Title != parsed.Title {
		t.Errorf("chapter Title = %q, want %q", chapter.Title, parsed.Title)
	}
	want := "<p>第一段内容。</p><p>第二段 &lt;b&gt;加粗&lt;/b&gt; 内容。</p>"
	if chapter.ContentHTML != want {
		t.Errorf("ContentHTML = %q, want %q", chapter.ContentHTML, want)
	}
	if strings.Contains(chapter.ContentHTML, "<b>") {
		t.Errorf("ContentHTML must escape raw HTML, got %q", chapter.ContentHTML)
	}
}
