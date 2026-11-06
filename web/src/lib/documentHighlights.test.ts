import { describe, expect, it } from 'vitest'

import { applyDocumentHighlights, highlightContentHtml } from './documentHighlights'

function mount(html: string): HTMLElement {
  const root = document.createElement('div')
  root.innerHTML = html
  document.body.append(root)
  return root
}

function markedTexts(root: HTMLElement): string[] {
  return Array.from(root.querySelectorAll('mark.document-highlight')).map((mark) => mark.textContent)
}

describe('applyDocumentHighlights', () => {
  it('wraps a passage inside one paragraph', () => {
    const root = mount('<p>该机制会导致复杂性上升。</p>')

    applyDocumentHighlights(root, ['该机制'])

    expect(markedTexts(root)).toEqual(['该机制'])
    expect(root.textContent).toBe('该机制会导致复杂性上升。')
  })

  it('matches a passage starting mid-node and splits the unmarked head and tail', () => {
    const root = mount('<p>开头内容。划线目标在中间出现。</p>')

    applyDocumentHighlights(root, ['划线目标'])

    expect(markedTexts(root)).toEqual(['划线目标'])
    const paragraph = root.querySelector('p') as HTMLElement
    expect(paragraph.childNodes[0].textContent).toBe('开头内容。')
    expect(paragraph.lastChild?.textContent).toBe('在中间出现。')
  })

  it('matches across paragraphs when the passage contains selection newlines', () => {
    const root = mount('<p>第一段内容</p>\n<p>第二段内容</p>')

    applyDocumentHighlights(root, ['第一段内容\n\n第二段内容'])

    expect(markedTexts(root)).toEqual(['第一段内容', '第二段内容'])
  })

  it('matches text split across inline elements', () => {
    const root = mount('<p>今天<span>天气</span>不错</p>')

    applyDocumentHighlights(root, ['天气不错'])

    expect(markedTexts(root)).toEqual(['天气', '不错'])
  })

  it('skips passages that do not occur in the chapter', () => {
    const root = mount('<p>正文</p>')

    applyDocumentHighlights(root, ['不存在的句子'])

    expect(markedTexts(root)).toEqual([])
    expect(root.innerHTML).toBe('<p>正文</p>')
  })

  it('collapses internal whitespace runs when matching', () => {
    const root = mount('<p>词一\n\t词二</p>')

    applyDocumentHighlights(root, ['词一 词二'])

    expect(markedTexts(root)).toEqual(['词一\n\t词二'])
  })

  it('is idempotent: reapplying the same passages keeps one layer of marks', () => {
    const root = mount('<p>同一段文字</p>')

    applyDocumentHighlights(root, ['同一段文字'])
    applyDocumentHighlights(root, ['同一段文字'])

    expect(markedTexts(root)).toEqual(['同一段文字'])
    expect(root.querySelectorAll('mark').length).toBe(1)
  })

  it('unwraps marks for passages that are no longer highlighted', () => {
    const root = mount('<p>甲句子。乙句子。</p>')

    applyDocumentHighlights(root, ['甲句子。', '乙句子。'])
    expect(markedTexts(root)).toEqual(['甲句子。', '乙句子。'])

    applyDocumentHighlights(root, ['乙句子。'])
    expect(markedTexts(root)).toEqual(['乙句子。'])
    expect(root.querySelectorAll('mark').length).toBe(1)
    expect(root.textContent).toBe('甲句子。乙句子。')
  })
})

describe('highlightContentHtml', () => {
  it('returns chapter HTML with passages wrapped in highlight marks', () => {
    const html = highlightContentHtml('<p>该机制会导致复杂性上升。</p>', ['该机制'])

    const root = mount(html)
    expect(markedTexts(root)).toEqual(['该机制'])
    expect(root.textContent).toBe('该机制会导致复杂性上升。')
  })

  it('stamps marks with the note id for 取消划线', () => {
    const html = highlightContentHtml('<p>该机制会导致复杂性上升。</p>', [{ id: 'note-1', highlight: '该机制' }])

    const root = mount(html)
    const mark = root.querySelector('mark.document-highlight') as HTMLElement
    expect(mark.dataset.noteId).toBe('note-1')
  })

  it('returns the input untouched when there is nothing to highlight', () => {
    const html = '<p>正文</p>'

    expect(highlightContentHtml(html, [])).toBe(html)
    expect(highlightContentHtml(html, ['不存在的句子'])).toBe('<p>正文</p>')
  })

  it('highlights each passage once regardless of duplicates', () => {
    const html = highlightContentHtml('<p>同一段文字</p>', ['同一段文字', '同一段文字'])

    const root = mount(html)
    expect(markedTexts(root)).toEqual(['同一段文字'])
  })
})
