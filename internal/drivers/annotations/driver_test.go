package annotations

import (
	"context"
	"strings"
	"testing"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

func TestAcquireParsesAnnotationBatch(t *testing.T) {
	driver := New()
	batch, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source: source.Source{Kind: source.KindAnnotations},
		Payload: strings.NewReader(`{
			"annotations": [{
				"provider": "apple-books",
				"book_identity": "book-123",
				"book_title": "思考，快与慢",
				"book_author": "Daniel Kahneman",
				"chapter": "第三章",
				"location": "1284",
				"highlight_color": "yellow",
				"highlight": "系统一自动而快速地运行。",
				"note": "这里对应直觉判断。",
				"highlighted_at": "2026-07-27T10:00:00Z"
			}]
		}`),
	})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if len(batch.Candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(batch.Candidates))
	}
	candidate := batch.Candidates[0]
	if candidate.ExternalID != "apple-books:book-123:1284" {
		t.Errorf("external id = %q", candidate.ExternalID)
	}
	if candidate.Title != "思考，快与慢" || candidate.Author != "Daniel Kahneman" {
		t.Errorf("candidate = %#v", candidate)
	}
	if candidate.Annotation == nil || candidate.Annotation.AnnotationNote != "这里对应直觉判断。" {
		t.Fatalf("annotation = %#v", candidate.Annotation)
	}
	if !strings.Contains(candidate.ContentHTML, "<blockquote>系统一自动而快速地运行。</blockquote>") {
		t.Errorf("content html = %q", candidate.ContentHTML)
	}
}

func TestAcquireUsesContentFingerprintWithoutLocation(t *testing.T) {
	driver := New()
	payload := `{"annotations":[{
		"provider":"kindle",
		"book_title":"Deep Work",
		"book_author":"Cal Newport",
		"highlight":"Clarity about what matters provides clarity about what does not."
	}]}`
	first, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source:  source.Source{Kind: source.KindAnnotations},
		Payload: strings.NewReader(payload),
	})
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	second, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source:  source.Source{Kind: source.KindAnnotations},
		Payload: strings.NewReader(payload),
	})
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}
	if first.Candidates[0].ExternalID == "" ||
		first.Candidates[0].ExternalID != second.Candidates[0].ExternalID {
		t.Errorf("external IDs = %q, %q", first.Candidates[0].ExternalID, second.Candidates[0].ExternalID)
	}
}

func TestAcquireRejectsInvalidAnnotationWithoutPartialBatch(t *testing.T) {
	driver := New()
	_, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source: source.Source{Kind: source.KindAnnotations},
		Payload: strings.NewReader(`{"annotations":[
			{"provider":"kindle","book_title":"Valid","highlight":"Useful"},
			{"provider":"kindle","book_title":"","highlight":"Missing book"}
		]}`),
	})
	if err == nil || !strings.Contains(err.Error(), "annotations[1].book_title") {
		t.Fatalf("Acquire() error = %v", err)
	}
}

func TestValidateRequiresAnnotationKind(t *testing.T) {
	driver := New()
	if _, err := driver.Validate(context.Background(), source.Spec{
		Name: "Books", Kind: source.KindManual, Locator: "books",
	}); err == nil {
		t.Fatal("Validate() error = nil")
	}
}
