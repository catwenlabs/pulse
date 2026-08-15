package document

import (
	"strings"
	"testing"
)

func TestParseMarkdownBuildsSingleChapter(t *testing.T) {
	parsed, err := ParseMarkdown("reading-notes.md", []byte("# 读书笔记\n\n第一段 **重点** 内容。\n\n<script>alert(1)</script>"))
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if parsed.Title != "读书笔记" {
		t.Errorf("Title = %q, want 读书笔记 from the first heading", parsed.Title)
	}
	if len(parsed.Chapters) != 1 {
		t.Fatalf("chapters = %d, want 1", len(parsed.Chapters))
	}
	chapter := parsed.Chapters[0]
	if !strings.Contains(chapter.ContentHTML, "<strong>重点</strong>") {
		t.Errorf("ContentHTML = %q, want rendered bold", chapter.ContentHTML)
	}
	if strings.Contains(chapter.ContentHTML, "<script>") {
		t.Errorf("ContentHTML = %q, raw HTML must not pass through", chapter.ContentHTML)
	}
	if !strings.Contains(chapter.ContentHTML, "<!-- raw HTML omitted -->") {
		t.Errorf("ContentHTML = %q, want raw HTML omitted by the safe renderer", chapter.ContentHTML)
	}
	if strings.Contains(chapter.ContentHTML, "读书笔记") {
		t.Errorf("ContentHTML = %q, the title heading should not repeat in the body", chapter.ContentHTML)
	}
}

func TestParseDispatchesMarkdownExtensions(t *testing.T) {
	parsed, err := Parse("reading-notes.markdown", []byte("# 标题\n\n正文。"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.Title != "标题" {
		t.Errorf("Title = %q, want 标题", parsed.Title)
	}
	if parsed.Chapters[0].ContentHTML != "<p>正文。</p>\n" {
		t.Errorf("ContentHTML = %q", parsed.Chapters[0].ContentHTML)
	}
}
