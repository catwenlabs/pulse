package document

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func buildTestEpub(t *testing.T) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	files := map[string]string{
		"mimetype": "application/epub+zip",
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">urn:uuid:test-book</dc:identifier>
    <dc:title>测试之书</dc:title>
    <dc:creator>测试作者</dc:creator>
  </metadata>
  <manifest>
    <item id="c1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="c1"/>
    <itemref idref="c2"/>
  </spine>
</package>`,
		"OEBPS/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>第一章</title></head>
<body><h1>第一章</h1><p>第一章内容。</p></body></html>`,
		"OEBPS/chapter2.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>第二章</title></head>
<body><h1>第二章</h1><p>第二章内容。</p></body></html>`,
	}
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

func TestParseEpubBuildsChaptersFromSpine(t *testing.T) {
	parsed, err := ParseEpub(buildTestEpub(t))
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if parsed.Identifier != "urn:uuid:test-book" {
		t.Errorf("Identifier = %q, want urn:uuid:test-book", parsed.Identifier)
	}
	if parsed.Title != "测试之书" {
		t.Errorf("Title = %q, want 测试之书", parsed.Title)
	}
	if parsed.Author != "测试作者" {
		t.Errorf("Author = %q, want 测试作者", parsed.Author)
	}
	if len(parsed.Chapters) != 2 {
		t.Fatalf("chapters = %d, want 2 (spine order)", len(parsed.Chapters))
	}
	first := parsed.Chapters[0]
	if first.Index != 0 || first.Title != "第一章" {
		t.Errorf("chapters[0] = index %d title %q, want 0 第一章", first.Index, first.Title)
	}
	if !bytes.Contains([]byte(first.ContentHTML), []byte("<p>第一章内容。</p>")) {
		t.Errorf("chapters[0] ContentHTML = %q, want body paragraph", first.ContentHTML)
	}
	if bytes.Contains([]byte(first.ContentHTML), []byte("<html")) || bytes.Contains([]byte(first.ContentHTML), []byte("<body")) {
		t.Errorf("chapters[0] ContentHTML = %q, must not keep document wrapper", first.ContentHTML)
	}
	if parsed.Chapters[1].Index != 1 || parsed.Chapters[1].Title != "第二章" {
		t.Errorf("chapters[1] = %+v, want index 1 第二章", parsed.Chapters[1])
	}
}

func TestParseEpubCanonicalizesImageSources(t *testing.T) {
	png := "\x89PNG\r\n\x1a\nfake-image-bytes"
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>图文之书</dc:title></metadata>
  <manifest>
    <item id="c1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/></spine>
</package>`,
		"OEBPS/chapter1.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>图</h1>
<p><img src="../images/pic.png" alt="插图"/></p></body></html>`,
	}
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	image, err := writer.Create("images/pic.png")
	if err != nil {
		t.Fatalf("create image entry: %v", err)
	}
	if _, err := image.Write([]byte(png)); err != nil {
		t.Fatalf("write image entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	parsed, err := ParseEpub(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	content := parsed.Chapters[0].ContentHTML
	if !strings.Contains(content, `src="images/pic.png"`) {
		t.Errorf("ContentHTML = %q, want img src canonicalized to the zip entry path", content)
	}
}

func TestParseEpubCanonicalizesSVGImageSources(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>封面之书</dc:title></metadata>
  <manifest>
    <item id="c1" href="Text/titlepage.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/></spine>
</package>`,
		"OEBPS/Text/titlepage.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml"><body>
<div><svg viewBox="0 0 100 100"><image width="100" height="100" xlink:href="../Images/cover.png"/></svg></div>
</body></html>`,
	}
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	parsed, err := ParseEpub(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	content := parsed.Chapters[0].ContentHTML
	if !strings.Contains(content, `xlink:href="OEBPS/Images/cover.png"`) {
		t.Errorf("ContentHTML = %q, want svg image href canonicalized to the zip entry path", content)
	}
}

func TestParseEpubSanitizesChapterContent(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	entries := map[string]string{
		"mimetype": "application/epub+zip",
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>危险之书</dc:title>
  </metadata>
  <manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c1"/></spine>
</package>`,
		"c1.xhtml": `<?xml version="1.0"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>坏章节</title>
<style>body { display: none }</style></head>
<body>
<h1>坏章节</h1>
<p onclick="steal()">普通段落。</p>
<script>alert(1)</script>
<a href="javascript:evil()">链接</a>
</body></html>`,
	}
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	parsed, err := ParseEpub(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	content := parsed.Chapters[0].ContentHTML
	for _, forbidden := range []string{"<script", "alert(1)", "<style", "onclick", "javascript:"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("ContentHTML contains forbidden %q: %s", forbidden, content)
		}
	}
	if !strings.Contains(content, "普通段落。") {
		t.Errorf("ContentHTML = %s, want paragraph text kept", content)
	}
	if !strings.Contains(content, "<a") {
		t.Errorf("ContentHTML = %s, want anchor element kept", content)
	}
}
