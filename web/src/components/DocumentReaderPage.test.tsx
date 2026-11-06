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

  it('rewrites chapter image references to the asset endpoint', async () => {
    renderPage({
      ...documentFixture,
      chapters: [
        { index: 0, title: '封面', content_html: '<div><svg viewBox="0 0 100 100"><image xlink:href="cover1.jpeg"/></svg></div>' },
        { index: 1, title: '第一章', content_html: '<p><img src="images/pic.png" alt="插图"/></p><p><img src="https://example.com/ext.png" alt="外链"/></p>' },
      ],
    })

    await screen.findByRole('heading', { name: '测试之书' })
    const inline = screen.getByRole('img', { name: '插图' })
    expect(inline.getAttribute('src')).toBe('/api/v1/documents/doc-1/asset/images/pic.png')
    const external = screen.getByRole('img', { name: '外链' })
    expect(external.getAttribute('src')).toBe('https://example.com/ext.png')
    const svgImage = document.querySelector('image')
    expect(svgImage?.getAttribute('xlink:href')).toBe('/api/v1/documents/doc-1/asset/cover1.jpeg')
  })

  it('restores the saved reading position inside the chapter', async () => {
    // Saved progress: chapter 1 at scroll_ratio 0.5. Mock geometry so the
    // restore math is observable: container top 0, chapter-1 section top
    // 1000 height 400 → scrollTop must land at 1000 + 0.5*400 - 80 = 1120.
    const rect = (top: number, height: number) => ({
      top, height, bottom: top + height, left: 0, right: 0, width: 0, x: 0, y: 0, toJSON: () => ({}),
    })
    const rectSpy = vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
      const element = this as HTMLElement
      if (element.dataset?.testid === 'reader-scroll') return rect(0, 800)
      if (element.dataset?.chapterIndex === '1') return rect(1000, 400)
      return rect(0, 0)
    })
    renderPage()

    await screen.findByRole('heading', { name: '第八章' })
    await act(async () => {})
    const container = screen.getByTestId('reader-scroll') as HTMLElement
    expect(container.scrollTop).toBe(1120)
    rectSpy.mockRestore()
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

  it('applies typography settings to chapter content', async () => {
    localStorage.clear()
    renderPage()

    await screen.findByRole('heading', { name: '测试之书' })
    const content = document.querySelector('.document-content') as HTMLElement
    expect(content.style.fontSize).toBe('17px')

    fireEvent.click(screen.getByRole('button', { name: '排版设置' }))
    expect(screen.getByRole('dialog', { name: '排版设置' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: '增大字号' }))
    expect(content.style.fontSize).toBe('18px')
    fireEvent.click(screen.getByRole('button', { name: '黑体' }))
    expect(content.style.fontFamily).toBe('var(--font-sans)')
    expect(JSON.parse(String(localStorage.getItem('pulse.reader-typography'))).fontSize).toBe(18)

    fireEvent.pointerDown(document.body)
    expect(screen.queryByRole('dialog', { name: '排版设置' })).toBeNull()
    localStorage.clear()
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
    fireEvent.scroll(screen.getByTestId('reader-scroll'))
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
      if (url.includes('/api/v1/notes/') && init?.method === 'PUT') {
        const body = JSON.parse(String(init.body))
        return new Response(JSON.stringify({
          id: url.split('/').pop(), document_id: 'doc-1', book_title: '测试之书',
          chapter_index: 1, highlight: '该机制', source: 'pulse', ...body,
        }), { status: 200 })
      }
      if (url.includes('/api/v1/notes/') && init?.method === 'DELETE') {
        return new Response(null, { status: 204 })
      }
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

  it('paints saved highlights onto the chapter text', async () => {
    mockReaderFetches()
    await screen.findByRole('heading', { name: '测试之书' })

    await waitFor(() => {
      const mark = document.querySelector('section[data-chapter-index="1"] mark.document-highlight')
      expect(mark?.textContent).toBe('该机制')
    })
  })

  it('saves a highlight instantly and adds the note afterwards', async () => {
    mockReaderFetches()
    await screen.findByRole('heading', { name: '测试之书' })

    selectInChapter(1, '复杂性')
    fireEvent.click(screen.getByRole('button', { name: '划线' }))

    // Instant save: the POST fires without any intermediate dialog.
    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some((call) => {
        if (!String(call[0]).endsWith('/notes') || call[1]?.method !== 'POST') return false
        const body = JSON.parse(String(call[1]?.body))
        return body.chapter_index === 1 && body.highlight === '复杂性'
      })).toBe(true)
    })
    expect(await screen.findByRole('status', { name: '已添加划线' })).toBeTruthy()
    await waitFor(() => {
      const mark = document.querySelector('section[data-chapter-index="1"] mark.document-highlight')
      expect(mark?.textContent).toBe('该机制')
    })

    // The note is added afterwards from the notice.
    fireEvent.click(screen.getByRole('button', { name: '添加笔记' }))
    const noteField = await screen.findByPlaceholderText('这条划线想记录什么？')
    fireEvent.change(noteField, { target: { value: '想法' } })
    fireEvent.click(screen.getByRole('button', { name: '保存笔记' }))

    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some((call) =>
        String(call[0]).endsWith('/api/v1/notes/note-9') && call[1]?.method === 'PUT')).toBe(true)
    })

    fireEvent.click(screen.getByRole('button', { name: '笔记 (2)' }))
    expect(await screen.findByText('想法')).toBeTruthy()
    expect(screen.getByText('第八章 · Pulse')).toBeTruthy()
  })

  it('does not create a duplicate note for an already highlighted passage', async () => {
    mockReaderFetches()
    await screen.findByRole('heading', { name: '测试之书' })
    // Wait until the fixture note has loaded before selecting it again.
    await waitFor(() => expect(screen.getByRole('button', { name: /笔记/ }).textContent).toBe('笔记 (1)'))

    // Chapter 1 already carries the fixture note '该机制'.
    selectInChapter(1, '该机制')
    fireEvent.click(screen.getByRole('button', { name: '划线' }))

    expect(await screen.findByRole('status', { name: '已添加划线' })).toBeTruthy()
    const posted = vi.mocked(fetch).mock.calls.filter((call) =>
      String(call[0]).endsWith('/notes') && call[1]?.method === 'POST')
    expect(posted).toHaveLength(0)
    fireEvent.click(await screen.findByRole('button', { name: '笔记 (1)' }))
    expect(await screen.findByText('呼应上一章')).toBeTruthy()
  })

  it('removes a highlight by re-selecting it and choosing 取消划线', async () => {
    mockReaderFetches()
    await screen.findByRole('heading', { name: '测试之书' })

    await waitFor(() => {
      expect(document.querySelector('section[data-chapter-index="1"] mark.document-highlight')).toBeTruthy()
    })
    const mark = document.querySelector('section[data-chapter-index="1"] mark.document-highlight') as HTMLElement
    const textNode = mark.firstChild as Text
    const range = {
      getBoundingClientRect: () => ({ top: 100, left: 100, width: 40, height: 20, right: 140, bottom: 120, x: 100, y: 100, toJSON: () => ({}) }),
      startContainer: textNode,
      endContainer: textNode,
    }
    vi.stubGlobal('getSelection', () => ({
      isCollapsed: false,
      toString: () => '该机制',
      anchorNode: textNode,
      getRangeAt: () => range,
      removeAllRanges: () => {},
    }))
    act(() => { document.dispatchEvent(new Event('selectionchange')) })

    fireEvent.click(screen.getByRole('button', { name: '取消划线' }))
    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some((call) =>
        String(call[0]).endsWith('/api/v1/notes/note-1') && call[1]?.method === 'DELETE')).toBe(true)
    })
    await waitFor(() => {
      expect(document.querySelector('section[data-chapter-index="1"] mark.document-highlight')).toBeNull()
    })
  })
})
