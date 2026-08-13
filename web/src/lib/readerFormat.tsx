export function scrollWithin(container: HTMLElement | null, element: HTMLElement) {
  if (!container) return
  container.scrollTo({
    top: container.scrollTop + element.getBoundingClientRect().top - container.getBoundingClientRect().top,
    behavior: 'smooth',
  })
}

// Walk up from an element to its nearest actually-scrollable ancestor. Used so
// expand/scroll works even when the parent doesn't hand the item an explicit
// scroll container (e.g. the digest, which scrolls inside the app shell's
// .main-content column rather than an element the page owns).
export function nearestScrollContainer(element: HTMLElement | null): HTMLElement | null {
  let node = element?.parentElement ?? null
  while (node) {
    const overflowY = window.getComputedStyle(node).overflowY
    if ((overflowY === 'auto' || overflowY === 'scroll') && node.scrollHeight > node.clientHeight) {
      return node
    }
    node = node.parentElement
  }
  return null
}


export function HighlightText({ text, query }: { text: string; query: string }) {
  if (!query) return text
  const index = text.toLocaleLowerCase().indexOf(query.toLocaleLowerCase())
  if (index < 0) return text
  const end = index + query.length
  return (
    <>
      {text.slice(0, index)}
      <mark className="rounded-sm bg-amber-200/70 px-0.5 text-inherit">{text.slice(index, end)}</mark>
      {text.slice(end)}
    </>
  )
}

export function compactTime(value: string): string {
  const date = new Date(value)
  const today = new Date()
  if (date.toDateString() === today.toDateString()) {
    return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit' }).format(date)
  }
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit' }).format(date)
}
