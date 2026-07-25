package html

import (
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

const defaultMaxBytes int64 = 4 << 20

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Driver struct {
	client HTTPClient
}

type config struct {
	Mode         string                  `json:"mode"`
	ItemSelector string                  `json:"item_selector,omitempty"`
	Fields       map[string]fieldMapping `json:"fields"`
}

type fieldMapping struct {
	Selector  string `json:"selector"`
	Attribute string `json:"attribute,omitempty"`
	HTML      bool   `json:"html,omitempty"`
}

type node struct {
	tag      string
	attrs    map[string]string
	text     string
	parent   *node
	children []*node
}

func New(client HTTPClient) *Driver {
	return &Driver{client: client}
}

func (driver *Driver) Kind() source.Kind {
	return source.KindHTML
}

func (driver *Driver) Validate(_ context.Context, spec source.Spec) (source.ValidatedSpec, error) {
	if spec.Kind != source.KindHTML {
		return source.ValidatedSpec{}, &source.ValidationError{
			Field: "kind", Message: "HTML driver requires html kind",
		}
	}
	validated, err := spec.Validate()
	if err != nil {
		return source.ValidatedSpec{}, err
	}
	if _, err := decodeConfig(validated.Config); err != nil {
		return source.ValidatedSpec{}, &source.ValidationError{Field: "config", Message: err.Error()}
	}
	return validated, nil
}

func (driver *Driver) Acquire(
	ctx context.Context,
	acquireRequest ingestion.AcquireRequest,
) (ingestion.AcquisitionBatch, error) {
	cfg, err := decodeConfig(acquireRequest.Source.Config)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("decode HTML config: %w", err)
	}
	if acquireRequest.Limits.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, acquireRequest.Limits.MaxDuration)
		defer cancel()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, acquireRequest.Source.Locator, nil)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("create HTML request: %w", err)
	}
	request.Header.Set("Accept", "text/html, application/xhtml+xml")
	response, err := driver.client.Do(request)
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("fetch HTML: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("fetch HTML: HTTP status %d", response.StatusCode)
	}
	maxBytes := acquireRequest.Limits.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read HTML: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("read HTML: response exceeds %d bytes", maxBytes)
	}
	document, err := parseDocument(string(body))
	if err != nil {
		return ingestion.AcquisitionBatch{}, fmt.Errorf("parse HTML: %w", err)
	}

	roots := []*node{document}
	if cfg.Mode == "collection" {
		roots = selectNodes(document, cfg.ItemSelector)
		if len(roots) == 0 {
			return ingestion.AcquisitionBatch{}, fmt.Errorf("extract HTML collection: zero items matched %q", cfg.ItemSelector)
		}
	}
	maxEntries := acquireRequest.Limits.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	details := make(map[string]string)
	if len(roots) > maxEntries {
		roots = roots[:maxEntries]
		details["truncated"] = "entries"
	}
	candidates := make([]ingestion.Candidate, 0, len(roots))
	for _, root := range roots {
		candidate := mapNode(root, cfg.Fields, response.Request, acquireRequest.Source.Locator)
		if cfg.Mode == "single" {
			candidate.ExternalID = string(acquireRequest.Source.ID)
			if candidate.URL == "" {
				candidate.URL = acquireRequest.Source.Locator
			}
		}
		candidates = append(candidates, candidate)
	}
	return ingestion.AcquisitionBatch{
		Candidates: candidates,
		Diagnostics: ingestion.Diagnostics{
			Status: "ok", CandidateCount: len(candidates), Details: details,
		},
	}, nil
}

func decodeConfig(raw json.RawMessage) (config, error) {
	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return config{}, err
	}
	if cfg.Mode == "" {
		cfg.Mode = "collection"
	}
	if cfg.Mode != "single" && cfg.Mode != "collection" {
		return config{}, fmt.Errorf("mode must be single or collection")
	}
	if cfg.Mode == "collection" && strings.TrimSpace(cfg.ItemSelector) == "" {
		return config{}, fmt.Errorf("item_selector is required for collection mode")
	}
	if len(cfg.Fields) == 0 {
		return config{}, fmt.Errorf("fields must include at least one mapping")
	}
	for name, mapping := range cfg.Fields {
		if strings.TrimSpace(mapping.Selector) == "" && cfg.Mode == "collection" {
			return config{}, fmt.Errorf("fields.%s.selector is required", name)
		}
		if _, err := parseSelector(mapping.Selector); err != nil {
			return config{}, fmt.Errorf("fields.%s.selector: %w", name, err)
		}
	}
	if _, err := parseSelector(cfg.ItemSelector); err != nil {
		return config{}, fmt.Errorf("item_selector: %w", err)
	}
	return cfg, nil
}

func mapNode(root *node, fields map[string]fieldMapping, responseRequest *http.Request, fallbackURL string) ingestion.Candidate {
	value := func(name string) string {
		mapping, ok := fields[name]
		if !ok {
			return ""
		}
		targets := selectNodes(root, mapping.Selector)
		if mapping.Selector == "" {
			targets = []*node{root}
		}
		if len(targets) == 0 {
			return ""
		}
		target := targets[0]
		if mapping.Attribute != "" {
			return strings.TrimSpace(target.attrs[strings.ToLower(mapping.Attribute)])
		}
		if mapping.HTML {
			return innerHTML(target)
		}
		return textContent(target)
	}
	rawURL := value("url")
	if rawURL != "" {
		base := fallbackURL
		if responseRequest != nil && responseRequest.URL != nil {
			base = responseRequest.URL.String()
		}
		if parsedBase, err := url.Parse(base); err == nil {
			if reference, err := url.Parse(rawURL); err == nil {
				rawURL = parsedBase.ResolveReference(reference).String()
			}
		}
	}
	var publishedAt *time.Time
	if raw := value("published_at"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			publishedAt = &parsed
		}
	}
	return ingestion.Candidate{
		ExternalID:  value("id"),
		URL:         rawURL,
		Title:       value("title"),
		Author:      value("author"),
		Summary:     value("summary"),
		ContentHTML: value("content_html"),
		PublishedAt: publishedAt,
	}
}

func parseDocument(input string) (*node, error) {
	document := &node{tag: "#document", attrs: make(map[string]string)}
	current := document
	for len(input) > 0 {
		start := strings.IndexByte(input, '<')
		if start < 0 {
			appendText(current, input)
			break
		}
		appendText(current, input[:start])
		input = input[start:]
		if strings.HasPrefix(input, "<!--") {
			end := strings.Index(input, "-->")
			if end < 0 {
				return nil, fmt.Errorf("unterminated comment")
			}
			input = input[end+3:]
			continue
		}
		end := tagEnd(input)
		if end < 0 {
			return nil, fmt.Errorf("unterminated tag")
		}
		raw := strings.TrimSpace(input[1:end])
		input = input[end+1:]
		if raw == "" || strings.HasPrefix(raw, "!") || strings.HasPrefix(raw, "?") {
			continue
		}
		if strings.HasPrefix(raw, "/") {
			tag := strings.ToLower(strings.Fields(strings.TrimSpace(raw[1:]))[0])
			for current != document && current.tag != tag {
				current = current.parent
			}
			if current != document {
				current = current.parent
			}
			continue
		}
		selfClosing := strings.HasSuffix(raw, "/")
		if selfClosing {
			raw = strings.TrimSpace(strings.TrimSuffix(raw, "/"))
		}
		tag, attrs := parseStartTag(raw)
		if tag == "" {
			continue
		}
		child := &node{tag: tag, attrs: attrs, parent: current}
		current.children = append(current.children, child)
		if !selfClosing && !voidTag(tag) {
			current = child
		}
	}
	return document, nil
}

func tagEnd(input string) int {
	var quote byte
	for index := 1; index < len(input); index++ {
		char := input[index]
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
		} else if char == '>' {
			return index
		}
	}
	return -1
}

func parseStartTag(raw string) (string, map[string]string) {
	index := 0
	skipSpaces := func() {
		for index < len(raw) && isSpace(raw[index]) {
			index++
		}
	}
	readName := func() string {
		start := index
		for index < len(raw) && !isSpace(raw[index]) && raw[index] != '=' {
			index++
		}
		return strings.ToLower(raw[start:index])
	}
	skipSpaces()
	tag := readName()
	attrs := make(map[string]string)
	for index < len(raw) {
		skipSpaces()
		if index >= len(raw) {
			break
		}
		name := readName()
		skipSpaces()
		value := ""
		if index < len(raw) && raw[index] == '=' {
			index++
			skipSpaces()
			if index < len(raw) && (raw[index] == '\'' || raw[index] == '"') {
				quote := raw[index]
				index++
				start := index
				for index < len(raw) && raw[index] != quote {
					index++
				}
				value = raw[start:index]
				if index < len(raw) {
					index++
				}
			} else {
				start := index
				for index < len(raw) && !isSpace(raw[index]) {
					index++
				}
				value = raw[start:index]
			}
		}
		if name != "" {
			attrs[name] = stdhtml.UnescapeString(value)
		}
	}
	return tag, attrs
}

type simpleSelector struct {
	tag     string
	id      string
	classes []string
}

func parseSelector(selector string) ([]simpleSelector, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, nil
	}
	parts := strings.Fields(selector)
	result := make([]simpleSelector, 0, len(parts))
	for _, part := range parts {
		if strings.ContainsAny(part, ">+~[:") {
			return nil, fmt.Errorf("unsupported selector %q", part)
		}
		simple := simpleSelector{}
		for len(part) > 0 {
			index := strings.IndexAny(part, ".#")
			if index < 0 {
				if simple.tag == "" {
					simple.tag = strings.ToLower(part)
				}
				part = ""
				continue
			}
			if index > 0 && simple.tag == "" {
				simple.tag = strings.ToLower(part[:index])
			}
			marker := part[index]
			part = part[index+1:]
			next := strings.IndexAny(part, ".#")
			value := part
			if next >= 0 {
				value = part[:next]
				part = part[next:]
			} else {
				part = ""
			}
			if value == "" {
				return nil, fmt.Errorf("invalid selector")
			}
			if marker == '#' {
				simple.id = value
			} else {
				simple.classes = append(simple.classes, value)
			}
		}
		result = append(result, simple)
	}
	return result, nil
}

func selectNodes(root *node, selector string) []*node {
	parts, err := parseSelector(selector)
	if err != nil || len(parts) == 0 {
		return nil
	}
	current := []*node{root}
	for _, part := range parts {
		var matches []*node
		for _, candidate := range current {
			walk(candidate, func(descendant *node) {
				if descendant != candidate && part.matches(descendant) {
					matches = append(matches, descendant)
				}
			})
		}
		current = matches
	}
	return current
}

func (selector simpleSelector) matches(candidate *node) bool {
	if selector.tag != "" && candidate.tag != selector.tag {
		return false
	}
	if selector.id != "" && candidate.attrs["id"] != selector.id {
		return false
	}
	classes := strings.Fields(candidate.attrs["class"])
	for _, wanted := range selector.classes {
		found := false
		for _, class := range classes {
			if class == wanted {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func walk(root *node, visit func(*node)) {
	visit(root)
	for _, child := range root.children {
		walk(child, visit)
	}
}

func appendText(target *node, text string) {
	target.text += stdhtml.UnescapeString(text)
}

func textContent(target *node) string {
	var builder strings.Builder
	walk(target, func(current *node) {
		if current.text != "" {
			builder.WriteString(current.text)
			builder.WriteByte(' ')
		}
	})
	return strings.Join(strings.Fields(builder.String()), " ")
}

func innerHTML(target *node) string {
	var builder strings.Builder
	for _, child := range target.children {
		writeNode(&builder, child)
	}
	return builder.String()
}

func writeNode(builder *strings.Builder, target *node) {
	builder.WriteByte('<')
	builder.WriteString(target.tag)
	names := make([]string, 0, len(target.attrs))
	for name := range target.attrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		builder.WriteByte(' ')
		builder.WriteString(name)
		builder.WriteString(`="`)
		builder.WriteString(stdhtml.EscapeString(target.attrs[name]))
		builder.WriteByte('"')
	}
	builder.WriteByte('>')
	builder.WriteString(stdhtml.EscapeString(target.text))
	for _, child := range target.children {
		writeNode(builder, child)
	}
	if !voidTag(target.tag) {
		builder.WriteString("</")
		builder.WriteString(target.tag)
		builder.WriteByte('>')
	}
}

func isSpace(char byte) bool {
	return char == ' ' || char == '\n' || char == '\r' || char == '\t'
}

func voidTag(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}
