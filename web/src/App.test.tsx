import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { App } from './App'

const source = {
  id: 'source-1',
  name: 'Example Feed',
  kind: 'rss',
  locator: 'https://example.com/feed',
  normalized_locator: 'https://example.com/feed',
  config: {},
  enabled: true,
  created_at: '2026-07-25T00:00:00Z',
  updated_at: '2026-07-25T00:00:00Z',
}
const scrollIntoView = vi.fn()

describe('App', () => {
  beforeEach(() => {
    scrollIntoView.mockClear()
    Object.defineProperty(Element.prototype, 'scrollIntoView', {
      configurable: true,
      value: scrollIntoView,
    })
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(0)
      return 1
    })
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes('/api/v1/entries')) {
        if (init?.method === 'PATCH' && !url.includes('/entries/')) {
          return new Response(JSON.stringify({ updated_count: 1 }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        if (init?.method === 'PATCH') {
          const patch = JSON.parse(String(init.body)) as { read?: boolean; starred?: boolean; later?: boolean }
          return new Response(JSON.stringify({
            id: 'entry-1',
            source_id: 'source-1',
            identity_key: 'external:entry-1',
            source_title: 'Reader article',
            display_title: '',
            author: 'Ada',
            summary: 'A useful summary',
            content_html: '<h2>Section title</h2><p>Article body</p><img src="https://images.example/cover.jpg" alt="Cover"><script>alert("unsafe")</script>',
            discovered_at: '2026-07-25T00:00:00Z',
            read_at: patch.read === false ? undefined : '2026-07-25T01:00:00Z',
            starred_at: patch.starred ? '2026-07-25T01:00:00Z' : undefined,
            later_at: patch.later ? '2026-07-25T01:00:00Z' : undefined,
            note: '',
          }), { status: 200, headers: { 'Content-Type': 'application/json' } })
        }
        return new Response(JSON.stringify([{
          id: 'entry-1',
          source_id: 'source-1',
          identity_key: 'external:entry-1',
          source_title: 'Reader article',
          display_title: '',
          author: 'Ada',
          summary: 'A useful summary',
          content_html: '<h2>Section title</h2><p>Article body</p><img src="https://images.example/cover.jpg" alt="Cover"><script>alert("unsafe")</script>',
          discovered_at: '2026-07-25T00:00:00Z',
          note: '',
        }]), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      if (url.endsWith('/api/v1/sources') && init?.method === 'POST') {
        return new Response(JSON.stringify({ ...source, id: 'source-2', name: 'New Feed' }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/sources/source-1') && init?.method === 'DELETE') {
        return new Response(null, { status: 204 })
      }
      if (url.endsWith('/api/v1/sources/preview')) {
        return new Response(JSON.stringify({
          diagnostics: { status: 'ok', candidate_count: 1 },
          candidates: [{
            title: 'Preview article',
            url: 'https://new.example/article',
            identity_key: 'external:article-1',
          }],
        }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/folders')) {
        return new Response(JSON.stringify([{
          id: 'folder-1',
          name: 'Tech',
          source_count: 1,
        }]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/sources')) {
        return new Response(JSON.stringify([source]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (init?.method === 'PATCH') {
        return new Response(JSON.stringify({ ...source, enabled: false }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.includes('/runs')) {
        return new Response(JSON.stringify({ id: 'run-1', status: 'pending' }), {
          status: 202,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      throw new Error(`unexpected request: ${url}`)
    }))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView
  })

  it('shows folders, subscriptions, and an expandable article stream', async () => {
    render(<App />)

    expect(screen.getByText('正在同步信息源…')).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: '全部文章' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: 'Example Feed' })).toBeInTheDocument()
    expect(screen.getByText('Tech')).toBeInTheDocument()
    fireEvent.click(await screen.findByText('Reader article'))
    expect(screen.getByText('Article body')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Section title' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Cover' })).toHaveAttribute('loading', 'lazy')
    expect(document.querySelector('.entry-prose script')).toBeNull()
    expect(screen.getByRole('button', { name: '更多操作' })).toBeInTheDocument()
  })

  it('creates an RSS source from the dialog', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })

    fireEvent.click(screen.getByRole('button', { name: '添加信息源' }))
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'New Feed' } })
    fireEvent.change(screen.getByLabelText('Feed 地址'), {
      target: { value: 'https://new.example/feed' },
    })
    fireEvent.click(screen.getByRole('button', { name: '测试并预览' }))
    expect(await screen.findByText('Preview article')).toBeInTheDocument()
    expect(screen.getByText('external:article-1')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '保存并启用' }))

    expect(await screen.findByText('New Feed')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('已添加 New Feed')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('previews a JSON API mapping with cursor pagination', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })

    fireEvent.click(screen.getByRole('button', { name: '添加信息源' }))
    fireEvent.change(screen.getByLabelText('来源类型'), { target: { value: 'json-api' } })
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'API Feed' } })
    fireEvent.change(screen.getByLabelText('API 地址'), {
      target: { value: 'https://api.example/items' },
    })
    fireEvent.change(screen.getByLabelText('列表路径'), { target: { value: 'data.items' } })
    fireEvent.change(screen.getByLabelText('ID 字段'), { target: { value: 'uuid' } })
    fireEvent.change(screen.getByLabelText('标题字段'), { target: { value: 'headline' } })
    fireEvent.change(screen.getByLabelText('分页方式'), { target: { value: 'cursor' } })
    fireEvent.change(screen.getByLabelText('游标路径'), { target: { value: 'paging.cursor' } })
    fireEvent.change(screen.getByLabelText('游标参数'), { target: { value: 'after' } })
    fireEvent.click(screen.getByRole('button', { name: '测试并预览' }))

    await screen.findByText('Preview article')
    const previewCall = vi.mocked(fetch).mock.calls.find(([url]) => String(url).endsWith('/preview'))
    expect(JSON.parse(String(previewCall?.[1]?.body))).toMatchObject({
      kind: 'json-api',
      config: {
        items_path: 'data.items',
        fields: { id: 'uuid', title: 'headline' },
        pagination: { mode: 'cursor', cursor_path: 'paging.cursor', cursor_param: 'after' },
      },
    })
  })

  it('previews a static HTML collection mapping', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })

    fireEvent.click(screen.getByRole('button', { name: '添加信息源' }))
    fireEvent.change(screen.getByLabelText('来源类型'), { target: { value: 'html' } })
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'News page' } })
    fireEvent.change(screen.getByLabelText('网页地址'), {
      target: { value: 'https://example.com/news' },
    })
    fireEvent.change(screen.getByLabelText('条目选择器'), { target: { value: 'article.card' } })
    fireEvent.change(screen.getByLabelText('标题选择器'), { target: { value: 'h2.title' } })
    fireEvent.change(screen.getByLabelText('链接选择器'), { target: { value: 'a.permalink' } })
    fireEvent.click(screen.getByRole('button', { name: '测试并预览' }))

    await screen.findByText('Preview article')
    const previewCall = vi.mocked(fetch).mock.calls.filter(([url]) => String(url).endsWith('/preview')).at(-1)
    expect(JSON.parse(String(previewCall?.[1]?.body))).toMatchObject({
      kind: 'html',
      config: {
        mode: 'collection',
        item_selector: 'article.card',
        fields: {
          title: { selector: 'h2.title' },
          url: { selector: 'a.permalink', attribute: 'href' },
        },
      },
    })
  })

  it('triggers a manual refresh', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })
    fireEvent.click(screen.getByRole('button', { name: '管理信息源' }))

    fireEvent.click(screen.getByRole('button', { name: '刷新 Example Feed' }))

    await waitFor(() => {
      expect(screen.getByText('抓取任务已进入队列')).toBeInTheDocument()
    })
  })

  it('pauses an enabled source', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })
    fireEvent.click(screen.getByRole('button', { name: '管理信息源' }))

    fireEvent.click(screen.getByRole('button', { name: '暂停 Example Feed' }))

    expect(await screen.findByText('已暂停 Example Feed')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '恢复 Example Feed' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '刷新 Example Feed' })).toBeDisabled()
  })

  it('confirms and archives a source while preserving its entries', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })
    fireEvent.click(screen.getByRole('button', { name: '管理信息源' }))

    fireEvent.click(screen.getByRole('button', { name: '删除 Example Feed' }))
    expect(screen.getByRole('dialog', { name: '删除信息源？' })).toBeInTheDocument()
    expect(screen.getByText('已经抓取的文章、收藏和笔记都会保留。')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '取消删除' }))
    expect(screen.getByRole('button', { name: '删除 Example Feed' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '删除 Example Feed' }))
    fireEvent.click(screen.getByRole('button', { name: '确认删除 Example Feed' }))

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: '删除 Example Feed' })).not.toBeInTheDocument()
    })
    expect(screen.getByRole('status')).toHaveTextContent('已删除 Example Feed')
    expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/sources/source-1', { method: 'DELETE' })
  })

  it('marks an unread entry as read when it is expanded', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })

    expect(await screen.findByText('Reader article')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Reader article'))
    expect(screen.getByText('Article body')).toBeInTheDocument()
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' })

    await waitFor(() => {
      const patches = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).endsWith('/api/v1/entries/entry-1') && init?.method === 'PATCH')
      expect(patches).toHaveLength(1)
      expect(JSON.parse(String(patches[0][1]?.body))).toEqual({ read: true })
    })

    expect(screen.queryByRole('button', { name: '收藏文章' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '稍后阅读' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '更多操作' }))
    expect(screen.getByRole('button', { name: '收藏文章' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '稍后阅读' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '收藏文章' }))
    expect(await screen.findByRole('button', { name: '取消收藏' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '稍后阅读' }))
    expect(await screen.findByRole('button', { name: '移出稍后阅读' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '编辑标题与笔记' }))
    fireEvent.change(screen.getByLabelText('显示标题'), { target: { value: 'My title' } })
    fireEvent.change(screen.getByLabelText('笔记'), { target: { value: 'Remember this' } })
    fireEvent.click(screen.getByRole('button', { name: '保存标题与笔记' }))

    await waitFor(() => {
      const patches = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).endsWith('/api/v1/entries/entry-1') && init?.method === 'PATCH')
      expect(patches).toHaveLength(4)
    })

    fireEvent.click(screen.getByRole('button', { name: '更多操作' }))
    fireEvent.click(screen.getByRole('button', { name: '编辑标题与笔记' }))
    expect(screen.queryByLabelText('笔记')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { expanded: true }))
    expect(screen.queryByText('Article body')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { expanded: false }))
    fireEvent.click(screen.getByRole('button', { name: '更多操作' }))
    fireEvent.click(screen.getByRole('button', { name: '更多操作' }))
    await waitFor(() => {
      const patches = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).endsWith('/api/v1/entries/entry-1') && init?.method === 'PATCH')
      expect(patches).toHaveLength(4)
    })
  })

  it('collapses the expanded entry when Escape is pressed', async () => {
    render(<App />)
    await screen.findByText('Reader article')

    fireEvent.click(screen.getByText('Reader article'))
    expect(screen.getByText('Article body')).toBeInTheDocument()
    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.queryByText('Article body')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument()
  })

  it('filters the stream when a subscription is selected', async () => {
    render(<App />)

    fireEvent.click(await screen.findByRole('button', { name: 'Example Feed' }))

    expect(await screen.findByRole('heading', { name: 'Example Feed' })).toBeInTheDocument()
    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some(([url]) =>
        String(url).includes('/api/v1/entries?') && String(url).includes('source_id=source-1'))).toBe(true)
    })
  })

  it('marks all entries or only the selected source as read from the reader toolbar', async () => {
    render(<App />)
    await screen.findByText('Reader article')

    fireEvent.click(screen.getByRole('button', { name: '将全部文章标记为已读' }))
    await waitFor(() => {
      expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/entries', expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ read: true }),
      }))
    })

    fireEvent.click(screen.getByRole('button', { name: 'Example Feed' }))
    await screen.findByRole('heading', { name: 'Example Feed' })
    fireEvent.click(screen.getByRole('button', { name: '将 Example Feed 全部标记为已读' }))
    await waitFor(() => {
      expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/entries?source_id=source-1', expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ read: true }),
      }))
    })
  })

  it('shows an empty state and reloads sources', async () => {
    let sourceLoads = 0
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/sources')) {
        sourceLoads += 1
        return new Response(JSON.stringify(sourceLoads === 1 ? [] : [source]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/folders') || url.includes('/api/v1/entries')) {
        return new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      throw new Error(`unexpected request: ${url}`)
    })

    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: '管理信息源' }))
    expect(await screen.findByText('还没有信息源')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '重新载入' }))
    expect(await screen.findByRole('button', { name: 'Example Feed' })).toBeInTheDocument()
  })

  it('reports loading, creation, and refresh errors', async () => {
    let failSourceLoad = true
    let failPreview = false
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/sources') && !init?.method) {
        if (failSourceLoad) {
          failSourceLoad = false
          return new Response(JSON.stringify({ detail: '数据库暂时不可用' }), {
            status: 503,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        return new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      if (url.endsWith('/api/v1/folders') || url.includes('/api/v1/entries')) {
        return new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      if (url.endsWith('/api/v1/sources/preview') && failPreview) {
        return new Response('not-json', { status: 500 })
      }
      throw new Error(`unexpected request: ${url}`)
    })

    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: '管理信息源' }))
    expect(await screen.findByText('数据库暂时不可用')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    await screen.findByText('还没有信息源')

    fireEvent.click(screen.getAllByRole('button', { name: '添加信息源' })[1])
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'Broken Feed' } })
    fireEvent.change(screen.getByLabelText('Feed 地址'), {
      target: { value: 'https://broken.example/feed' },
    })
    failPreview = true
    fireEvent.click(screen.getByRole('button', { name: '测试并预览' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('请求失败（500）')

    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
