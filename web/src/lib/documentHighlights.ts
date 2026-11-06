// Applies saved highlight passages onto chapter HTML. Notes store
// the selected text (from Selection.toString()), which can contain newlines
// and indentation that differ from the underlying HTML text nodes, so
// matching runs on a whitespace-normalized index built over the parsed text
// nodes and maps back onto them to wrap <mark> elements. Each note colors its
// first occurrence; passages that no longer match the text are skipped.
// highlightContentHtml bakes the marks into the HTML string before React
// renders it: React 19 resets innerHTML on re-renders, so marks painted onto
// live DOM after the fact would be wiped by the next unrelated state update.
// Marks carry data-note-id so the reader can offer 取消划线 when the user
// re-selects highlighted text.

const HIGHLIGHT_CLASS = 'document-highlight'
const WHITESPACE = /\s/

interface CharEntry {
  node: Text
  offset: number
}

interface NormalizedText {
  text: string
  entries: CharEntry[]
}

// HighlightPassage pairs one note with its highlighted text. Plain strings
// stay accepted for callers that only need the visual mark.
export interface HighlightPassage {
  id: string
  highlight: string
}

export function normalizePassage(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

type HighlightRoot = HTMLElement | DocumentFragment

// applyDocumentHighlights is idempotent: it first unwraps existing highlight
// marks, then re-applies every passage, so re-running with the same or a
// changed note list always leaves a single layer of marks.
export function applyDocumentHighlights(root: HighlightRoot, passages: Array<string | HighlightPassage>): void {
  unwrapHighlights(root)
  for (const passage of dedupePassages(passages)) {
    applyPassage(root, passage)
  }
}

// dedupePassages drops empty passages and collapses duplicates by normalized
// text, keeping the first note id.
function dedupePassages(passages: Array<string | HighlightPassage>): HighlightPassage[] {
  const seen = new Set<string>()
  const result: HighlightPassage[] = []
  for (const passage of passages) {
    const entry: HighlightPassage = typeof passage === 'string' ? { id: '', highlight: passage } : passage
    const needle = normalizePassage(entry.highlight)
    if (needle === '' || seen.has(needle)) continue
    seen.add(needle)
    result.push({ id: entry.id, highlight: needle })
  }
  return result
}

function unwrapHighlights(root: HighlightRoot): void {
  const marks = Array.from(root.querySelectorAll<HTMLElement>(`mark.${HIGHLIGHT_CLASS}`))
  for (const mark of marks) {
    const parent = mark.parentNode
    if (!parent) continue
    while (mark.firstChild) parent.insertBefore(mark.firstChild, mark)
    parent.removeChild(mark)
    parent.normalize()
  }
}

function applyPassage(root: HighlightRoot, passage: HighlightPassage): void {
  const { text, entries } = buildNormalizedText(root)
  const start = text.indexOf(passage.highlight)
  if (start < 0) return
  // Group matched characters by text node. Within one node the matched
  // characters are contiguous, so a min/max range per node is exact.
  const groups = new Map<Text, { start: number; end: number }>()
  for (let index = start; index < start + passage.highlight.length; index++) {
    const entry = entries[index]
    const group = groups.get(entry.node)
    if (group) {
      group.start = Math.min(group.start, entry.offset)
      group.end = Math.max(group.end, entry.offset + 1)
    } else {
      groups.set(entry.node, { start: entry.offset, end: entry.offset + 1 })
    }
  }
  for (const [node, range] of groups) wrapNodeRange(node, range.start, range.end, passage.id)
}

// buildNormalizedText walks every text node under root and emits one
// character per non-whitespace code point plus single spaces for whitespace
// runs, recording which original node and offset each emitted character came
// from.
function buildNormalizedText(root: HighlightRoot): NormalizedText {
  const walker = root.ownerDocument.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const chars: string[] = []
  const entries: CharEntry[] = []
  let pendingSpace = false
  let seenText = false
  let node = walker.nextNode()
  while (node !== null) {
    const text = node as Text
    for (let offset = 0; offset < text.length; offset++) {
      const value = text.data.charAt(offset)
      if (WHITESPACE.test(value)) {
        pendingSpace = true
        continue
      }
      if (seenText && pendingSpace) {
        // The emitted space maps onto the character that follows the run, so
        // it never widens a node's matched range beyond its own characters.
        chars.push(' ')
        entries.push({ node: text, offset })
      }
      pendingSpace = false
      seenText = true
      chars.push(value)
      entries.push({ node: text, offset })
    }
    node = walker.nextNode()
  }
  return { text: chars.join(''), entries }
}

// wrapNodeRange marks [start, end) inside one text node, splitting off any
// unmarked head and tail first.
function wrapNodeRange(node: Text, start: number, end: number, noteId: string): void {
  let target = node
  if (end < target.length) target.splitText(end)
  if (start > 0) target = target.splitText(start)
  const mark = node.ownerDocument.createElement('mark')
  mark.className = HIGHLIGHT_CLASS
  if (noteId !== '') mark.dataset.noteId = noteId
  target.replaceWith(mark)
  mark.append(target)
}

// highlightContentHtml returns chapter HTML with every passage wrapped in a
// highlight mark. The chapter is parsed in an inert <template> so nothing
// executes, marks are applied to the detached tree, and the result is
// serialized back for dangerouslySetInnerHTML.
export function highlightContentHtml(html: string, passages: Array<string | HighlightPassage>): string {
  const unique = dedupePassages(passages)
  if (unique.length === 0) return html
  const template = document.createElement('template')
  template.innerHTML = html
  applyDocumentHighlights(template.content, unique)
  return template.innerHTML
}
