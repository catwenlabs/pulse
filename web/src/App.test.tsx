import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
    window.history.replaceState(null, '', '/')
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
      if (url.includes('/api/v1/sources/') && url.endsWith('/entries') && init?.method === 'POST') {
        return new Response(JSON.stringify({ id: 'manual-job', status: 'pending' }), {
          status: 202,
          headers: { 'Content-Type': 'application/json' },
        })
      }
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
        const input = JSON.parse(String(init.body)) as {
          name: string
          kind: string
          locator: string
        }
        return new Response(JSON.stringify({
          ...source,
          id: 'source-2',
          name: input.name,
          kind: input.kind,
          locator: input.locator,
          normalized_locator: input.locator,
        }), {
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

  it('scrolls the reading area to the top and marks an unread entry as read when expanded', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })

    expect(await screen.findByText('Reader article')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Reader article'))
    expect(screen.getByText('Article body')).toBeInTheDocument()
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' })
    expect(scrollIntoView.mock.contexts.at(-1)).toHaveAttribute('data-entry-detail', 'entry-1')

    await waitFor(() => {
      const patches = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).endsWith('/api/v1/entries/entry-1') && init?.method === 'PATCH')
      expect(patches).toHaveLength(1)
      expect(JSON.parse(String(patches[0][1]?.body))).toEqual({ read: true })
    })

    expect(screen.queryByRole('menuitem', { name: '收藏文章' })).not.toBeInTheDocument()
    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    expect(screen.getByRole('menuitem', { name: '收藏文章' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '稍后阅读' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('menuitem', { name: '收藏文章' }))
    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '稍后阅读' }))
    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '编辑标题与笔记' }))
    fireEvent.change(screen.getByLabelText('显示标题'), { target: { value: 'My title' } })
    fireEvent.change(screen.getByLabelText('笔记'), { target: { value: 'Remember this' } })
    fireEvent.click(screen.getByRole('button', { name: '保存标题与笔记' }))

    await waitFor(() => {
      const patches = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).endsWith('/api/v1/entries/entry-1') && init?.method === 'PATCH')
      expect(patches).toHaveLength(4)
    })

    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.click(screen.getByRole('menuitem', { name: '编辑标题与笔记' }))
    expect(screen.queryByLabelText('笔记')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { expanded: true }))
    expect(screen.queryByText('Article body')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { expanded: false }))
    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.keyDown(document, { key: 'Escape' })
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
    scrollIntoView.mockClear()
    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.queryByText('Article body')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument()
    expect(scrollIntoView).toHaveBeenCalledOnce()
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' })
    expect(scrollIntoView.mock.contexts.at(-1)).toHaveAttribute('data-entry-row', 'entry-1')
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

  it('uses an accessible off-canvas navigation drawer on mobile', async () => {
    vi.stubGlobal('matchMedia', vi.fn().mockImplementation((query: string) => ({
      matches: query === '(max-width: 767px)',
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })))

    render(<App />)
    await screen.findByText('Reader article')

    const menuButton = screen.getByRole('button', { name: '打开导航' })
    expect(document.getElementById('mobile-navigation')).toBeNull()
    expect(menuButton).toHaveAttribute('aria-expanded', 'false')

    fireEvent.click(menuButton)
    const drawer = document.getElementById('mobile-navigation')
    expect(drawer).not.toBeNull()
    expect(drawer!).toHaveAttribute('aria-hidden', 'false')
    expect(menuButton).toHaveAttribute('aria-expanded', 'true')
    expect(document.querySelector('main')).toHaveAttribute('inert')
    const closeButton = screen.getByRole('button', { name: '关闭导航' })
    expect(closeButton).toHaveFocus()
    screen.getByRole('link', { name: 'Pulse 首页' }).focus()
    fireEvent.keyDown(screen.getByRole('link', { name: 'Pulse 首页' }), { key: 'Tab', shiftKey: true })
    expect(screen.getByRole('button', { name: '管理信息源' })).toHaveFocus()
    fireEvent.keyDown(screen.getByRole('button', { name: '管理信息源' }), { key: 'Tab' })
    expect(screen.getByRole('link', { name: 'Pulse 首页' })).toHaveFocus()

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(document.getElementById('mobile-navigation')).toBeNull()
    await waitFor(() => expect(menuButton).toHaveFocus())
    expect(screen.getByRole('heading', { level: 1, name: '全部文章' })).toBeInTheDocument()

    fireEvent.click(menuButton)
    fireEvent.click(screen.getByRole('button', { name: 'Example Feed' }))
    expect(document.getElementById('mobile-navigation')).toBeNull()
    await waitFor(() => expect(menuButton).toHaveFocus())
    expect(await screen.findByRole('heading', { name: 'Example Feed' })).toBeInTheDocument()
  })

  it('shows an installable bookmarklet from the navigation', async () => {
    render(<App />)
    await screen.findByText('Reader article')

    const installButton = screen.getByRole('button', { name: '安装保存书签' })
    installButton.focus()
    fireEvent.click(installButton)

    const dialog = screen.getByRole('dialog', { name: '安装“保存到 Pulse”' })
    const code = screen.getByLabelText('Bookmarklet 代码')
    expect(dialog).toBeInTheDocument()
    expect((code as HTMLTextAreaElement).value).toContain('javascript:')
    expect((code as HTMLTextAreaElement).value).toContain(`${window.location.origin}/#save?`)
    expect((code as HTMLTextAreaElement).value).toContain("'_blank'")
    expect((code as HTMLTextAreaElement).value).toContain('noopener')
    expect(within(dialog).getByRole('heading', { name: 'Mac Chrome' })).toBeInTheDocument()
    expect(within(dialog).getByRole('heading', { name: 'iPhone Chrome' })).toBeInTheDocument()
    expect(within(dialog).getByText(/长按刚创建的书签/)).toBeInTheDocument()
    expect(within(dialog).getByText(/打开要收藏的网页，再从书签中点“保存到 Pulse”/)).toBeInTheDocument()

    const closeButton = within(dialog).getByRole('button', { name: '关闭' })
    const doneButton = within(dialog).getByRole('button', { name: '完成' })
    doneButton.focus()
    fireEvent.keyDown(dialog, { key: 'Tab' })
    expect(closeButton).toHaveFocus()
    closeButton.focus()
    fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true })
    expect(doneButton).toHaveFocus()

    fireEvent.keyDown(dialog, { key: 'Escape' })
    expect(screen.queryByRole('dialog', { name: '安装“保存到 Pulse”' })).not.toBeInTheDocument()
    expect(installButton).toHaveFocus()
  })

  it('returns focus to the navigation menu after closing the bookmarklet dialog on mobile', async () => {
    vi.stubGlobal('matchMedia', vi.fn().mockImplementation((query: string) => ({
      matches: query === '(max-width: 767px)',
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })))

    render(<App />)
    await screen.findByText('Reader article')

    const menuButton = screen.getByRole('button', { name: '打开导航' })
    fireEvent.click(menuButton)
    fireEvent.click(screen.getByRole('button', { name: '安装保存书签' }))
    const dialog = screen.getByRole('dialog', { name: '安装“保存到 Pulse”' })
    fireEvent.keyDown(dialog, { key: 'Escape' })

    expect(dialog).not.toBeInTheDocument()
    expect(menuButton).toHaveFocus()
  })

  it('groups reading annotations by book and imports a new highlight', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    const annotationSource = {
      ...source,
      id: 'annotation-source',
      name: 'Apple Books 批注',
      kind: 'annotations',
      locator: 'apple-books',
    }
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const url = String(input)
      if (url.endsWith('/api/v1/sources') && !init?.method) {
        return new Response(JSON.stringify([source, annotationSource]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.includes('/api/v1/entries?') && url.includes('source_id=annotation-source')) {
        return new Response(JSON.stringify(Array.from({ length: 4 }, (_, index) => ({
          id: `annotation-entry-${index}`,
          source_id: 'annotation-source',
          identity_key: `external:apple-books:book-123:${1284 + index}`,
          source_title: '思考，快与慢',
          display_title: '',
          author: 'Daniel Kahneman',
          summary: index === 3 ? '第四条可展开的批注。' : '系统一自动而快速地运行。',
          content_html: '<blockquote>系统一自动而快速地运行。</blockquote>',
          discovered_at: '2026-07-27T10:00:00Z',
          note: '',
          annotation: {
            provider: 'apple-books',
            book_identity: 'book-123',
            book_title: '思考，快与慢',
            book_author: 'Daniel Kahneman',
            chapter: '第三章',
            location: String(1284 + index),
            highlight_color: 'yellow',
            annotation_note: '这里对应直觉判断。',
          },
        }))), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/sources/annotation-source/annotations') && init?.method === 'POST') {
        return new Response('{"id":"annotation-job","status":"pending"}', {
          status: 202,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return defaultFetch(input, init)
    })

    render(<App />)
    await screen.findByText('Reader article')
    fireEvent.click(screen.getByRole('link', { name: '阅读笔记' }))

    expect(await screen.findByRole('heading', { name: '阅读笔记' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: '思考，快与慢' })).toBeInTheDocument()
    expect(screen.getByText('4 条批注')).toBeInTheDocument()
    expect(screen.queryByText('第四条可展开的批注。')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '展开全部 4 条' }))
    expect(screen.getByText('第四条可展开的批注。')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '导入批注' }))
    fireEvent.change(screen.getByLabelText('来源平台'), { target: { value: 'apple-books' } })
    fireEvent.change(screen.getByLabelText('书名'), { target: { value: '原则' } })
    fireEvent.change(screen.getByLabelText('作者'), { target: { value: 'Ray Dalio' } })
    fireEvent.change(screen.getByLabelText('高亮原文'), { target: { value: '可信度加权决策。' } })
    fireEvent.click(screen.getByRole('button', { name: '加入导入队列' }))

    await waitFor(() => {
      const request = vi.mocked(fetch).mock.calls.find(([url, init]) =>
        String(url).endsWith('/api/v1/sources/annotation-source/annotations') &&
        init?.method === 'POST')
      expect(request).toBeDefined()
      expect(String(request?.[1]?.body)).toContain('可信度加权决策')
    })
  })

  it('saves a bookmarklet URL into an existing Manual Source', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    const manualSource = {
      ...source,
      id: 'manual-source',
      name: '网页收藏',
      kind: 'manual',
      locator: 'reading-list',
      normalized_locator: 'reading-list',
    }
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith('/api/v1/sources') && !init?.method) {
        return new Response(JSON.stringify([source, manualSource]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return defaultFetch(input, init)
    })
    window.history.replaceState(null, '', '/#save?url=https%3A%2F%2Fexample.com%2Fstory&title=Saved%20Story')

    render(<App />)

    expect(await screen.findByRole('heading', { name: '保存到 Pulse' })).toBeInTheDocument()
    await waitFor(() => expect(window.location.hash).toBe(''))
    expect(screen.getByLabelText('网页地址')).toHaveValue('https://example.com/story')
    expect(screen.getByLabelText('标题')).toHaveValue('Saved Story')
    expect(screen.getByLabelText('保存到')).toHaveValue('manual-source')
    fireEvent.click(screen.getByRole('button', { name: '保存网页' }))

    expect(await screen.findByRole('status')).toHaveTextContent('已加入保存队列')
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/sources/manual-source/entries',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          url: 'https://example.com/story',
          title: 'Saved Story',
        }),
      }),
    )
  })

  it('explicitly creates a Manual Source before saving when none exists', async () => {
    window.history.replaceState(null, '', '/#save?url=https%3A%2F%2Fexample.com%2Fnew&title=New')

    render(<App />)
    await screen.findByRole('heading', { name: '保存到 Pulse' })
    fireEvent.click(screen.getByRole('button', { name: '创建“网页收藏”并保存' }))

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/v1/sources', expect.objectContaining({
        method: 'POST',
      }))
      expect(fetch).toHaveBeenCalledWith(
        '/api/v1/sources/source-2/entries',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    expect(screen.getByRole('status')).toHaveTextContent('已创建“网页收藏”并加入保存队列')
    const createCall = vi.mocked(fetch).mock.calls.find(([url, init]) =>
      String(url).endsWith('/api/v1/sources') && init?.method === 'POST')
    expect(JSON.parse(String(createCall?.[1]?.body)).locator).toMatch(/^reading-list-/)
  })

  it('re-enables a paused Manual Source instead of creating a conflicting one', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    const pausedSource = {
      ...source,
      id: 'manual-paused',
      name: '旧网页收藏',
      kind: 'manual',
      locator: 'reading-list',
      normalized_locator: 'reading-list',
      enabled: false,
    }
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/sources') && !init?.method) {
        return new Response(JSON.stringify([source, pausedSource]), { status: 200 })
      }
      if (url.endsWith('/api/v1/sources/manual-paused') && init?.method === 'PATCH') {
        return new Response(JSON.stringify({ ...pausedSource, enabled: true }), { status: 200 })
      }
      return defaultFetch(input, init)
    })
    window.history.replaceState(null, '', '/#save?url=https%3A%2F%2Fexample.com%2Fpaused&title=Paused')

    render(<App />)
    await screen.findByRole('heading', { name: '保存到 Pulse' })
    expect(screen.getByLabelText('保存到')).toHaveValue('manual-paused')
    fireEvent.click(screen.getByRole('button', { name: '保存网页' }))

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/v1/sources/manual-paused', expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ enabled: true }),
      }))
      expect(fetch).toHaveBeenCalledWith(
        '/api/v1/sources/manual-paused/entries',
        expect.objectContaining({ method: 'POST' }),
      )
    })
  })

  it('refreshes the save form when a reused popup receives another page', async () => {
    window.history.replaceState(null, '', '/#save?url=https%3A%2F%2Fexample.com%2Ffirst&title=First')
    render(<App />)
    await screen.findByRole('heading', { name: '保存到 Pulse' })

    window.location.hash = '#save?url=https%3A%2F%2Fexample.com%2Fsecond&title=Second'
    window.dispatchEvent(new HashChangeEvent('hashchange'))

    await waitFor(() => {
      expect(screen.getByLabelText('网页地址')).toHaveValue('https://example.com/second')
      expect(screen.getByLabelText('标题')).toHaveValue('Second')
    })
  })

  it('rejects unsafe bookmarklet URLs before submitting', async () => {
    window.history.replaceState(null, '', '/#save?url=javascript%3Aalert(1)&title=Unsafe')

    render(<App />)
    await screen.findByRole('heading', { name: '保存到 Pulse' })
    fireEvent.click(screen.getByRole('button', { name: '创建“网页收藏”并保存' }))

    expect(screen.getByRole('alert')).toHaveTextContent('只支持 HTTP 或 HTTPS 网页地址')
    expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).endsWith('/entries'))).toBe(false)
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

  it('treats a null folder list as an empty list', async () => {
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/folders')) {
        return new Response('null', {
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
      if (url.includes('/api/v1/entries')) {
        return new Response('[]', {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      throw new Error(`unexpected request: ${url}`)
    })

    render(<App />)

    expect(await screen.findByText('尚未创建文件夹')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Example Feed' })).toBeInTheDocument()
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
