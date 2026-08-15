package document

import (
	"strings"
	"testing"
)

func TestChapterExcerptWindowsAroundSelection(t *testing.T) {
	chapter := "<p>" + strings.Repeat("前文。", 100) + "</p><p>如前所述，该机制会导致复杂性上升。</p><p>" + strings.Repeat("后文。", 100) + "</p>"

	excerpt := ChapterExcerpt(chapter, "该机制", 60)
	if !strings.Contains(excerpt, "该机制") {
		t.Errorf("excerpt = %q, want the selection inside", excerpt)
	}
	if strings.Contains(excerpt, "<p>") {
		t.Errorf("excerpt = %q, HTML must be stripped", excerpt)
	}
	count := len([]rune(excerpt))
	if count > 60+20 { // allowance for the ellipsis markers and selection itself
		t.Errorf("excerpt length = %d runes, want bounded around 60", count)
	}
	if !strings.Contains(excerpt, "前文") || !strings.Contains(excerpt, "后文") {
		t.Errorf("excerpt = %q, want text on both sides of the selection", excerpt)
	}
}

func TestChapterExcerptFallsBackToHead(t *testing.T) {
	chapter := "<p>" + strings.Repeat("开头内容。", 50) + "</p>"

	excerpt := ChapterExcerpt(chapter, "不存在的选区", 30)
	if !strings.HasPrefix(excerpt, "开头内容") {
		t.Errorf("excerpt = %q, want the chapter head", excerpt)
	}
	if count := len([]rune(excerpt)); count > 40 {
		t.Errorf("excerpt length = %d, want bounded", count)
	}
}
