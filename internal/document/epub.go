package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// ParseEpub converts an epub file into a multi-chapter Document. Metadata
// comes from the OPF package (dc:identifier, dc:title, dc:creator); chapters
// follow spine order and hold the sanitized body content of each XHTML
// document. Chapter titles prefer the book's table of contents (the EPUB3
// nav document or the EPUB2 NCX), falling back to the first h1 and then the
// document title element. Image srcs are canonicalized to their zip entry
// paths so the asset endpoint can resolve them against the stored original.
func ParseEpub(content []byte) (Document, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return Document{}, &ValidationError{Field: "file", Message: "not a readable epub archive"}
	}
	opfPath, err := locateOPF(reader)
	if err != nil {
		return Document{}, err
	}
	packageFile, err := readZipEntry(reader, opfPath)
	if err != nil {
		return Document{}, err
	}
	book, err := parseOPF(packageFile)
	if err != nil {
		return Document{}, err
	}
	baseDir := path.Dir(opfPath)
	tocTitles := loadTocTitles(reader, book, baseDir)

	document := Document{
		Identifier: book.Identifier,
		Title:      book.Title,
		Author:     book.Author,
		Chapters:   []Chapter{},
	}
	chapterPaths := make([]chapterMeta, 0, len(book.Spine))
	for index, itemRef := range book.Spine {
		item, ok := book.Manifest[itemRef]
		if !ok {
			return Document{}, fmt.Errorf("spine references unknown manifest item %q", itemRef)
		}
		chapterPath := path.Join(baseDir, item.Href)
		chapterFile, err := readZipEntry(reader, chapterPath)
		if err != nil {
			return Document{}, err
		}
		title, fromHeading, body, err := extractXHTMLBody(chapterFile, chapterPath)
		if err != nil {
			return Document{}, fmt.Errorf("extract chapter %q: %w", item.Href, err)
		}
		titled := fromHeading
		if tocTitle := strings.TrimSpace(tocTitles[normalizeHref(chapterPath)]); tocTitle != "" {
			title = tocTitle
			titled = true
		}
		document.Chapters = append(document.Chapters, Chapter{
			Index:       index,
			Title:       title,
			ContentHTML: body,
		})
		chapterPaths = append(chapterPaths, chapterMeta{path: chapterPath, ownTitle: titled})
	}
	titleSplitContinuations(document.Chapters, chapterPaths)
	if len(document.Chapters) == 0 {
		return Document{}, &ValidationError{Field: "file", Message: "epub spine is empty"}
	}
	return document, nil
}

// titleSplitContinuations gives calibre's split continuation files
// (basename_split_001.html following basename_split_000.html) the previous
// file's title plus a continuation marker. Only files without their own
// title (neither TOC entry nor h1) inherit, and only from a previous file
// carrying a real title — junk head titles never propagate. The marker is
// not stacked, so a long split chain stays readable.
func titleSplitContinuations(chapters []Chapter, metas []chapterMeta) {
	const marker = "（续）"
	for index := 1; index < len(chapters); index++ {
		previous := metas[index-1]
		if metas[index].ownTitle || !previous.ownTitle || chapters[index-1].Title == "" {
			continue
		}
		previousStem, previousOK := splitStem(previous.path)
		currentStem, currentOK := splitStem(metas[index].path)
		if !previousOK || !currentOK || previousStem != currentStem {
			continue
		}
		chapters[index].Title = strings.TrimSuffix(chapters[index-1].Title, marker) + marker
		metas[index].ownTitle = true
	}
}

// splitStem reports the calibre split-family stem of a chapter path: for
// "text/part0005_split_003.html" it returns "text/part0005_split_".
func splitStem(chapterPath string) (string, bool) {
	stem := strings.TrimSuffix(chapterPath, path.Ext(chapterPath))
	trimmed := strings.TrimRight(stem, "0123456789")
	if trimmed == stem || !strings.HasSuffix(trimmed, "_split_") {
		return "", false
	}
	return trimmed, true
}

// ReadEpubEntry returns the raw bytes of one zip entry inside an epub file.
func ReadEpubEntry(content []byte, entry string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("open epub archive: %w", err)
	}
	return readZipEntry(reader, entry)
}

// ImageContentType reports the content type for image zip entries, or "" when
// the name is not a supported image.
func ImageContentType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

type opfItem struct {
	Href       string
	MediaType  string
	Properties string
}

// chapterMeta carries per-chapter parsing state alongside the Chapter value:
// the canonical zip entry path (used for TOC matching) and whether the title
// is the chapter's own (from the TOC or an h1 heading) rather than a head
// title fallback.
type chapterMeta struct {
	path     string
	ownTitle bool
}

type opfPackage struct {
	Identifier string
	Title      string
	Author     string
	Manifest   map[string]opfItem
	Spine      []string
	TocID      string
	// Nav and NCX hold the first manifest items of each TOC kind in
	// document order, so duplicate declarations resolve deterministically.
	Nav *opfItem
	NCX *opfItem
}

type opfXML struct {
	Metadata struct {
		Identifier []string `xml:"identifier"`
		Title      []string `xml:"title"`
		Creator    []string `xml:"creator"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Toc      string `xml:"toc,attr"`
		ItemRefs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// locateOPF reads META-INF/container.xml to find the OPF package path.
func locateOPF(reader *zip.Reader) (string, error) {
	container, err := readZipEntry(reader, "META-INF/container.xml")
	if err != nil {
		return "", &ValidationError{Field: "file", Message: "missing META-INF/container.xml"}
	}
	var rootFile struct {
		RootFiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	if err := xml.Unmarshal(container, &rootFile); err != nil {
		return "", fmt.Errorf("parse container.xml: %w", err)
	}
	if len(rootFile.RootFiles) == 0 || strings.TrimSpace(rootFile.RootFiles[0].FullPath) == "" {
		return "", &ValidationError{Field: "file", Message: "container.xml has no rootfile"}
	}
	return rootFile.RootFiles[0].FullPath, nil
}

func parseOPF(content []byte) (opfPackage, error) {
	var parsed opfXML
	if err := xml.Unmarshal(content, &parsed); err != nil {
		return opfPackage{}, &ValidationError{Field: "file", Message: "invalid OPF package"}
	}
	book := opfPackage{Manifest: map[string]opfItem{}}
	if len(parsed.Metadata.Identifier) > 0 {
		book.Identifier = strings.TrimSpace(parsed.Metadata.Identifier[0])
	}
	if len(parsed.Metadata.Title) > 0 {
		book.Title = strings.TrimSpace(parsed.Metadata.Title[0])
	}
	if len(parsed.Metadata.Creator) > 0 {
		book.Author = strings.TrimSpace(parsed.Metadata.Creator[0])
	}
	for _, item := range parsed.Manifest.Items {
		entry := opfItem{
			Href:       item.Href,
			MediaType:  item.MediaType,
			Properties: item.Properties,
		}
		if book.Nav == nil && hasProperty(entry.Properties, "nav") {
			book.Nav = &entry
		}
		if book.NCX == nil && entry.MediaType == "application/x-dtbncx+xml" {
			book.NCX = &entry
		}
		book.Manifest[item.ID] = entry
	}
	for _, ref := range parsed.Spine.ItemRefs {
		book.Spine = append(book.Spine, ref.IDRef)
	}
	book.TocID = parsed.Spine.Toc
	return book, nil
}

// loadTocTitles maps chapter zip entry paths to their table-of-contents
// labels. The EPUB3 nav document is preferred; otherwise the NCX referenced
// by the spine toc attribute (or by its media type) is parsed. Books with no
// usable TOC get an empty map and keep the heading heuristics.
func loadTocTitles(reader *zip.Reader, book opfPackage, baseDir string) map[string]string {
	if item := book.Nav; item != nil {
		navPath := path.Join(baseDir, item.Href)
		if content, err := readZipEntry(reader, navPath); err == nil {
			if titles := navTocTitles(content, path.Dir(navPath)); len(titles) > 0 {
				return titles
			}
		}
	}
	item, ok := book.ncxItem()
	if !ok {
		return nil
	}
	ncxPath := path.Join(baseDir, item.Href)
	if content, err := readZipEntry(reader, ncxPath); err == nil {
		if titles := ncxTocTitles(content, path.Dir(ncxPath)); len(titles) > 0 {
			return titles
		}
	}
	return nil
}

func hasProperty(list, name string) bool {
	for _, property := range strings.Fields(list) {
		if property == name {
			return true
		}
	}
	return false
}

// ncxItem finds the EPUB2 NCX: the manifest item the spine toc attribute
// points at, or the first item carrying the NCX media type.
func (book opfPackage) ncxItem() (opfItem, bool) {
	if book.TocID != "" {
		if item, ok := book.Manifest[book.TocID]; ok {
			return item, true
		}
	}
	if book.NCX != nil {
		return *book.NCX, true
	}
	return opfItem{}, false
}

type ncxNavPoint struct {
	Label   string `xml:"navLabel>text"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	NavPoints []ncxNavPoint `xml:"navPoint"`
}

// ncxTocTitles parses a toc.ncx navMap into chapter path → label entries.
func ncxTocTitles(content []byte, baseDir string) map[string]string {
	var parsed struct {
		NavMap struct {
			NavPoints []ncxNavPoint `xml:"navPoint"`
		} `xml:"navMap"`
	}
	if err := xml.Unmarshal(content, &parsed); err != nil {
		return nil
	}
	titles := map[string]string{}
	var walk func(points []ncxNavPoint)
	walk = func(points []ncxNavPoint) {
		for _, point := range points {
			addTocTitle(titles, baseDir, point.Content.Src, point.Label)
			walk(point.NavPoints)
		}
	}
	walk(parsed.NavMap.NavPoints)
	return titles
}

// navTocTitles parses an EPUB3 nav document's toc nav into chapter path →
// label entries.
func navTocTitles(content []byte, baseDir string) map[string]string {
	node, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return nil
	}
	nav := findTocNav(node)
	if nav == nil {
		return nil
	}
	titles := map[string]string{}
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && current.Data == "a" {
			for _, attr := range current.Attr {
				if attr.Key == "href" {
					addTocTitle(titles, baseDir, attr.Val, nodeText(current))
					break
				}
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(nav)
	return titles
}

// findTocNav returns the nav element typed epub:type="toc". An untyped
// fallback is trusted only when it is the document's sole nav element — the
// first nav of an untyped book is usually a landmarks or page-list nav whose
// labels must not override chapter titles.
func findTocNav(root *html.Node) *html.Node {
	var typed, only *html.Node
	navs := 0
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && current.Data == "nav" {
			navs++
			if only == nil {
				only = current
			}
			for _, attr := range current.Attr {
				if (attr.Key == "type" || attr.Key == "epub:type") && attr.Val == "toc" {
					typed = current
				}
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if typed != nil {
		return typed
	}
	if navs == 1 {
		return only
	}
	return nil
}

// addTocTitle records a TOC label for a chapter file. Fragments are ignored
// (one chapter per spine file), external targets are skipped, and the first
// entry wins — part openers precede their chapters in document order. Keys
// are percent-decoded because TOC hrefs and manifest hrefs may encode the
// same entry differently.
func addTocTitle(titles map[string]string, baseDir, href, label string) {
	target, _, _ := strings.Cut(href, "#")
	label = strings.TrimSpace(label)
	if target == "" || label == "" || strings.Contains(target, "://") {
		return
	}
	key := normalizeHref(path.Join(baseDir, target))
	if _, exists := titles[key]; !exists {
		titles[key] = label
	}
}

// normalizeHref percent-decodes a joined zip entry path when the decoding
// succeeds, so "text/chapter%201.xhtml" and "text/chapter 1.xhtml" compare
// equal. Undecodable paths (a stray % not followed by hex) pass through.
func normalizeHref(joined string) string {
	if decoded, err := url.PathUnescape(joined); err == nil {
		return decoded
	}
	return joined
}

// maxZipEntryBytes bounds one decompressed epub entry. Epubs are untrusted
// uploads and DEFLATE expands ~1000:1, so reading without a cap lets a small
// archive demand unbounded memory. 32MB comfortably exceeds any legitimate
// chapter or TOC document.
const maxZipEntryBytes = 32 << 20

func readZipEntry(reader *zip.Reader, name string) ([]byte, error) {
	entry, err := reader.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open epub entry %q: %w", name, err)
	}
	defer entry.Close()
	body, err := io.ReadAll(io.LimitReader(entry, maxZipEntryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read epub entry %q: %w", name, err)
	}
	if len(body) > maxZipEntryBytes {
		return nil, fmt.Errorf("read epub entry %q: entry exceeds %d bytes", name, maxZipEntryBytes)
	}
	return body, nil
}

// extractXHTMLBody parses an XHTML chapter document and returns its title,
// whether the title came from a real heading (h1) rather than the head title
// fallback, and the body's inner HTML. Relative img srcs are resolved to
// their canonical zip entry path.
func extractXHTMLBody(content []byte, chapterPath string) (string, bool, string, error) {
	node, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return "", false, "", fmt.Errorf("parse XHTML: %w", err)
	}
	var findBody func(*html.Node) *html.Node
	findBody = func(current *html.Node) *html.Node {
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode {
				if child.Data == "body" {
					return child
				}
				if body := findBody(child); body != nil {
					return body
				}
			}
		}
		return nil
	}
	body := findBody(node)
	if body == nil {
		return "", false, "", &ValidationError{Field: "file", Message: "chapter has no body"}
	}
	sanitize(body)
	canonicalizeImageSources(body, chapterPath)

	var title string
	fromHeading := false
	if h1 := findFirstTag(node, "h1"); h1 != nil {
		title = nodeText(h1)
		fromHeading = true
	} else if headTitle := findFirstTag(node, "title"); headTitle != nil {
		title = nodeText(headTitle)
	}

	var rendered bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&rendered, child); err != nil {
			return "", false, "", fmt.Errorf("render chapter body: %w", err)
		}
	}
	return strings.TrimSpace(title), fromHeading, strings.TrimSpace(rendered.String()), nil
}

// droppedElements are removed together with their content.
var droppedElements = map[string]bool{
	"script": true,
	"style":  true,
	"iframe": true,
	"object": true,
	"embed":  true,
}

// sanitize strips untrusted markup from a chapter body in place: script-like
// elements with their content, event-handler attributes, and javascript:
// URLs. Everything else is kept as-is; the reader applies its own styling.
func sanitize(parent *html.Node) {
	for child := parent.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == html.ElementNode {
			if droppedElements[child.Data] {
				parent.RemoveChild(child)
				child = next
				continue
			}
			kept := child.Attr[:0]
			for _, attr := range child.Attr {
				if !isUnsafeAttribute(attr) {
					kept = append(kept, attr)
				}
			}
			child.Attr = kept
		}
		sanitize(child)
		child = next
	}
}

func isUnsafeAttribute(attr html.Attribute) bool {
	if strings.HasPrefix(attr.Key, "on") {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(attr.Val), "javascript:")
}

// canonicalizeImageSources resolves relative img srcs (and SVG image hrefs,
// which calibre-style cover pages use) against the chapter's zip entry path
// so every reference is the canonical entry path (no ../ segments).
func canonicalizeImageSources(parent *html.Node, chapterPath string) {
	chapterDir := path.Dir(chapterPath)
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && (current.Data == "img" || current.Data == "image") {
			for index, attr := range current.Attr {
				// The SVG xlink:href attribute arrives with Key "href" (the
				// parser moves xlink into the attribute namespace).
				if attr.Key != "src" && attr.Key != "xlink:href" && attr.Key != "href" {
					continue
				}
				if strings.Contains(attr.Val, "://") {
					continue
				}
				current.Attr[index].Val = path.Join(chapterDir, attr.Val)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(parent)
}

func findFirstTag(current *html.Node, tag string) *html.Node {
	for child := current.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == tag {
			return child
		}
		if found := findFirstTag(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func nodeText(current *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(current)
	return strings.TrimSpace(builder.String())
}
