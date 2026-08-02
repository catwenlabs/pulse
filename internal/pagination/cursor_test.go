package pagination

import (
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTripAndFilterBinding(t *testing.T) {
	position := Position{
		Kind: "stories", Search: "go", State: "unread", Tag: "tech",
		SourceID: "00000000-0000-0000-0000-000000000001", Bucket: 1,
		Time: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
		ID:   "00000000-0000-0000-0000-000000000002",
	}
	raw, err := Encode(position)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := Decode(raw, "stories", "go", "unread", "tech", position.SourceID)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded != position {
		t.Fatalf("Decode() = %+v, want %+v", decoded, position)
	}
	if _, err := Decode(raw, "stories", "go", "read", "tech", position.SourceID); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("Decode() mismatch error = %v, want ErrInvalidCursor", err)
	}
}
