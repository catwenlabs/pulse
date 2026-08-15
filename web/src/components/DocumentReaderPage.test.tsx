import { act, fireEvent, render, screen } from '@testing-library/react'
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
