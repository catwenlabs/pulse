import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DocumentReaderPage } from './DocumentReaderPage'

const documentFixture = {
  id: 'doc-1',
  title: '测试之书',
  author: '作者',
  chapters: [
    { index: 0, title: '第一章', content_html: '<p>开头内容。</p>' },
    { index: 1, title: '第八章', content_html: '<p>该机制会导致复杂性上升。</p>' },
    { index: 2, title: '第九章', content_html: '<p>结尾。</p>' },
  ],
  progress: { chapter_index: 1, scroll_ratio: 0.5 },
}

function renderPage(fixture: unknown = documentFixture) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
    new Response(JSON.stringify(fixture), { status: 200 }),
  ))
  if (typeof Element.prototype.scrollIntoView !== 'function') {
    Element.prototype.scrollIntoView = vi.fn()
  }
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <DocumentReaderPage documentID="doc-1" />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('DocumentReaderPage', () => {
  it('renders all chapters with a chapter sidebar', async () => {
    renderPage()

    expect(await screen.findByRole('heading', { name: '测试之书' })).toBeTruthy()
    expect(screen.getByText('该机制会导致复杂性上升。')).toBeTruthy()
    expect(screen.getByText('结尾。')).toBeTruthy()

    const headings = screen.getAllByRole('button', { name: /第[一八九]章/ })
    expect(headings).toHaveLength(3)
  })

  it('links to the original file download', async () => {
    renderPage()

    await screen.findByRole('heading', { name: '测试之书' })
    const link = screen.getByRole('link', { name: '下载原件' })
    expect(link.getAttribute('href')).toBe('/api/v1/documents/doc-1/original')
  })

  it('restores the saved reading position', async () => {
    const scrollSpy = vi.fn()
    Element.prototype.scrollIntoView = scrollSpy
    renderPage()

    await screen.findByRole('heading', { name: '第八章' })
    await act(async () => {})
    expect(scrollSpy).toHaveBeenCalled()
  })

  it('jumps to a chapter from the sidebar', async () => {
    const scrollSpy = vi.fn()
    Element.prototype.scrollIntoView = scrollSpy
    renderPage()

    await screen.findByRole('heading', { name: '第九章' })
    scrollSpy.mockClear()
    fireEvent.click(screen.getByRole('button', { name: '第九章' }))

    expect(scrollSpy).toHaveBeenCalled()
  })

  it('persists progress on scroll', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(documentFixture), { status: 200 }))
      .mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    if (typeof Element.prototype.scrollIntoView !== 'function') {
      Element.prototype.scrollIntoView = vi.fn()
    }
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <DocumentReaderPage documentID="doc-1" />
      </QueryClientProvider>,
    )

    await screen.findByRole('heading', { name: '测试之书' })
    vi.useFakeTimers()
    fireEvent.scroll(document, { target: document.scrollingElement })
    await act(async () => { await vi.advanceTimersByTimeAsync(1500) })

    const progressCall = fetchMock.mock.calls.find((call) => String(call[0]).includes('/progress'))
    expect(progressCall).toBeTruthy()
    expect(progressCall?.[1].method).toBe('PUT')
    const payload = JSON.parse(String(progressCall?.[1].body))
    expect(payload.chapter_index).toBeGreaterThanOrEqual(0)
    expect(payload.scroll_ratio).toBeGreaterThanOrEqual(0)
    expect(payload.scroll_ratio).toBeLessThanOrEqual(1)
  })
})

describe('highlighting', () => {
  const notesFixture = [{
    id: 'note-1', document_id: 'doc-1', book_title: '测试之书',
    chapter_index: 1, highlight: '该机制', note: '呼应上一章', source: 'import',
  }]

  function mockReaderFetches(notes: unknown = notesFixture) {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes('/ai/tools')) return new Response('[]', { status: 200 })
      if (url.endsWith('/notes')) {
        if (init?.method === 'POST') {
          const body = JSON.parse(String(init.body))
          return new Response(JSON.stringify({
            id: 'note-9', document_id: 'doc-1', book_title: '测试之书', source: 'pulse', ...body,
          }), { status: 201 })
        }
        return new Response(JSON.stringify(notes), { status: 200 })
      }
      return new Response(JSON.stringify(documentFixture), { status: 200 })
    }))
    if (typeof Element.prototype.scrollIntoView !== 'function') {
      Element.prototype.scrollIntoView = vi.fn()
    }
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: false, media: query,
      addEventListener: () => {}, removeEventListener: () => {},
    }))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <DocumentReaderPage documentID="doc-1" />
      </QueryClientProvider>,
    )
  }

  function selectInChapter(chapter: number, text: string) {
    const content = document.querySelector(`section[data-chapter-index="${chapter}"] .document-content`)
    const range = { getBoundingClientRect: () => ({ top: 100, left: 100, width: 40, height: 20, right: 140, bottom: 120, x: 100, y: 100, toJSON: () => ({}) }) }
    vi.stubGlobal('getSelection', () => ({
      isCollapsed: false,
      toString: () => text,
      anchorNode: content?.firstChild ?? null,
      getRangeAt: () => range,
      removeAllRanges: () => {},
    }))
    act(() => { document.dispatchEvent(new Event('selectionchange')) })
  }

  it('lists existing notes in the notes panel', async () => {
    mockReaderFetches()
    await screen.findByRole('heading', { name: '测试之书' })

    await waitFor(() => expect(screen.getByRole('button', { name: /笔记/ }).textContent).toBe('笔记 (1)'))
    fireEvent.click(screen.getByRole('button', { name: '笔记 (1)' }))

    expect(await screen.findByText('呼应上一章')).toBeTruthy()
    expect(screen.getByText('第八章 · 导入')).toBeTruthy()
  })

  it('writes a highlight from the selection toolbar', async () => {
    mockReaderFetches()
    await screen.findByRole('heading', { name: '测试之书' })

    selectInChapter(1, '该机制')
    fireEvent.click(screen.getByRole('button', { name: '划线' }))

    const noteField = await screen.findByPlaceholderText('这条划线想记录什么？(可选)')
    fireEvent.change(noteField, { target: { value: '想法' } })
    fireEvent.click(screen.getByRole('button', { name: '保存划线' }))

    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some((call) => {
        if (!String(call[0]).endsWith('/notes') || call[1]?.method !== 'POST') return false
        const body = JSON.parse(String(call[1]?.body))
        return body.chapter_index === 1 && body.highlight === '该机制' && body.note === '想法'
      })).toBe(true)
    })

    fireEvent.click(screen.getByRole('button', { name: '笔记 (2)' }))
    expect(await screen.findByText('想法')).toBeTruthy()
    expect(screen.getByText('第八章 · Pulse')).toBeTruthy()
  })
})
