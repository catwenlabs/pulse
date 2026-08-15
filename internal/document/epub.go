package document

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// ParseEpub converts an epub file into a multi-chapter Document. Metadata
// comes from the OPF package (dc:identifier, dc:title, dc:creator); chapters
// follow spine order and hold the sanitized body content of each XHTML
// document. Chapter titles come from the first h1, falling back to the
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

	document := Document{
		Identifier: book.Identifier,
		Title:      book.Title,
		Author:     book.Author,
		Chapters:   []Chapter{},
	}
	for index, itemRef := range book.Spine {
		href, ok := book.Manifest[itemRef]
		if !ok {
			return Document{}, fmt.Errorf("spine references unknown manifest item %q", itemRef)
		}
		chapterFile, err := readZipEntry(reader, path.Join(baseDir, href))
		if err != nil {
			return Document{}, err
		}
		title, body, err := extractXHTMLBody(chapterFile, path.Join(baseDir, href))
		if err != nil {
			return Document{}, fmt.Errorf("extract chapter %q: %w", href, err)
		}
		document.Chapters = append(document.Chapters, Chapter{
			Index:       index,
			Title:       title,
			ContentHTML: body,
		})
	}
	if len(document.Chapters) == 0 {
		return Document{}, &ValidationError{Field: "file", Message: "epub spine is empty"}
	}
	return document, nil
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

type opfPackage struct {
	Identifier string
	Title      string
	Author     string
	Manifest   map[string]string
	Spine      []string
}

type opfXML struct {
	Metadata struct {
		Identifier []string `xml:"identifier"`
		Title      []string `xml:"title"`
		Creator    []string `xml:"creator"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
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
	book := opfPackage{Manifest: map[string]string{}}
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
		book.Manifest[item.ID] = item.Href
	}
	for _, ref := range parsed.Spine.ItemRefs {
		book.Spine = append(book.Spine, ref.IDRef)
	}
	return book, nil
}

func readZipEntry(reader *zip.Reader, name string) ([]byte, error) {
	entry, err := reader.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open epub entry %q: %w", name, err)
	}
	defer entry.Close()
	body, err := io.ReadAll(entry)
	if err != nil {
		return nil, fmt.Errorf("read epub entry %q: %w", name, err)
	}
	return body, nil
}

// extractXHTMLBody parses an XHTML chapter document and returns its title
// (first h1, falling back to head title) and the body's inner HTML. Relative
// img srcs are resolved to their canonical zip entry path.
func extractXHTMLBody(content []byte, chapterPath string) (string, string, error) {
	node, err := html.Parse(bytes.NewReader(content))
	if err != nil {
		return "", "", fmt.Errorf("parse XHTML: %w", err)
	}
	var find func(*html.Node) (*html.Node, *html.Node)
	find = func(current *html.Node) (*html.Node, *html.Node) {
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.ElementNode {
				continue
			}
			switch child.Data {
			case "h1":
				return child, nil
			case "body":
				return nil, child
			}
			if heading, body := find(child); heading != nil || body != nil {
				return heading, body
			}
		}
		return nil, nil
	}
	_, body := find(node)
	if body == nil {
		return "", "", &ValidationError{Field: "file", Message: "chapter has no body"}
	}
	sanitize(body)
	canonicalizeImageSources(body, chapterPath)

	var title string
	var h1 *html.Node
	if h1, _ = find(node); h1 != nil {
		title = nodeText(h1)
	} else if headTitle := findFirstTag(node, "title"); headTitle != nil {
		title = nodeText(headTitle)
	}

	var rendered bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&rendered, child); err != nil {
			return "", "", fmt.Errorf("render chapter body: %w", err)
		}
	}
	return strings.TrimSpace(title), strings.TrimSpace(rendered.String()), nil
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

// canonicalizeImageSources resolves relative img srcs against the chapter's
// zip entry path so every src is the canonical entry path (no ../ segments).
func canonicalizeImageSources(parent *html.Node, chapterPath string) {
	chapterDir := path.Dir(chapterPath)
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && current.Data == "img" {
			for index, attr := range current.Attr {
				if attr.Key != "src" && attr.Key != "xlink:href" {
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
