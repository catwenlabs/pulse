import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
  vi.restoreAllMocks()
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
        const scoped = Object.keys(scope).length > 0
        return new Response(JSON.stringify({
          scope,
          matching_stories: scoped ? 2 : 101,
          matching_stories_truncated: !scoped,
          selected_stories: scoped ? 2 : 0,
          safety_limit: 100,
          can_queue: scoped,
        }), { status: 200 })
      }
      if (url === '/api/v1/digests/digest-1') return new Response(JSON.stringify(digest), { status: 200 })
      if (url === '/api/v1/stories' && init?.method === 'PATCH') {
        return new Response('{"updated_count":1}', { status: 200 })
      }
      if (url === '/api/v1/digests' && init?.method === 'POST') {
        return new Response('{"id":"job-1","kind":"digest","target_id":"digest-2","status":"pending"}', { status: 202 })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithQueryClient(<DigestPage />)

    expect(await screen.findByText('今天主要有两个值得关注的主题。')).toBeInTheDocument()
    expect(screen.getByRole('dialog', { name: '设置追更范围' })).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: /标题一/ })[0]).toHaveAttribute('href', '/stories/story-1')
    expect(screen.getAllByText(/S2 · 标题二/).every((element) => element.closest('a') === null)).toBe(true)
    expect(screen.getByText('主题一')).toBeInTheDocument()
    expect(screen.getByText(/先看标题一/)).toBeInTheDocument()
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)

    fireEvent.click(screen.getByRole('button', { name: '将 1 个 Story 标记为已读' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/v1/stories', expect.objectContaining({
      method: 'PATCH',
      body: JSON.stringify({ read: true, story_ids: ['story-1'] }),
    })))
    expect(await screen.findByRole('button', { name: '相关 Story 已标为已读' })).toBeDisabled()

    fireEvent.change(screen.getByLabelText('最多 Story（可选）'), { target: { value: '12' } })
    fireEvent.change(screen.getByLabelText('最早时间（可选）'), { target: { value: '2026-08-01T08:00' } })
    await screen.findByText(/当前范围（08\/01 08:00 之后）匹配 2 个未读 Story/)
    fireEvent.click(screen.getByRole('button', { name: '生成追更摘要' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/v1/digests', expect.objectContaining({
      method: 'POST',
      body: expect.stringContaining('"max_stories":12'),
    })))
  })

  it('prefills the safety limit when the default scope is oversized', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/digests?limit=50') return new Response('[]', { status: 200 })
      if (url.startsWith('/api/v1/digests/preview')) {
        const maxStories = new URL(url, 'http://localhost').searchParams.get('max_stories')
        const selectedStories = maxStories ? Number(maxStories) : 0
        return new Response(JSON.stringify({
          scope: maxStories ? { max_stories: selectedStories } : {},
          matching_stories: 101,
          matching_stories_truncated: true,
          selected_stories: selectedStories,
          safety_limit: 100,
          can_queue: Boolean(maxStories),
        }), { status: 200 })
      }
      if (url === '/api/v1/digests' && init?.method === 'POST') {
        return new Response('{"id":"job-1","kind":"digest","target_id":"digest-1","status":"pending"}', { status: 202 })
      }
      if (url === '/api/v1/digests/digest-1') {
        return new Response('{"id":"digest-1","status":"pending","mode":"catch_up","story_count":100}', { status: 200 })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithQueryClient(<DigestPage />)

    const maxStoriesInput = await screen.findByLabelText('最多 Story（可选）')
    await waitFor(() => expect(maxStoriesInput).toHaveValue('100'))
    const generateButton = await screen.findByRole('button', { name: '生成追更摘要' })
    expect(generateButton).toBeEnabled()

    fireEvent.click(generateButton)
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/v1/digests', expect.objectContaining({
      method: 'POST',
      body: expect.stringContaining('"max_stories":100'),
    })))
  })

  it('puts the default Digest action in the page header', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/digests?limit=50') return new Response('[]', { status: 200 })
      if (url.startsWith('/api/v1/digests/preview')) {
        return new Response('{"scope":{},"matching_stories":15,"matching_stories_truncated":false,"selected_stories":15,"safety_limit":100,"can_queue":true}', { status: 200 })
      }
      if (url === '/api/v1/digests' && init?.method === 'POST') {
        return new Response('{"id":"job-1","kind":"digest","target_id":"digest-1","status":"pending"}', { status: 202 })
      }
      if (url === '/api/v1/digests/digest-1') {
        return new Response('{"id":"digest-1","status":"pending","mode":"catch_up","story_count":15}', { status: 200 })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithQueryClient(<DigestPage />)

    expect(await screen.findByText(/当前有 15 条未读 Story/)).toBeInTheDocument()
    expect(screen.queryByText('直接整理未读 Story')).not.toBeInTheDocument()
    expect(screen.queryByText('标题级处理')).not.toBeInTheDocument()
    expect(screen.queryByRole('dialog', { name: '设置追更范围' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('最多 Story（可选）')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '生成追更摘要' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/v1/digests', expect.objectContaining({
      method: 'POST',
      body: '{}',
    })))
  })

  it('keeps the header action width stable while a Digest is being created', async () => {
    let releaseCreate: ((response: Response) => void) | undefined
    const createResponse = new Promise<Response>((resolve) => {
      releaseCreate = resolve
    })
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url === '/api/v1/digests?limit=50') return new Response('[]', { status: 200 })
      if (url.startsWith('/api/v1/digests/preview')) {
        return new Response('{"scope":{},"matching_stories":15,"matching_stories_truncated":false,"selected_stories":15,"safety_limit":100,"can_queue":true}', { status: 200 })
      }
      if (url === '/api/v1/digests' && init?.method === 'POST') return createResponse
      if (url === '/api/v1/digests/digest-1') {
        return new Response('{"id":"digest-1","status":"pending","mode":"catch_up","story_count":15}', { status: 200 })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithQueryClient(<DigestPage />)

    const action = await screen.findByRole('button', { name: '生成追更摘要' })
    expect(action).toHaveClass('min-w-[148px]')
    expect(action).toHaveClass('cursor-pointer')
    fireEvent.click(action)
    expect(await screen.findByRole('button', { name: '正在排队…' })).toHaveClass('min-w-[148px]')

    releaseCreate?.(new Response('{"id":"job-1","kind":"digest","target_id":"digest-1","status":"pending"}', { status: 202 }))
    expect(await screen.findByText('正在整理标题级速览，完成后会自动更新。')).toBeInTheDocument()
  })

  it('keeps the previous result visible while another Digest loads', async () => {
    const firstDigest = {
      id: 'digest-1', status: 'completed', mode: 'catch_up', story_count: 1,
      overview: '第一份摘要仍然可见。', created_at: '2026-08-04T09:00:00Z', stories: [],
    }
    const secondDigest = {
      id: 'digest-2', status: 'completed', mode: 'catch_up', story_count: 1,
      overview: '第二份摘要已经更新。', created_at: '2026-08-04T10:00:00Z', stories: [],
    }
    let releaseSecond: ((response: Response) => void) | undefined
    const secondResponse = new Promise<Response>((resolve) => {
      releaseSecond = resolve
    })
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/digests?limit=50') return new Response(JSON.stringify([firstDigest, secondDigest]), { status: 200 })
      if (url.startsWith('/api/v1/digests/preview')) {
        return new Response('{"scope":{},"matching_stories":0,"matching_stories_truncated":false,"selected_stories":0,"safety_limit":100,"can_queue":true}', { status: 200 })
      }
      if (url === '/api/v1/digests/digest-1') return new Response(JSON.stringify(firstDigest), { status: 200 })
      if (url === '/api/v1/digests/digest-2') return secondResponse
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    const { container } = renderWithQueryClient(<main><DigestPage /></main>)

    expect(await screen.findByText('第一份摘要仍然可见。')).toBeInTheDocument()
    const historyItems = screen.getAllByRole('button', { name: /1 个未读 Story/ })
    const scrollContainer = container.querySelector('main')!
    const historyLayout = container.querySelector('.ai-history-layout')!
    Object.defineProperty(scrollContainer, 'scrollTop', { configurable: true, value: 360, writable: true })
    vi.spyOn(scrollContainer, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect)
    vi.spyOn(historyLayout, 'getBoundingClientRect').mockReturnValue({ top: 120 } as DOMRect)
    const scrollTo = vi.fn()
    Object.defineProperty(scrollContainer, 'scrollTo', { configurable: true, value: scrollTo })
    fireEvent.click(historyItems[1])

    expect(scrollTo).toHaveBeenCalledWith({ top: 456, left: 0, behavior: 'smooth' })
    expect(screen.getByText('第一份摘要仍然可见。')).toBeInTheDocument()
    expect(screen.getByText('正在更新摘要…')).toBeInTheDocument()
    expect(screen.queryByText('正在加载追更摘要')).not.toBeInTheDocument()

    releaseSecond?.(new Response(JSON.stringify(secondDigest), { status: 200 }))
    expect(await screen.findByText('第二份摘要已经更新。')).toBeInTheDocument()
  })

  it('rejects a non-positive Story limit before making a request', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input)
      if (url.startsWith('/api/v1/digests/preview')) {
        return Promise.resolve(new Response('{"scope":{},"matching_stories":101,"matching_stories_truncated":true,"selected_stories":0,"safety_limit":100,"can_queue":false}', { status: 200 }))
      }
      return Promise.resolve(new Response('[]', { status: 200 }))
    })
    vi.stubGlobal('fetch', fetchMock)
    renderWithQueryClient(<DigestPage />)
    await screen.findByRole('dialog', { name: '设置追更范围' })
    fireEvent.change(screen.getByLabelText('最多 Story（可选）'), { target: { value: '0' } })
    fireEvent.click(screen.getByRole('button', { name: '数量必须是正整数' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('数量必须是正整数')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
  })

  it('refreshes the history status after a Digest completes', async () => {
    const pendingDigest = {
      id: 'digest-1',
      status: 'running',
      mode: 'catch_up',
      story_count: 1,
      created_at: '2026-08-04T09:00:00Z',
    }
    const completedDigest = { ...pendingDigest, status: 'completed' }
    let listCalls = 0
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/digests?limit=50') {
        const digest = listCalls++ === 0 ? pendingDigest : completedDigest
        return new Response(JSON.stringify([digest]), { status: 200 })
      }
      if (url.startsWith('/api/v1/digests/preview')) {
        return new Response('{"scope":{},"matching_stories":0,"matching_stories_truncated":false,"selected_stories":0,"safety_limit":100,"can_queue":true}', { status: 200 })
      }
      if (url === '/api/v1/digests/digest-1') {
        return new Response(JSON.stringify(completedDigest), { status: 200 })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithQueryClient(<DigestPage />)

    const historyItem = await screen.findByRole('button', { name: /1 个未读 Story/ })
    expect(within(historyItem).getByText('生成中')).toBeInTheDocument()
    await waitFor(() => expect(within(historyItem).getByText('已完成')).toBeInTheDocument(), { timeout: 4000 })
    expect(listCalls).toBeGreaterThan(1)
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
          summary: '来源摘要一', content_html: '<p>来源正文一</p><script>window.bad = true</script>', discovered_at: '2026-08-04T08:00:00Z',
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
    expect(screen.getByText('还没有摘要。点击「生成AI摘要」后，AI 才会读取这个 Story 的内容。')).toBeInTheDocument()
    expect(screen.getByText('来源正文一')).toBeInTheDocument()
    expect(document.querySelector('.story-entry-reader script')).toBeNull()
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)

    fireEvent.click(screen.getByRole('button', { name: '生成AI摘要' }))
    expect(await screen.findByText('这是完成后的摘要。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重新生成AI摘要' })).toBeInTheDocument()
    expect(document.querySelector('.story-summary-card')).toHaveClass('pb-6')
    expect(screen.getByRole('link', { name: /E1 · 来源一/ })).toHaveAttribute('href', '#entry-entry-1')
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(true)
  })
})
