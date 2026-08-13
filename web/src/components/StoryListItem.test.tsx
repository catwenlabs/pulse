import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { StoryListItem, type StoryListItemProps } from './StoryListItem'

const story = {
  id: 'story-1',
  display_title: '',
  note: '',
  representative: {
    id: 'entry-1',
    source_id: 'source-1',
    identity_key: 'k1',
    source_title: '标题一',
    canonical_url: 'https://example.com/1',
    discovered_at: '2026-08-04T09:00:00Z',
  },
  entries: [
    { id: 'entry-1', source_id: 'source-1', identity_key: 'k1', source_title: '标题一', canonical_url: 'https://example.com/1', discovered_at: '2026-08-04T09:00:00Z' },
    { id: 'entry-2', source_id: 'source-2', identity_key: 'k2', source_title: '标题一副本', canonical_url: 'https://example.com/2', discovered_at: '2026-08-04T09:05:00Z' },
  ],
  entry_count: 2,
  source_count: 2,
}

const readStory = { ...story, read_at: '2026-08-04T10:00:00Z' }

function Harness(props: Omit<StoryListItemProps, 'expanded' | 'onExpandedChange'> & { initialExpanded?: boolean; onExpandedChange?: (open: boolean) => void }) {
  const { initialExpanded = false, onExpandedChange, ...rest } = props
  const [expanded, setExpanded] = useState(initialExpanded)
  return (
    <StoryListItem
      {...rest}
      expanded={expanded}
      onExpandedChange={(open) => {
        setExpanded(open)
        onExpandedChange?.(open)
      }}
    />
  )
}

function renderItem(ui: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
  })
  const rootRoute = createRootRoute()
  const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: '/', component: () => <>{ui}</> })
  const storyRoute = createRoute({ getParentRoute: () => rootRoute, path: '/stories/$storyID', component: () => null })
  const router = createRouter({
    routeTree: rootRoute.addChildren([indexRoute, storyRoute]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

function mockFetch(handler: (url: string, init?: RequestInit) => Response | undefined) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const response = handler(String(input), init)
    if (!response) throw new Error(`unexpected request: ${String(input)}`)
    return response
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const defaultRow = {
  title: '标题一',
  sourceLabel: '2 个来源',
  timestamp: '2026-08-04T09:00:00Z',
  read: false,
}

function handleGetStory(url: string, init?: RequestInit) {
  if (url === '/api/v1/stories/story-1' && !init) return new Response(JSON.stringify(readStory), { status: 200 })
  if (url === '/api/v1/stories/story-1' && init?.method === 'PATCH') return new Response(JSON.stringify(readStory), { status: 200 })
  return undefined
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('StoryListItem', () => {
  it('renders the collapsed row without fetching the story', async () => {
    const fetchMock = mockFetch(() => undefined)
    renderItem(<Harness storyId="story-1" row={defaultRow} />)
    expect(await screen.findByText('标题一')).toBeInTheDocument()
    expect(screen.getByText('2 个来源')).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('fetches the story once when expanded and marks it read', async () => {
    const fetchMock = mockFetch(handleGetStory)
    const onChanged = vi.fn()
    renderItem(<Harness storyId="story-1" row={defaultRow} initialExpanded onChanged={onChanged} />)
    expect(await screen.findByRole('button', { name: '更多操作' })).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/v1/stories/story-1', undefined))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/v1/stories/story-1', expect.objectContaining({ method: 'PATCH' })))
    await waitFor(() => expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ kind: 'patched' })))
  })

  it('does not mark read on expand when disabled', async () => {
    const fetchMock = mockFetch((url, init) => {
      if (url === '/api/v1/stories/story-1' && !init) return new Response(JSON.stringify(readStory), { status: 200 })
      return undefined
    })
    renderItem(<Harness storyId="story-1" row={defaultRow} initialExpanded markReadOnExpand={false} />)
    expect(await screen.findByRole('button', { name: '更多操作' })).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalledWith('/api/v1/stories/story-1', expect.objectContaining({ method: 'PATCH' }))
  })

  it('stars the story from the action menu and notifies onChanged', async () => {
    const fetchMock = mockFetch((url, init) => {
      if (url === '/api/v1/stories/story-1' && !init) return new Response(JSON.stringify(readStory), { status: 200 })
      if (url === '/api/v1/stories/story-1' && init?.method === 'PATCH') {
        return new Response(JSON.stringify({ ...readStory, starred_at: '2026-08-04T11:00:00Z' }), { status: 200 })
      }
      return undefined
    })
    const onChanged = vi.fn()
    renderItem(<Harness storyId="story-1" row={{ ...defaultRow, read: true }} initialExpanded onChanged={onChanged} />)
    fireEvent.pointerDown(await screen.findByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '收藏文章' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/v1/stories/story-1', expect.objectContaining({
      method: 'PATCH',
      body: JSON.stringify({ starred: true }),
    })))
    await waitFor(() => expect(onChanged).toHaveBeenCalledWith(expect.objectContaining({ kind: 'patched', patch: { starred: true } })))
  })

  it('opens the merge resolution dialog on a 409 conflict', async () => {
    mockFetch((url, init) => {
      if (url === '/api/v1/stories/story-1' && !init) return new Response(JSON.stringify(readStory), { status: 200 })
      if (url === '/api/v1/stories/story-1/merge') {
        return new Response(JSON.stringify({ title: 'conflict', status: 409 }), { status: 409 })
      }
      return undefined
    })
    renderItem(
      <Harness
        storyId="story-1"
        row={{ ...defaultRow, read: true }}
        initialExpanded
        mergeCandidates={[{ storyId: 'story-2', title: '目标 Story' }]}
      />,
    )
    fireEvent.pointerDown(await screen.findByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '合并到其他 Story' }))
    fireEvent.click(await screen.findByRole('button', { name: '合并到：目标 Story' }))
    expect(await screen.findByRole('dialog', { name: '解决 Story 元数据冲突' })).toBeInTheDocument()
  })

  it('collapses and returns focus to the row trigger on Escape', async () => {
    mockFetch(handleGetStory)
    const onExpandedChange = vi.fn()
    renderItem(<Harness storyId="story-1" row={{ ...defaultRow, read: true }} initialExpanded onExpandedChange={onExpandedChange} />)
    expect(await screen.findByRole('button', { name: '更多操作' })).toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => expect(onExpandedChange).toHaveBeenCalledWith(false))
    await waitFor(() => expect(screen.queryByRole('button', { name: '更多操作' })).not.toBeInTheDocument())
    expect(document.activeElement).toBe(document.querySelector('button[aria-expanded]'))
  })
})
