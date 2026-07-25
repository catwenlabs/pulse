package push

import (
	"bytes"
	"context"
	"testing"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

func TestWebhookAndManualDriversDecodeCandidatePayload(t *testing.T) {
	for _, kind := range []source.Kind{source.KindWebhook, source.KindManual} {
		t.Run(string(kind), func(t *testing.T) {
			driver := New(kind)
			batch, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
				Source:  source.Source{ID: "source-1", Kind: kind},
				Payload: bytes.NewBufferString(`{"id":"item-1","title":"Pushed","url":"https://example.com/item"}`),
				Limits:  ingestion.Limits{MaxBytes: 1024},
			})
			if err != nil {
				t.Fatalf("Acquire() error = %v", err)
			}
			if len(batch.Candidates) != 1 || batch.Candidates[0].ExternalID != "item-1" ||
				batch.Candidates[0].Title != "Pushed" {
				t.Errorf("candidates = %+v", batch.Candidates)
			}
		})
	}
}

func TestPushDriverRejectsOversizedAndUnknownJSON(t *testing.T) {
	driver := New(source.KindWebhook)
	if _, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source:  source.Source{Kind: source.KindWebhook},
		Payload: bytes.NewBufferString(`{"title":"too long"}`),
		Limits:  ingestion.Limits{MaxBytes: 4},
	}); err == nil {
		t.Fatal("size error = nil")
	}
	if _, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source:  source.Source{Kind: source.KindWebhook},
		Payload: bytes.NewBufferString(`{"title":"ok","unexpected":true}`),
		Limits:  ingestion.Limits{MaxBytes: 1024},
	}); err == nil {
		t.Fatal("unknown field error = nil")
	}
}
