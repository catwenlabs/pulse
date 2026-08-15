// Chapter HTML stores image references as canonical entry paths inside the
// imported file (see the epub parser's canonicalizeImageSources). The browser
// cannot resolve those against the page URL, so the reader rewrites every
// relative reference to the document's asset endpoint before rendering.
// Absolute URLs (http/https/data) are left untouched.

function resolveReference(value: string | null, prefix: string): string | null {
  if (!value || value.includes('://') || value.startsWith('data:') || value.startsWith('/')) {
    return null
  }
  return prefix + value
}

export function resolveDocumentAssets(html: string, documentID: string): string {
  if (!html.includes('<img') && !html.includes('<image')) return html
  if (typeof DOMParser === 'undefined') return html
  const parsed = new DOMParser().parseFromString(html, 'text/html')
  const prefix = `/api/v1/documents/${documentID}/asset/`
  for (const image of parsed.querySelectorAll('img[src]')) {
    const next = resolveReference(image.getAttribute('src'), prefix)
    if (next) image.setAttribute('src', next)
  }
  // Calibre-style cover pages embed the cover as <svg><image xlink:href=...>.
  for (const image of parsed.querySelectorAll('image')) {
    for (const attribute of ['xlink:href', 'href']) {
      const next = resolveReference(image.getAttribute(attribute), prefix)
      if (next) image.setAttribute(attribute, next)
    }
  }
  return parsed.body.innerHTML
}
