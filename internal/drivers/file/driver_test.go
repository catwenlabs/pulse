package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

func TestDriverReadsMarkdownFromAllowedRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("# Local note\n\nHello **Pulse**."), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	driver := New([]string{root})
	batch, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source: source.Source{ID: "file-source", Kind: source.KindFile, Locator: path},
		Limits: ingestion.Limits{MaxBytes: 1024},
	})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if len(batch.Candidates) != 1 || batch.Candidates[0].Title != "Local note" ||
		!strings.Contains(batch.Candidates[0].ContentHTML, "<strong>Pulse</strong>") {
		t.Errorf("candidate = %+v", batch.Candidates)
	}
}

func TestDriverRejectsPathOutsideAllowedRoots(t *testing.T) {
	driver := New([]string{t.TempDir()})
	_, err := driver.Validate(context.Background(), source.Spec{
		Name: "outside", Kind: source.KindFile, Locator: "/etc/passwd",
	})
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
}
