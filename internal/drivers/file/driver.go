package file

import (
	"bufio"
	"context"
	"fmt"
	stdhtml "html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/source"
)

const defaultMaxBytes int64 = 4 << 20

var (
	boldPattern      = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	htmlTitlePattern = regexp.MustCompile(`(?is)<(?:title|h1)[^>]*>(.*?)</(?:title|h1)>`)
	htmlTagPattern   = regexp.MustCompile(`<[^>]+>`)
)

type Driver struct {
	roots []string
}

func New(roots []string) *Driver {
	canonical := make([]string, 0, len(roots))
	for _, root := range roots {
		if resolved, err := filepath.Abs(root); err == nil {
			canonical = append(canonical, filepath.Clean(resolved))
		}
	}
	return &Driver{roots: canonical}
}

func (driver *Driver) Kind() source.Kind {
	return source.KindFile
}

func (driver *Driver) Validate(_ context.Context, spec source.Spec) (source.ValidatedSpec, error) {
	if spec.Kind != source.KindFile {
		return source.ValidatedSpec{}, &source.ValidationError{
			Field: "kind", Message: "file driver requires file kind",
		}
	}
	validated, err := spec.Validate()
	if err != nil {
		return source.ValidatedSpec{}, err
	}
	path, err := driver.allowedPath(validated.Locator)
	if err != nil {
		return source.ValidatedSpec{}, &source.ValidationError{Field: "locator", Message: err.Error()}
	}
	validated.Locator = path
	validated.NormalizedLocator = path
	return validated, nil
}

func (driver *Driver) Acquire(
	_ context.Context,
	request ingestion.AcquireRequest,
) (ingestion.AcquisitionBatch, error) {
	path, err := driver.allowedPath(request.Source.Locator)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("open file source: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("open file source: %w", err)
	}
	defer file.Close()
	maxBytes := request.Limits.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read file source: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read file source: file exceeds %d bytes", maxBytes)
	}
	title, content := fileContent(path, string(body))
	return ingestion.AcquisitionBatch{
		Candidates: []ingestion.Candidate{{
			ExternalID: string(request.Source.ID),
			URL:        "file://" + filepath.ToSlash(path),
			Title:      title, ContentHTML: content,
		}},
		Diagnostics: ingestion.Diagnostics{Status: "ok", CandidateCount: 1},
	}, nil
}

func (driver *Driver) allowedPath(locator string) (string, error) {
	absolute, err := filepath.Abs(locator)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}
	extension := strings.ToLower(filepath.Ext(absolute))
	if extension != ".md" && extension != ".markdown" && extension != ".html" && extension != ".htm" {
		return "", fmt.Errorf("only Markdown and HTML files are supported")
	}
	for _, root := range driver.roots {
		resolvedRoot := root
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			resolvedRoot = resolved
		}
		relative, err := filepath.Rel(resolvedRoot, absolute)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("path is outside configured import roots")
}

func fileContent(path, body string) (string, string) {
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".html" || extension == ".htm" {
		title := strings.TrimSuffix(filepath.Base(path), extension)
		if match := htmlTitlePattern.FindStringSubmatch(body); len(match) == 2 {
			title = strings.TrimSpace(stdhtml.UnescapeString(htmlTagPattern.ReplaceAllString(match[1], "")))
		}
		return title, body
	}

	title := strings.TrimSuffix(filepath.Base(path), extension)
	var output strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}
		escaped := stdhtml.EscapeString(line)
		escaped = boldPattern.ReplaceAllString(escaped, "<strong>$1</strong>")
		output.WriteString("<p>")
		output.WriteString(escaped)
		output.WriteString("</p>")
	}
	return title, output.String()
}
