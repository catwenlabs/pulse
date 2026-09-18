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

func writeEpubZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
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

// ncx-style book mirroring the reported bug: chapter files carry a literal
// 未知 head title and no h1, while the NCX navMap holds the real labels.
func TestParseEpubUsesNcxTocTitles(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>目录之书</dc:title></metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="c1" href="text/part0006.html" media-type="application/xhtml+xml"/>
    <item id="c2" href="text/part0007.html" media-type="application/xhtml+xml"/>
    <item id="c3" href="text/part0008.html" media-type="application/xhtml+xml"/>
    <item id="c4" href="text/part0009.html" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="c1"/><itemref idref="c2"/><itemref idref="c3"/><itemref idref="c4"/>
  </spine>
</package>`,
		"toc.ncx": `<?xml version="1.0"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="num_1"><navLabel><text>第一部 1978～1983</text></navLabel>
      <content src="text/part0006.html#top"/>
      <navPoint id="num_2"><navLabel><text>1978 中国，回来了</text></navLabel>
        <content src="text/part0007.html#sigil_toc_id_1"/>
      </navPoint>
    </navPoint>
    <navPoint id="num_3"><navLabel><text>1980 告别浪漫的年代</text></navLabel>
      <content src="text/part0008.html"/>
    </navPoint>
  </navMap>
</ncx>`,
		"text/part0006.html": `<html><head><title>未知</title></head><body><h2>第一部 1978～1983</h2><p>部扉页。</p></body></html>`,
		"text/part0007.html": `<html><head><title>未知</title></head><body><h2>1978 中国，回来了</h2><p>正文。</p></body></html>`,
		"text/part0008.html": `<html><head><title>未知</title></head><body><p>无标题章节。</p></body></html>`,
		"text/part0009.html": `<html><head><title>未知</title></head><body><h1>后记</h1><p>目录未收录。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	want := []string{"第一部 1978～1983", "1978 中国，回来了", "1980 告别浪漫的年代", "后记"}
	if len(parsed.Chapters) != len(want) {
		t.Fatalf("chapters = %d, want %d", len(parsed.Chapters), len(want))
	}
	for index, title := range want {
		if got := parsed.Chapters[index].Title; got != title {
			t.Errorf("chapters[%d].Title = %q, want %q", index, got, title)
		}
	}
}

// EPUB3 nav document: the epub:type="toc" nav wins over landmarks and over
// the in-file h1 heading; nested entries resolve by fragment-stripped href.
func TestParseEpubUsesNavTocTitles(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>导航之书</dc:title></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
    <item id="c3" href="chapter3.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/><itemref idref="c3"/></spine>
</package>`,
		"OEBPS/nav.xhtml": `<?xml version="1.0"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body>
<nav epub:type="landmarks"><ol><li><a href="chapter1.xhtml">开始阅读</a></li></ol></nav>
<nav epub:type="toc"><ol>
  <li><a href="chapter1.xhtml">第一章 目录标题</a></li>
  <li><a href="chapter2.xhtml#section">第二章 带锚点</a>
    <ol><li><a href="chapter3.xhtml">嵌套小节</a></li></ol>
  </li>
</ol></nav>
</body></html>`,
		"OEBPS/chapter1.xhtml": `<html><head><title>第一章</title></head><body><h1>第一章 文件内标题</h1></body></html>`,
		"OEBPS/chapter2.xhtml": `<html><head><title>未知</title></head><body><h2>正文</h2></body></html>`,
		"OEBPS/chapter3.xhtml": `<html><head><title>未知</title></head><body><p>嵌套正文。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	want := []string{"第一章 目录标题", "第二章 带锚点", "嵌套小节"}
	if len(parsed.Chapters) != len(want) {
		t.Fatalf("chapters = %d, want %d", len(parsed.Chapters), len(want))
	}
	for index, title := range want {
		if got := parsed.Chapters[index].Title; got != title {
			t.Errorf("chapters[%d].Title = %q, want %q", index, got, title)
		}
	}
}

// Books without a usable TOC keep the h1 / head-title heuristic and still parse.
func TestParseEpubKeepsHeadingTitlesWithoutToc(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>无目录之书</dc:title></metadata>
  <manifest>
    <item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="c2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="missing"><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`,
		"c1.xhtml": `<html><head><title>未知</title></head><body><h1>开篇</h1></body></html>`,
		"c2.xhtml": `<html><head><title>尾声</title></head><body><p>结尾。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if parsed.Chapters[0].Title != "开篇" {
		t.Errorf("chapters[0].Title = %q, want 开篇 (h1)", parsed.Chapters[0].Title)
	}
	if parsed.Chapters[1].Title != "尾声" {
		t.Errorf("chapters[1].Title = %q, want 尾声 (head title)", parsed.Chapters[1].Title)
	}
}

// TOC entry hrefs resolve against the TOC file's own directory, so an NCX
// living deeper than the OPF still matches chapter zip entry paths.
func TestParseEpubResolvesTocPathsAgainstTocFileDirectory(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>分层之书</dc:title></metadata>
  <manifest>
    <item id="ncx" href="OEBPS/toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="c1" href="OEBPS/text/a.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx"><itemref idref="c1"/></spine>
</package>`,
		"OEBPS/toc.ncx": `<?xml version="1.0"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="n1"><navLabel><text>深处章节</text></navLabel>
      <content src="text/a.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`,
		"OEBPS/text/a.xhtml": `<html><head><title>未知</title></head><body><p>正文。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if parsed.Chapters[0].Title != "深处章节" {
		t.Errorf("chapters[0].Title = %q, want 深处章节", parsed.Chapters[0].Title)
	}
}

// calibre splits oversized chapters into basename_split_000.html,
// basename_split_001.html, ... and only the first file gets a TOC entry;
// continuation files inherit the previous file's title with a marker.
func TestParseEpubTitlesCalibreSplitContinuations(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>拆分之书</dc:title></metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="c1" href="text/part0005_split_000.html" media-type="application/xhtml+xml"/>
    <item id="c2" href="text/part0005_split_001.html" media-type="application/xhtml+xml"/>
    <item id="c3" href="text/part0005_split_002.html" media-type="application/xhtml+xml"/>
    <item id="c4" href="text/part0006.html" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx">
    <itemref idref="c1"/><itemref idref="c2"/><itemref idref="c3"/><itemref idref="c4"/>
  </spine>
</package>`,
		"toc.ncx": `<?xml version="1.0"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="n1"><navLabel><text>前言 我对历史的本质始终迷惑不解</text></navLabel>
      <content src="text/part0005_split_000.html#start"/>
    </navPoint>
    <navPoint id="n2"><navLabel><text>1978 中国，回来了</text></navLabel>
      <content src="text/part0006.html"/>
    </navPoint>
  </navMap>
</ncx>`,
		"text/part0005_split_000.html": `<html><head><title>未知</title></head><body><p>前半。</p></body></html>`,
		"text/part0005_split_001.html": `<html><head><title>未知</title></head><body><p>中段。</p></body></html>`,
		"text/part0005_split_002.html": `<html><head><title>未知</title></head><body><p>结尾。</p></body></html>`,
		"text/part0006.html":           `<html><head><title>未知</title></head><body><p>正文。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	want := []string{
		"前言 我对历史的本质始终迷惑不解",
		"前言 我对历史的本质始终迷惑不解（续）",
		"前言 我对历史的本质始终迷惑不解（续）",
		"1978 中国，回来了",
	}
	if len(parsed.Chapters) != len(want) {
		t.Fatalf("chapters = %d, want %d", len(parsed.Chapters), len(want))
	}
	for index, title := range want {
		if got := parsed.Chapters[index].Title; got != title {
			t.Errorf("chapters[%d].Title = %q, want %q", index, got, title)
		}
	}
}

// A sole untyped nav (generator omitted epub:type) is still trusted as the TOC.
func TestParseEpubUsesSoleUntypedNav(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>无类型之书</dc:title></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="nav"/><itemref idref="c1"/></spine>
</package>`,
		"nav.xhtml": `<html><body><nav><ol>
  <li><a href="c1.xhtml">唯一导航章节</a></li>
  <li><a href="https://example.com/elsewhere">外链不算目录</a></li>
</ol></nav></body></html>`,
		"c1.xhtml": `<html><head><title>未知</title></head><body><p>正文。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if got := parsed.Chapters[len(parsed.Chapters)-1].Title; got != "唯一导航章节" {
		t.Errorf("chapter title = %q, want 唯一导航章节 (external link skipped)", got)
	}
}

// Several untyped navs are ambiguous (the first is usually a landmarks nav);
// none of them may override chapter titles.
func TestParseEpubIgnoresUntypedNavsWhenSeveralExist(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>多导航之书</dc:title></metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="nav"/><itemref idref="c1"/></spine>
</package>`,
		"nav.xhtml": `<html><body>
<nav><ol><li><a href="c1.xhtml">开始阅读</a></li></ol></nav>
<nav><ol><li><a href="c1.xhtml">目录条目</a></li></ol></nav>
</body></html>`,
		"c1.xhtml": `<html><head><title>未知</title></head><body><h1>正文标题</h1></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if got := parsed.Chapters[len(parsed.Chapters)-1].Title; got != "正文标题" {
		t.Errorf("chapter title = %q, want 正文标题 (ambiguous navs ignored)", got)
	}
}

// Duplicate properties="nav" items resolve to the first in manifest document
// order, deterministically.
func TestParseEpubPicksFirstNavItemInDocumentOrder(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>重复之书</dc:title></metadata>
  <manifest>
    <item id="nav1" href="nav1.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="nav2" href="nav2.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/></spine>
</package>`,
		"nav1.xhtml": `<html><body><nav epub:type="toc"><ol><li><a href="c1.xhtml">第一导航</a></li></ol></nav></body></html>`,
		"nav2.xhtml": `<html><body><nav epub:type="toc"><ol><li><a href="c1.xhtml">第二导航</a></li></ol></nav></body></html>`,
		"c1.xhtml":   `<html><head><title>未知</title></head><body><p>正文。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if got := parsed.Chapters[0].Title; got != "第一导航" {
		t.Errorf("chapter title = %q, want 第一导航 (first nav item in document order)", got)
	}
}

// A malformed NCX must not fail the import; headings keep naming the chapters.
func TestParseEpubFallsBackWhenNcxMalformed(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>坏目录之书</dc:title></metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx"><itemref idref="c1"/></spine>
</package>`,
		"toc.ncx":  `<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/"><navMap>断裂的 XML`,
		"c1.xhtml": `<html><head><title>未知</title></head><body><h1>靠标题的章节</h1></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if got := parsed.Chapters[0].Title; got != "靠标题的章节" {
		t.Errorf("chapter title = %q, want 靠标题的章节 (malformed NCX ignored)", got)
	}
}

// TOC hrefs may percent-encode spaces while manifest hrefs do not (or vice
// versa); both forms must resolve to the same chapter.
func TestParseEpubMatchesPercentEncodedTocHrefs(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>空格之书</dc:title></metadata>
  <manifest>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="c1" href="text/chapter 1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine toc="ncx"><itemref idref="c1"/></spine>
</package>`,
		"toc.ncx": `<?xml version="1.0"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>
    <navPoint id="n1"><navLabel><text>带空格的章节</text></navLabel>
      <content src="text/chapter%201.xhtml"/>
    </navPoint>
  </navMap>
</ncx>`,
		"text/chapter 1.xhtml": `<html><head><title>未知</title></head><body><p>正文。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if got := parsed.Chapters[0].Title; got != "带空格的章节" {
		t.Errorf("chapter title = %q, want 带空格的章节 (percent-decoded href match)", got)
	}
}

// Split files in a book without any TOC keep their fallback titles as-is;
// junk head titles never propagate through the continuation chain.
func TestParseEpubDoesNotPropagateFallbackTitlesInSplits(t *testing.T) {
	epub := writeEpubZip(t, map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>无目录拆分之书</dc:title></metadata>
  <manifest>
    <item id="c1" href="text/part0001_split_000.html" media-type="application/xhtml+xml"/>
    <item id="c2" href="text/part0001_split_001.html" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/></spine>
</package>`,
		"text/part0001_split_000.html": `<html><head><title>未知</title></head><body><p>前半。</p></body></html>`,
		"text/part0001_split_001.html": `<html><head><title>未知</title></head><body><p>后半。</p></body></html>`,
	})

	parsed, err := ParseEpub(epub)
	if err != nil {
		t.Fatalf("ParseEpub() error = %v", err)
	}
	if got := parsed.Chapters[1].Title; got != "未知" {
		t.Errorf("chapters[1].Title = %q, want 未知 unchanged (no fallback propagation)", got)
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
