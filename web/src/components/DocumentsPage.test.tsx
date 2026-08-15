import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DocumentsPage } from './DocumentsPage'

function renderPage(onOpenDocument = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <DocumentsPage onOpenDocument={onOpenDocument} />
    </QueryClientProvider>,
  )
  return onOpenDocument
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DocumentsPage', () => {
  it('lists documents and opens one on click', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(
      JSON.stringify([
        { id: 'doc-1', title: '人类简史', author: '尤瓦尔·赫拉利', chapter_count: 12 },
        { id: 'doc-2', title: '笔记', chapter_count: 1 },
      ]),
      { status: 200 },
    )))
    const onOpen = renderPage()

    expect(await screen.findByText('人类简史')).toBeTruthy()
    expect(screen.getByText('尤瓦尔·赫拉利')).toBeTruthy()
    expect(screen.getByText('12 章')).toBeTruthy()

    fireEvent.click(screen.getByText('人类简史'))
    expect(onOpen).toHaveBeenCalledWith('doc-1')
  })

  it('uploads a file and refreshes the list', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('[]', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"id":"doc-9","title":"新书","chapters":[]}', { status: 201 }))
      .mockResolvedValueOnce(new Response('[{"id":"doc-9","title":"新书","chapter_count":3}]', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    renderPage()

    await screen.findByText('还没有导入任何文档')
    const input = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, { target: { files: [new File(['data'], 'new.epub')] } })

    expect(await screen.findByText('新书')).toBeTruthy()
    const uploadCall = fetchMock.mock.calls[1]
    expect(uploadCall[0]).toBe('/api/v1/documents')
    expect(uploadCall[1].method).toBe('POST')
    expect((uploadCall[1].body as FormData).get('file')).toBeTruthy()
  })

  it('shows the server message when the upload is a duplicate', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('[]', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"detail":"document already imported"}', { status: 409 }))
    vi.stubGlobal('fetch', fetchMock)
    renderPage()

    await screen.findByText('还没有导入任何文档')
    const input = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, { target: { files: [new File(['data'], 'dup.epub')] } })

    expect(await screen.findByText('document already imported')).toBeTruthy()
  })

  it('filters the list by the search box', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValue(new Response('[]', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    renderPage()

    await screen.findByText('还没有导入任何文档')
    fireEvent.change(screen.getByPlaceholderText('搜索书名、作者或章节内容'), { target: { value: '复杂性' } })

    await waitFor(() => {
      expect(fetchMock.mock.calls.some((call) => String(call[0]).includes('search=%E5%A4%8D%E6%9D%82%E6%80%A7'))).toBe(true)
    })
  })
})
