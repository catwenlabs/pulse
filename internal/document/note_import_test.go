package document

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseNoteImportAcceptsFullContract(t *testing.T) {
	when := "2026-01-15T10:30:00Z"
	file, err := ParseNoteImport([]byte(jsonString(t, map[string]any{
		"notes": []any{
			map[string]any{
				"book_identifier": "urn:isbn:9787544253994",
				"book_title":      "人类简史",
				"book_author":     "尤瓦尔·赫拉利",
				"chapter_index":   3,
				"location":        "loc-1287",
				"highlight":       "该机制会导致复杂性上升。",
				"note":            "和上一章呼应",
				"highlight_color": "yellow",
				"highlighted_at":  when,
			},
		},
	})))
	if err != nil {
		t.Fatalf("ParseNoteImport error = %v", err)
	}
	if len(file.Notes) != 1 {
		t.Fatalf("notes = %d, want 1", len(file.Notes))
	}
	entry := file.Notes[0]
	if entry.BookIdentifier != "urn:isbn:9787544253994" || entry.BookTitle != "人类简史" || entry.BookAuthor != "尤瓦尔·赫拉利" {
		t.Errorf("book identity = %+v", entry)
	}
	if entry.ChapterIndex != 3 || entry.Location != "loc-1287" {
		t.Errorf("placement = %+v", entry)
	}
	if entry.Highlight == "" || entry.Note != "和上一章呼应" || entry.Color != "yellow" {
		t.Errorf("payload = %+v", entry)
	}
	if entry.HighlightedAt == nil || !entry.HighlightedAt.Equal(time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)) {
		t.Errorf("highlighted_at = %v", entry.HighlightedAt)
	}
}

func TestParseNoteImportAllowsMinimalEntry(t *testing.T) {
	file, err := ParseNoteImport([]byte(`{"notes":[{"book_title":"只有书名","highlight":"一句话"}]}`))
	if err != nil {
		t.Fatalf("ParseNoteImport error = %v", err)
	}
	if len(file.Notes) != 1 || file.Notes[0].BookTitle != "只有书名" {
		t.Fatalf("file = %+v", file)
	}
}

func TestParseNoteImportRejectsInvalidEntries(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"empty notes", `{"notes":[]}`, "notes"},
		{"no book identity", `{"notes":[{"highlight":"x"}]}`, "notes[0].book_title"},
		{"empty highlight", `{"notes":[{"book_title":"书","highlight":"  "}]}`, "notes[0].highlight"},
		{"oversized highlight", `{"notes":[{"book_title":"书","highlight":"` + repeatRunes(10001) + `"}]}`, "notes[0].highlight"},
		{"bad timestamp", `{"notes":[{"book_title":"书","highlight":"x","highlighted_at":"not-a-time"}]}`, "notes[0].highlighted_at"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ParseNoteImport([]byte(testCase.body))
			if err == nil {
				t.Fatalf("ParseNoteImport(%s) succeeded, want error", testCase.name)
			}
			validation, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("error = %T, want *ValidationError", err)
			}
			if validation.Field != testCase.want {
				t.Errorf("field = %q, want %q", validation.Field, testCase.want)
			}
		})
	}
}

func jsonString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}

func repeatRunes(count int) string {
	runes := make([]rune, count)
	for index := range runes {
		runes[index] = '长'
	}
	return string(runes)
}
