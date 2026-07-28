package annotation

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecodeBatchRejectsTooManyAnnotations(t *testing.T) {
	items := make([]string, MaxBatchEntries+1)
	for index := range items {
		items[index] = `{"provider":"kindle","book_title":"Book","highlight":"Text"}`
	}

	_, err := DecodeBatch([]byte(`{"annotations":[` + strings.Join(items, ",") + `]}`))
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("at most %d", MaxBatchEntries)) {
		t.Fatalf("DecodeBatch() error = %v", err)
	}
}

func TestDecodeBatchRejectsUnknownAndTrailingData(t *testing.T) {
	tests := []string{
		`{"annotations":[{"provider":"kindle","book_title":"Book","highlight":"Text","extra":true}]}`,
		`{"annotations":[{"provider":"kindle","book_title":"Book","highlight":"Text"}]} {}`,
	}
	for _, payload := range tests {
		if _, err := DecodeBatch([]byte(payload)); err == nil {
			t.Fatalf("DecodeBatch(%q) succeeded", payload)
		}
	}
}

func TestDecodeBatchNormalizesAndValidatesFields(t *testing.T) {
	batch, err := DecodeBatch([]byte(`{"annotations":[{"provider":" kindle ","book_title":" Book ","highlight":" Text ","highlighted_at":"2026-07-28T08:00:00Z"}]}`))
	if err != nil {
		t.Fatalf("DecodeBatch() error = %v", err)
	}
	if got := batch.Annotations[0].Provider; got != "kindle" {
		t.Fatalf("Provider = %q", got)
	}

	tooLongID := strings.Repeat("x", 513)
	_, err = DecodeBatch([]byte(fmt.Sprintf(
		`{"annotations":[{"id":%q,"provider":"kindle","book_title":"Book","highlight":"Text"}]}`,
		tooLongID,
	)))
	if err == nil || !strings.Contains(err.Error(), "id: exceeds 512 bytes") {
		t.Fatalf("DecodeBatch() error = %v", err)
	}
}
