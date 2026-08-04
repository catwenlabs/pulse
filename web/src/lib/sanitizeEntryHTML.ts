export function sanitizeEntryHTML(value: string, baseURL?: string): string {
  const document = new DOMParser().parseFromString(value, 'text/html')
  const blocked = 'script,style,iframe,object,embed,form,input,button,textarea,select,link,meta,base,svg,math'
  document.body.querySelectorAll(blocked).forEach((element) => element.remove())

  const allowed = new Set([
    'A', 'B', 'BLOCKQUOTE', 'BR', 'CODE', 'DEL', 'DIV', 'EM', 'FIGCAPTION', 'FIGURE',
    'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'HR', 'I', 'IMG', 'LI', 'OL', 'P', 'PRE',
    'S', 'SPAN', 'STRONG', 'SUB', 'SUP', 'TABLE', 'TBODY', 'TD', 'TFOOT', 'TH',
    'THEAD', 'TR', 'U', 'UL',
  ])

  Array.from(document.body.querySelectorAll('*')).forEach((element) => {
    if (!allowed.has(element.tagName)) {
      element.replaceWith(...Array.from(element.childNodes))
      return
    }

    const href = element.tagName === 'A' ? safeContentURL(element.getAttribute('href'), baseURL) : ''
    const src = element.tagName === 'IMG' ? safeContentURL(element.getAttribute('src'), baseURL) : ''
    const alt = element.tagName === 'IMG' ? element.getAttribute('alt') || '' : ''
    element.getAttributeNames().forEach((name) => element.removeAttribute(name))

    if (element.tagName === 'A' && href) {
      element.setAttribute('href', href)
      element.setAttribute('target', '_blank')
      element.setAttribute('rel', 'noopener noreferrer')
    }
    if (element.tagName === 'IMG' && src) {
      element.setAttribute('src', src)
      element.setAttribute('alt', alt)
      element.setAttribute('loading', 'lazy')
      element.setAttribute('decoding', 'async')
      element.setAttribute('referrerpolicy', 'no-referrer')
    } else if (element.tagName === 'IMG') {
      element.remove()
    }
  })

  return document.body.innerHTML
}

function safeContentURL(value: string | null, baseURL?: string): string {
  if (!value) return ''
  try {
    const origin = typeof window === 'undefined' ? 'http://localhost' : window.location.origin
    const url = new URL(value, baseURL || origin)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : ''
  } catch {
    return ''
  }
}
