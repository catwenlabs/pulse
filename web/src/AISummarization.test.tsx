import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DigestPage, StoryDetailPage } from './AISummarization'

function renderWithQueryClient(element: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
  })
  return render(<QueryClientProvider client={client}>{element}</QueryClientProvider>)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DigestPage', () => {
  it('renders structured title-only results and queues a scoped Digest on demand', async () => {
    const digest = {
      id: 'digest-1',
      status: 'completed',
      mode: 'catch_up',
      story_count: 2,
      overview: '今天主要有两个值得关注的主题。',
      provider: 'openai-compatible',
      model: 'qwen3',
      created_at: '2026-08-04T09:00:00Z',
      stories: [
        { label: 'S1', story_id: 'story-1', title: '标题一', entry_count: 2, source_count: 2, available: true },
        { label: 'S2', story_id: 'story-2', title: '标题二', entry_count: 1, source_count: 1, available: false },
      ],
      themes: [{ title: '主题一', summary: '只根据标题归类。', story_ids: ['story-1'] }],
      priorities: [{ rank: 1, title: '先看标题一', reason: '标题显示它可能更重要。', story_ids: ['story-1'] }],
      omissions: [{ label: 'S2', story_id: 'story-2', title: '标题二', reason: '模型未在重点部分引用' }],
    }
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/digests?limit=50') return new Response(JSON.stringify([digest]), { status: 200 })
      if (url.startsWith('/api/v1/digests/preview')) {
        const query = new URL(url, 'http://localhost').searchParams
        const scope = {
          ...(query.get('start_at') ? { start_at: query.get('start_at') } : {}),
          ...(query.get('end_at') ? { end_at: query.get('end_at') } : {}),
          ...(query.get('max_stories') ? { max_stories: Number(query.get('max_stories')) } : {}),
        }
        return new Response(JSON.stringify({ scope, matching_stories: 2, matching_stories_truncated: false, selected_stories: 2, safety_limit: 100, can_queue: true }), { status: 200 })
      }
      if (url === '/api/v1/digests/digest-1') return new Response(JSON.stringify(digest), { status: 200 })
      if (url === '/api/v1/digests' && init?.method === 'POST') {
        return new Response('{"id":"job-1","kind":"digest","target_id":"digest-2","status":"pending"}', { status: 202 })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithQueryClient(<DigestPage />)

    expect(await screen.findByText('今天主要有两个值得关注的主题。')).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: /标题一/ })[0]).toHaveAttribute('href', '/stories/story-1')
    expect(screen.getAllByText(/S2 · 标题二/).every((element) => element.closest('a') === null)).toBe(true)
    expect(screen.getByText('主题一')).toBeInTheDocument()
    expect(screen.getByText(/先看标题一/)).toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)

    fireEvent.change(screen.getByLabelText('最多 Story（可选）'), { target: { value: '12' } })
    fireEvent.change(screen.getByLabelText('最早时间（可选）'), { target: { value: '2026-08-01T08:00' } })
    await screen.findByText(/当前范围（08\/01 08:00 之后）匹配 2 个未读 Story/)
    fireEvent.click(screen.getByRole('button', { name: '生成追更摘要' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/v1/digests', expect.objectContaining({
      method: 'POST',
      body: expect.stringContaining('"max_stories":12'),
    })))
  })

  it('rejects a non-positive Story limit before making a request', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/api/v1/digests/preview')) {
        return Promise.resolve(new Response('{"scope":{},"matching_stories":0,"matching_stories_truncated":false,"selected_stories":0,"safety_limit":100,"can_queue":true}', { status: 200 }))
      }
      return Promise.resolve(new Response('[]', { status: 200 }))
    })
    vi.stubGlobal('fetch', fetchMock)
    renderWithQueryClient(<DigestPage />)
    await screen.findByText('还没有追更摘要。生成一份，稍后可以回来查看。')
    fireEvent.change(screen.getByLabelText('最多 Story（可选）'), { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: '数量必须是正整数' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('数量必须是正整数')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
  })
})

describe('StoryDetailPage', () => {
  it('shows the Story without requesting AI until the user clicks', async () => {
    const story = {
      id: 'story-1',
      display_title: '一个需要总结的 Story',
      source_count: 2,
      entry_count: 2,
      representative: {
        id: 'entry-1', source_id: 'source-1', identity_key: 'key-1', source_title: '来源一',
        summary: '来源摘要一', discovered_at: '2026-08-04T08:00:00Z',
      },
      entries: [
        {
          id: 'entry-1', source_id: 'source-1', identity_key: 'key-1', source_title: '来源一',
          summary: '来源摘要一', discovered_at: '2026-08-04T08:00:00Z',
        },
        {
          id: 'entry-2', source_id: 'source-2', identity_key: 'key-2', source_title: '来源二',
          summary: '来源摘要二', discovered_at: '2026-08-04T08:01:00Z',
        },
      ],
      ai_summary: { story_id: 'story-1', status: 'not_requested' },
    }
    const completed = {
      ...story,
      ai_summary: {
        story_id: 'story-1', status: 'completed', overview: '这是完成后的摘要。',
        key_points: ['关键点一'], sources: [{ label: 'E1', entry_id: 'entry-1', title: '来源一', note: '它补充了背景。' }],
      },
    }
    let storyFetches = 0
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/stories/story-1/ai-summary' && init?.method === 'POST') {
        return new Response('{"id":"job-1","kind":"story_summary","target_id":"story-1","status":"pending"}', { status: 202 })
      }
      if (url === '/api/v1/stories/story-1') {
        storyFetches += 1
        return new Response(JSON.stringify(storyFetches === 1 ? story : completed), { status: 200 })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithQueryClient(<StoryDetailPage storyID="story-1" />)
    expect(await screen.findByText('一个需要总结的 Story')).toBeInTheDocument()
    expect(screen.getByText('还没有摘要。点击右上角按钮后，AI 才会读取这个 Story 的内容。')).toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)

    fireEvent.click(screen.getByRole('button', { name: '生成 Story 摘要' }))
    expect(await screen.findByText('这是完成后的摘要。')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /E1 · 来源一/ })).toHaveAttribute('href', '#entry-entry-1')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(true)
  })
})
