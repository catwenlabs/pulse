import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { NotesHubPage } from './NotesHubPage'

const notesFixture = [
  { id: 'note-1', document_id: 'doc-1', book_title: '人类简史', book_author: '尤瓦尔·赫拉利', chapter_index: 3, highlight: '高亮一', note: '想法一', source: 'pulse' },
  { id: 'note-2', document_id: 'doc-1', book_title: '人类简史', book_author: '尤瓦尔·赫拉利', chapter_index: 4, highlight: '高亮二', source: 'import' },
  { id: 'note-3', book_title: '没导入的书', book_author: '某作者', chapter_index: 0, location: 'loc-9', highlight: '散落的高亮', source: 'import' },
]

const documentsFixture = [
  { id: 'doc-1', title: '人类简史', author: '尤瓦尔·赫拉利', chapter_count: 12 },
  { id: 'doc-2', title: '另一本书', chapter_count: 3 },
]

function mockFetches(notes: unknown = notesFixture) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    if (url.startsWith('/api/v1/notes')) {
      return new Response(JSON.stringify(notes), { status: 200 })
    }
    if (url.endsWith('/link')) {
      return new Response(null, { status: 204 })
    }
    if (url.endsWith('/documents/notes/import')) {
      return new Response('{"imported":5,"unmatched":2}', { status: 200 })
    }
    if (url.startsWith('/api/v1/documents')) {
      return new Response(JSON.stringify(documentsFixture), { status: 200 })
    }
    return new Response('[]', { status: 200 })
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderPage(fetchMock = mockFetches()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <NotesHubPage onOpenDocument={vi.fn()} />
    </QueryClientProvider>,
  )
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('NotesHubPage', () => {
  it('groups notes by book and marks unmatched ones', async () => {
    renderPage()

    expect(await screen.findByText('人类简史')).toBeTruthy()
    expect(screen.getByText('高亮一')).toBeTruthy()
    expect(screen.getByText('想法一')).toBeTruthy()
    expect(screen.getByText('高亮二')).toBeTruthy()

    expect(screen.getByText('没导入的书')).toBeTruthy()
    expect(screen.getByText('散落的高亮')).toBeTruthy()
    expect(screen.getByText('未匹配')).toBeTruthy()
  })

  it('filters notes with the search box', async () => {
    const fetchMock = renderPage()

    await screen.findByText('人类简史')
    fireEvent.change(screen.getByPlaceholderText('搜索高亮、笔记或书名'), { target: { value: '复杂性' } })

    await waitFor(() => {
      expect(fetchMock.mock.calls.some((call) => String(call[0]).includes('search=%E5%A4%8D%E6%9D%82%E6%80%A7'))).toBe(true)
    })
  })

  it('links an unmatched note to a document', async () => {
    const fetchMock = renderPage()

    await screen.findByText('散落的高亮')
    fireEvent.click(screen.getByRole('button', { name: '关联' }))

    fireEvent.click(await screen.findByRole('button', { name: /人类简史/ }))

    await waitFor(() => {
      expect(fetchMock.mock.calls.some((call) => (
        String(call[0]) === '/api/v1/documents/notes/note-3/link'
        && JSON.parse(String(call[1]?.body)).document_id === 'doc-1'
      ))).toBe(true)
    })
  })

  it('imports a notes JSON file and shows the summary', async () => {
    renderPage()

    await screen.findByText('人类简史')
    const input = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, { target: { files: [new File(['{"notes":[]}'], 'notes.json', { type: 'application/json' })] } })

    expect(await screen.findByText('已导入 5 条，未匹配 2 条')).toBeTruthy()
  })
})
