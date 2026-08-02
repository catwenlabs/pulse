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
let sourceState = [source]
let folderState = [{
  id: 'folder-1',
  name: 'Tech',
  source_count: 1,
  source_ids: ['source-1'],
}]
const scrollIntoView = vi.fn()
const scrollTo = vi.fn()

class FakeEventSource {
  static instances: FakeEventSource[] = []
  readonly listeners = new Map<string, Array<(event: MessageEvent<string>) => void>>()
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  readonly close = vi.fn()

  constructor(readonly url: string) {
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: MessageEvent<string>) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener])
  }

  open() {
    this.onopen?.()
  }

  emit(type: string, data: string) {
    const event = new MessageEvent<string>(type, { data })
    for (const listener of this.listeners.get(type) ?? []) listener(event)
  }
}

function realtimeStory(id: string, title: string) {
  const entryID = `entry-${id}`
  return {
    id,
    display_title: title,
    representative: {
      id: entryID,
      source_id: 'source-1',
      identity_key: `external:${entryID}`,
      source_title: title,
      summary: `${title} summary`,
      content_html: `<p>${title} body</p>`,
      discovered_at: '2026-07-25T00:00:00Z',
    },
    entry_count: 1,
    source_count: 1,
  }
}

function realtimePage(stories: ReturnType<typeof realtimeStory>[]) {
  return {
    stories,
    total_stories: stories.length,
    reader_counts: {
      inbox_stories: stories.length,
      unread_stories: stories.length,
      starred_stories: 0,
      later_stories: 0,
      hidden_stories: 0,
    },
  }
}

describe('App', () => {
  beforeEach(() => {
    FakeEventSource.instances = []
    sourceState = [source]
    folderState = [{
      id: 'folder-1',
      name: 'Tech',
      source_count: 1,
      source_ids: ['source-1'],
    }]
    window.history.replaceState(null, '', '/')
    scrollIntoView.mockClear()
    scrollTo.mockClear()
    Object.defineProperty(Element.prototype, 'scrollIntoView', {
      configurable: true,
      value: scrollIntoView,
    })
    Object.defineProperty(Element.prototype, 'scrollTo', {
      configurable: true,
      value: scrollTo,
    })
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(0)
      return 1
    })
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/healthz')) {
        return new Response(JSON.stringify({ status: 'ok' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.includes('/api/v1/sources/') && url.endsWith('/entries') && init?.method === 'POST') {
        return new Response(JSON.stringify({ id: 'manual-job', status: 'pending' }), {
          status: 202,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.includes('/api/v1/stories')) {
        const representative = {
          id: 'entry-1',
          source_id: 'source-1',
          identity_key: 'external:entry-1',
          source_title: 'Reader article',
          author: 'Ada',
          summary: 'A useful summary',
          content_html: '<h2>Section title</h2><p>Article body</p><img src="https://images.example/cover.jpg" alt="Cover"><script>alert("unsafe")</script>',
          discovered_at: '2026-07-25T00:00:00Z',
        }
        if (init?.method === 'PATCH' && /\/stories\/[^?]+$/.test(url)) {
          const patch = JSON.parse(String(init.body)) as {
            read?: boolean
            starred?: boolean
            later?: boolean
            display_title?: string
            note?: string
          }
          return new Response(JSON.stringify({
            id: 'story-1',
            display_title: patch.display_title ?? '',
            note: patch.note ?? '',
            read_at: patch.read === false ? undefined : '2026-07-25T01:00:00Z',
            starred_at: patch.starred ? '2026-07-25T01:00:00Z' : undefined,
            later_at: patch.later ? '2026-07-25T01:00:00Z' : undefined,
            representative,
            entries: [representative],
            entry_count: 1,
            source_count: 1,
          }), { status: 200, headers: { 'Content-Type': 'application/json' } })
        }
        if (init?.method === 'PATCH') {
          return new Response(JSON.stringify({ updated_count: 1 }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          })
        }
        return new Response(JSON.stringify(
          /\/stories\/[^?]+$/.test(url)
            ? {
                id: 'story-1',
                representative,
                entries: [representative],
                entry_count: 1,
                source_count: 1,
              }
            : {
                stories: [{
                  id: 'story-1',
                  representative,
                  entry_count: 1,
                  source_count: 1,
                }],
                total_stories: 1,
                reader_counts: {
                  inbox_stories: 1,
                  unread_stories: 1,
                  starred_stories: 0,
                  later_stories: 0,
                  hidden_stories: 0,
                },
              },
        ), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      if (url.includes('/api/v1/sources/') && url.endsWith('/entries')) {
        return new Response(JSON.stringify({
          entries: [{
            entry: {
              id: 'entry-1',
              source_id: 'source-1',
              identity_key: 'external:entry-1',
              source_title: 'Reader article',
              author: 'Ada',
              summary: 'A useful summary',
              content_html: '<h2>Section title</h2><p>Article body</p><img src="https://images.example/cover.jpg" alt="Cover"><script>alert("unsafe")</script>',
              discovered_at: '2026-07-25T00:00:00Z',
            },
            story: {
              id: 'story-1',
            },
          }],
          total_entries: 1,
          reader_counts: {
            inbox_stories: 1,
            unread_stories: 1,
            starred_stories: 0,
            later_stories: 0,
            hidden_stories: 0,
          },
        }), { status: 200, headers: { 'Content-Type': 'application/json' } })
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
          author: 'Ada',
          summary: 'A useful summary',
          content_html: '<h2>Section title</h2><p>Article body</p><img src="https://images.example/cover.jpg" alt="Cover"><script>alert("unsafe")</script>',
          discovered_at: '2026-07-25T00:00:00Z',
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
      if (url.endsWith('/api/v1/folders') && init?.method === 'POST') {
        const created = {
          id: 'folder-2',
          name: JSON.parse(String(init.body)).name,
          source_count: 0,
          source_ids: [],
        }
        folderState = [...folderState, created]
        return new Response(JSON.stringify(created), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.includes('/api/v1/folders/') && url.includes('/sources/')) {
        const [, folderID, sourceID] = url.match(/folders\/([^/]+)\/sources\/([^/]+)/) ?? []
        folderState = folderState.map((folder) => {
          if (folder.id !== folderID) return folder
          const sourceIDs = init?.method === 'PUT'
            ? [...new Set([...folder.source_ids, sourceID])]
            : folder.source_ids.filter((id) => id !== sourceID)
          return { ...folder, source_ids: sourceIDs, source_count: sourceIDs.length }
        })
        return new Response(null, { status: 204 })
      }
      if (url.endsWith('/api/v1/folders')) {
        return new Response(JSON.stringify(folderState), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/api/v1/sources')) {
        return new Response(JSON.stringify(sourceState), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (init?.method === 'PATCH') {
        const patch = JSON.parse(String(init.body)) as { enabled?: boolean; name?: string; locator?: string }
        return new Response(JSON.stringify({ ...source, ...patch }), {
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
    delete (Element.prototype as { scrollTo?: unknown }).scrollTo
  })

  it('shows folders, subscriptions, and an expandable article stream', async () => {
    render(<App />)

    expect(screen.getByText('正在同步信息源…')).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: '全部文章' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: 'Example Feed' })).toHaveAttribute('title', 'Example Feed')
    const folder = screen.getByRole('button', { name: 'Tech，1 个订阅源' })
    expect(folder).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(folder)
    expect(screen.queryByRole('button', { name: 'Example Feed' })).not.toBeInTheDocument()
    fireEvent.click(folder)
    expect(screen.getByRole('button', { name: 'Example Feed' })).toBeInTheDocument()
    fireEvent.click(await screen.findByText('Reader article'))
    expect(screen.getByText('Article body')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Section title' })).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Cover' })).toHaveAttribute('loading', 'lazy')
    expect(document.querySelector('.entry-prose script')).toBeNull()
    expect(screen.getByRole('button', { name: '更多操作' })).toBeInTheDocument()
  })

  it('persists independent drag order for folders, root Sources, and Folder Sources', async () => {
    sourceState = [
      { ...source, id: 'source-1', name: 'Folder One' },
      { ...source, id: 'source-2', name: 'Folder Two' },
      { ...source, id: 'source-3', name: 'Root One' },
      { ...source, id: 'source-4', name: 'Root Two' },
    ]
    folderState = [
      { id: 'folder-1', name: 'Tech', source_count: 2, source_ids: ['source-1', 'source-2'] },
      { id: 'folder-2', name: 'Reading', source_count: 1, source_ids: ['source-1'] },
    ]
    render(<App />)
    await screen.findByRole('button', { name: 'Tech，2 个订阅源' })
    expect(screen.getAllByTitle('Folder One')).toHaveLength(2)

    const dataTransfer = {
      effectAllowed: '',
      dropEffect: '',
      setData: vi.fn(),
      getData: vi.fn(),
    }
    const dragBefore = (dragged: HTMLElement, target: HTMLElement) => {
      fireEvent.dragStart(dragged, { dataTransfer })
      fireEvent.dragOver(target, { dataTransfer })
      expect(screen.getByRole('separator', { name: '放置于此' })).toBeVisible()
      fireEvent.drop(target, { dataTransfer })
      expect(screen.queryByRole('separator', { name: '放置于此' })).not.toBeInTheDocument()
    }

    dragBefore(
      screen.getByRole('button', { name: 'Reading，1 个订阅源' }),
      screen.getByRole('button', { name: 'Tech，2 个订阅源' }),
    )
    await waitFor(() => {
      expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/folders/order', expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ folder_ids: ['folder-2', 'folder-1'] }),
      }))
    })

    dragBefore(
      screen.getAllByTitle('Folder Two')[0],
      screen.getAllByTitle('Folder One')[0],
    )
    await waitFor(() => {
      expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/folders/folder-1/sources/order', expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ source_ids: ['source-2', 'source-1'] }),
      }))
    })

    dragBefore(
      screen.getByTitle('Root Two'),
      screen.getByTitle('Root One'),
    )
    await waitFor(() => {
      expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/sources/order', expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ source_ids: ['source-4', 'source-3'] }),
      }))
    })
  })

  it('organizes a source into existing and newly created folders', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })

    fireEvent.click(screen.getByRole('button', { name: '管理信息源' }))
    fireEvent.click(screen.getByRole('button', { name: '整理 Example Feed 到文件夹' }))

    const tech = screen.getByRole('checkbox', { name: /Tech/ })
    expect(tech).toBeChecked()
    fireEvent.click(tech)
    fireEvent.change(screen.getByLabelText('新建并加入文件夹（可选）'), {
      target: { value: 'Reading' },
    })
    fireEvent.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('已整理 Example Feed')).toBeInTheDocument()
    expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/folders', expect.objectContaining({
      method: 'POST',
    }))
    expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/folders/folder-2/sources/source-1', {
      method: 'PUT',
    })
    expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/folders/folder-1/sources/source-1', {
      method: 'DELETE',
    })
    expect(screen.getByRole('button', { name: 'Reading，1 个订阅源' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Tech，0 个订阅源' })).toBeInTheDocument()
  })

  it('debounces article search and highlights visible matches', async () => {
    render(<App />)
    await screen.findByText('Reader article')

    fireEvent.change(screen.getByLabelText('搜索文章'), { target: { value: 'Reader' } })

    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some(([url]) =>
        String(url).includes('q=Reader'))).toBe(true)
    })
    expect(screen.getByText('Reader', { selector: 'mark' })).toBeInTheDocument()
  })

  it('creates an RSS source from the dialog', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })

    fireEvent.click(screen.getByRole('button', { name: '添加信息源' }))
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'New Feed' } })
    fireEvent.change(screen.getByLabelText('Feed 地址'), {
      target: { value: 'https://new.example/feed' },
    })
    fireEvent.click(screen.getByRole('checkbox', { name: /Tech/ }))
    fireEvent.click(screen.getByRole('button', { name: '测试并预览' }))
    expect(await screen.findByText('Preview article')).toBeInTheDocument()
    expect(screen.getByText('external:article-1')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '保存并启用' }))

    expect(await screen.findByText('New Feed')).toBeInTheDocument()
    expect(screen.getByText('已添加 New Feed')).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/folders/folder-1/sources/source-2', {
      method: 'PUT',
    })
  })

  it('keeps a newly assigned source in its folder when folder refresh fails', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })
    const baseFetch = vi.mocked(fetch).getMockImplementation()!
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const url = String(input)
      if (url.endsWith('/api/v1/folders') && (!init?.method || init.method === 'GET')) {
        throw new Error('refresh failed')
      }
      return baseFetch(input, init)
    })

    fireEvent.click(screen.getByRole('button', { name: '添加信息源' }))
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'New Feed' } })
    fireEvent.change(screen.getByLabelText('Feed 地址'), {
      target: { value: 'https://new.example/feed' },
    })
    fireEvent.click(screen.getByRole('checkbox', { name: /Tech/ }))
    fireEvent.click(screen.getByRole('button', { name: '测试并预览' }))
    await screen.findByText('Preview article')
    fireEvent.click(screen.getByRole('button', { name: '保存并启用' }))

    expect(await screen.findByText('已添加 New Feed')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Tech，2 个订阅源' })).toBeInTheDocument()
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

  it('edits a source name and locator', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })
    fireEvent.click(screen.getByRole('button', { name: '管理信息源' }))

    fireEvent.click(screen.getByRole('button', { name: '编辑 Example Feed' }))
    const dialog = screen.getByRole('dialog', { name: '编辑信息源' })
    fireEvent.change(within(dialog).getByLabelText('名称'), { target: { value: 'Renamed Feed' } })
    fireEvent.change(within(dialog).getByLabelText('Feed 地址'), { target: { value: 'https://example.com/new.xml' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '保存修改' }))

    expect(await screen.findByText('已更新 Renamed Feed')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Renamed Feed' })).toBeInTheDocument()
    expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/sources/source-1', expect.objectContaining({
      method: 'PATCH',
      body: '{"name":"Renamed Feed","locator":"https://example.com/new.xml"}',
    }))
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
    expect(await screen.findByText('已删除 Example Feed')).toBeInTheDocument()
    expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/sources/source-1', { method: 'DELETE' })
  })

  it('scrolls the reading area to the top and marks an unread entry as read when expanded', async () => {
    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })

    expect(await screen.findByText('Reader article')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Reader article'))
    expect(screen.getByText('Article body')).toBeInTheDocument()
    const detail = document.querySelector<HTMLElement>('[data-entry-detail="entry-1"]')!
    expect(detail).toHaveClass('min-h-full')
    expect(detail).toHaveClass('max-md:pt-0')
    expect(detail.closest('[aria-label="文章列表"]')?.parentElement).toHaveClass('h-full')
    expect(detail.closest('[aria-label="文章列表"]')?.parentElement).not.toHaveClass('max-md:h-auto')
    expect(screen.getByRole('button', { name: '关闭文章' })).toBeInTheDocument()
    expect(scrollTo).toHaveBeenCalledWith(expect.objectContaining({ behavior: 'smooth' }))
    expect(scrollTo.mock.contexts.at(-1)).toHaveAttribute('aria-label', '文章列表')
    expect(scrollIntoView).not.toHaveBeenCalled()

    await waitFor(() => {
      const patches = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).endsWith('/api/v1/stories/story-1') && init?.method === 'PATCH')
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
      const storyPatches = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).endsWith('/api/v1/stories/story-1') && init?.method === 'PATCH')
      expect(storyPatches).toHaveLength(4)
    })

    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.click(screen.getByRole('menuitem', { name: '编辑标题与笔记' }))
    expect(screen.queryByLabelText('笔记')).not.toBeInTheDocument()
    const entryRow = document.querySelector<HTMLElement>('[data-entry-row="entry-1"]')!
    fireEvent.click(within(entryRow).getByRole('button', { expanded: true }))
    expect(screen.queryByText('Article body')).not.toBeInTheDocument()
    fireEvent.click(within(entryRow).getByRole('button', { expanded: false }))
    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.keyDown(document, { key: 'Escape' })
    await waitFor(() => {
      const storyPatches = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).endsWith('/api/v1/stories/story-1') && init?.method === 'PATCH')
      expect(storyPatches).toHaveLength(4)
    })
  })

  it('splits a member entry out of a story and merges the story into another', async () => {
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
    const first = {
      id: 'entry-1', source_id: 'source-1', identity_key: 'x:1', source_title: 'First story',
      display_title: '', author: 'Ada', summary: '', content_html: '<p>body one</p>',
      discovered_at: '2026-07-25T00:00:00Z', read_at: '2026-07-25T00:00:00Z', note: '',
    }
    const alt = {
      id: 'entry-2', source_id: 'source-2', identity_key: 'x:2', source_title: 'Alt source',
      display_title: '', author: 'Bo', summary: '', content_html: '<p>body two</p>',
      discovered_at: '2026-07-25T00:00:00Z', note: '',
    }
    const second = {
      id: 'entry-3', source_id: 'source-3', identity_key: 'x:3', source_title: 'Second story',
      display_title: '', author: 'Cy', summary: '', content_html: '<p>body three</p>',
      discovered_at: '2026-07-25T00:00:00Z', read_at: '2026-07-25T00:00:00Z', note: '',
    }
    let mergeAttempts = 0
    const folder = { id: 'folder-1', name: 'Tech', source_count: 1, source_ids: ['source-1'] }
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/healthz')) return json({ status: 'ok' })
      if (url.endsWith('/api/v1/folders')) return json([folder])
      if (url.endsWith('/api/v1/sources')) return json([source])
      if (url.includes('/stories/story-1/representative')) return json({ id: 'story-1', representative: alt, entries: [first, alt], entry_count: 2, source_count: 2 })
      if (url.includes('/stories/story-1/merge')) {
        mergeAttempts += 1
        if (mergeAttempts === 1) return json({ code: 'story_metadata_conflict', detail: 'metadata conflict' }, 409)
        return json({ id: 'story-2', representative: second, entries: [second], entry_count: 1, source_count: 1 })
      }
      if (url.includes('/stories/story-1/split')) return json({ id: 'story-3', representative: alt, entries: [alt], entry_count: 1, source_count: 1 })
      if (/\/api\/v1\/stories\/story-1$/.test(url)) return json({ id: 'story-1', representative: first, entries: [first, alt], entry_count: 2, source_count: 2 })
      if (url.includes('/api/v1/stories')) return json({
        stories: [
          { id: 'story-1', representative: first, entry_count: 2, source_count: 2 },
          { id: 'story-2', representative: second, entry_count: 1, source_count: 1 },
        ],
        total_stories: 2,
        reader_counts: {
          inbox_stories: 2,
          unread_stories: 0,
          starred_stories: 0,
          later_stories: 0,
          hidden_stories: 0,
        },
      })
      return json([])
    }))

    render(<App />)
    await screen.findByRole('button', { name: 'Example Feed' })
    await screen.findByText('First story')

    fireEvent.click(screen.getByText('First story'))
    fireEvent.click(await screen.findByRole('button', { name: '设为默认来源 Bo' }))
    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some(([url, init]) =>
        String(url).includes('/stories/story-1/representative') && init?.method === 'PUT')).toBe(true)
    })
    fireEvent.click(await screen.findByRole('button', { name: '分开 Alt source' }))
    fireEvent.click(screen.getByLabelText('复制显示标题'))
    fireEvent.click(screen.getByRole('button', { name: '确认拆分' }))
    await waitFor(() => {
      const splitCalls = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).includes('/stories/story-1/split') && init?.method === 'POST')
      expect(splitCalls).toHaveLength(1)
      expect(JSON.parse(String(splitCalls[0][1]?.body))).toEqual({ entry_id: 'entry-2', copy_display_title: true, move_display_title: false })
    })

    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '合并到其他 Story' }))
    fireEvent.click(screen.getByRole('button', { name: '合并到：Second story' }))
    await screen.findByLabelText('合并后的显示标题')
    fireEvent.click(screen.getByRole('button', { name: '确认合并' }))
    await waitFor(() => {
      const mergeCalls = vi.mocked(fetch).mock.calls.filter(([url, init]) =>
        String(url).includes('/stories/story-1/merge') && init?.method === 'POST')
      expect(mergeCalls).toHaveLength(2)
      expect(JSON.parse(String(mergeCalls[0][1]?.body))).toEqual({ into: 'story-2' })
      expect(JSON.parse(String(mergeCalls[1][1]?.body))).toEqual({ into: 'story-2', display_title: '', note: '' })
    })
  })

  it('requires Story metadata confirmation before deleting a final Entry', async () => {
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
    const representative = {
      id: 'entry-1', source_id: 'source-1', identity_key: 'x:1', source_title: 'Reader article',
      discovered_at: '2026-07-25T00:00:00Z', content_html: '<p>body</p>',
    }
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/healthz')) return json({ status: 'ok' })
      if (url.endsWith('/api/v1/folders')) return json([])
      if (url.endsWith('/api/v1/sources')) return json([source])
      if (url.endsWith('/api/v1/sources/source-1/health')) return json({ source_id: 'source-1', status: 'ok' })
      if (url.endsWith('/api/v1/entries/entry-1') && init?.method === 'DELETE') {
        if (!url.includes('confirm=true')) return json({
          code: 'confirmation_required',
          detail: 'metadata loss',
          story_id: 'story-1',
          display_title: 'Saved title',
          note: 'Saved note',
          entry_count: 1,
        }, 409)
        return new Response(null, { status: 204 })
      }
      if (url.endsWith('/api/v1/stories/story-1')) return json({
        id: 'story-1', representative, entries: [representative], entry_count: 1, source_count: 1,
      })
      if (url.includes('/api/v1/stories')) return json({
        stories: [{ id: 'story-1', representative, entry_count: 1, source_count: 1 }],
        total_stories: 1,
        reader_counts: { inbox_stories: 1, unread_stories: 1, starred_stories: 0, later_stories: 0, hidden_stories: 0 },
      })
      return json([])
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<App />)
    await screen.findByText('Reader article')
    fireEvent.click(screen.getByText('Reader article'))
    fireEvent.pointerDown(screen.getByRole('button', { name: '更多操作' }), { button: 0 })
    fireEvent.click(await screen.findByRole('menuitem', { name: '永久删除来源内容' }))
    fireEvent.click(screen.getByRole('button', { name: '确认永久删除' }))

    await screen.findByText(/这会同时删除 Story「Saved title」及其笔记/)
    expect(fetchMock.mock.calls.filter(([url, init]) => String(url).includes('/api/v1/entries/entry-1') && init?.method === 'DELETE')).toHaveLength(1)
    fireEvent.click(screen.getByRole('button', { name: '确认永久删除' }))
    await waitFor(() => {
      const deletes = fetchMock.mock.calls.filter(([url, init]) => String(url).includes('/api/v1/entries/entry-1') && init?.method === 'DELETE')
      expect(deletes).toHaveLength(2)
      expect(String(deletes[1][0])).toContain('confirm=true')
    })
  })

  it('collapses the expanded entry when Escape is pressed', async () => {
    render(<App />)
    await screen.findByText('Reader article')

    fireEvent.click(screen.getByText('Reader article'))
    expect(screen.getByText('Article body')).toBeInTheDocument()
    scrollTo.mockClear()
    fireEvent.keyDown(document, { key: 'Escape' })

    expect(screen.queryByText('Article body')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument()
    expect(scrollTo).toHaveBeenCalledWith(expect.objectContaining({ behavior: 'smooth' }))
    expect(scrollTo.mock.contexts.at(-1)).toHaveAttribute('aria-label', '文章列表')
    expect(scrollIntoView).not.toHaveBeenCalled()
  })

  it('closes the expanded entry with the visible close button', async () => {
    render(<App />)
    await screen.findByText('Reader article')

    fireEvent.click(screen.getByText('Reader article'))
    expect(screen.getByText('Article body')).toBeInTheDocument()
    scrollTo.mockClear()
    fireEvent.click(screen.getByRole('button', { name: '关闭文章' }))

    expect(screen.queryByText('Article body')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { expanded: false })).toHaveFocus()
    expect(scrollTo).toHaveBeenCalledWith(expect.objectContaining({ behavior: 'smooth' }))
  })

  it('filters the stream when a subscription is selected', async () => {
    render(<App />)

    fireEvent.click(await screen.findByRole('button', { name: 'Example Feed' }))

    expect(await screen.findByRole('heading', { name: 'Example Feed' })).toBeInTheDocument()
    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some(([url]) =>
        String(url).includes('/api/v1/sources/source-1/entries?') && !String(url).includes('source_id='))).toBe(true)
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
    expect(screen.getByRole('button', { name: '更多导航' })).toHaveFocus()
    fireEvent.keyDown(screen.getByRole('button', { name: '更多导航' }), { key: 'Tab' })
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
    fireEvent.pointerDown(screen.getByRole('button', { name: '更多导航' }), { button: 0 })
    fireEvent.click(screen.getByRole('menuitem', { name: '安装保存书签' }))
    const dialog = screen.getByRole('dialog', { name: '安装“保存到 Pulse”' })
    fireEvent.keyDown(dialog, { key: 'Escape' })

    expect(dialog).not.toBeInTheDocument()
    expect(menuButton).toHaveFocus()
  })

  it('navigates from the compact mobile menu and dismisses only the menu on Escape', async () => {
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
    const more = screen.getByRole('button', { name: '更多导航' })

    fireEvent.pointerDown(more, { button: 0 })
    expect(screen.getByRole('menuitem', { name: '收藏' })).toBeInTheDocument()
    expect(await screen.findByRole('status')).toHaveTextContent('本地服务已连接')
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByRole('menuitem', { name: '收藏' })).not.toBeInTheDocument()
    expect(document.getElementById('mobile-navigation')).not.toBeNull()

    fireEvent.keyDown(more, { key: 'ArrowDown' })
    fireEvent.click(await screen.findByRole('menuitem', { name: '收藏' }))
    expect(document.getElementById('mobile-navigation')).toBeNull()
    await waitFor(() => expect(menuButton).toHaveFocus())
    expect(await screen.findByRole('heading', { name: '收藏' })).toBeInTheDocument()
    fireEvent.click(menuButton)
    fireEvent.pointerDown(screen.getByRole('button', { name: '更多导航' }), { button: 0 })
    expect(screen.getByRole('menuitem', { name: '收藏' })).toHaveAttribute('aria-current', 'page')
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
      if (url.includes('/api/v1/sources/annotation-source/entries')) {
        return new Response(JSON.stringify({
          entries: Array.from({ length: 4 }, (_, index) => ({
            entry: {
            id: `annotation-entry-${index}`,
            source_id: 'annotation-source',
            identity_key: `external:apple-books:book-123:${1284 + index}`,
            source_title: '思考，快与慢',
            author: 'Daniel Kahneman',
            summary: index === 3 ? '第四条可展开的批注。' : '系统一自动而快速地运行。',
            content_html: '<blockquote>系统一自动而快速地运行。</blockquote>',
            discovered_at: '2026-07-27T10:00:00Z',
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
          },
            story: { id: `story-${index}` },
          })),
          total_entries: 4,
          reader_counts: {
            inbox_stories: 4,
            unread_stories: 4,
            starred_stories: 0,
            later_stories: 0,
            hidden_stories: 0,
          },
        }), {
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
      expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/stories', expect.objectContaining({
        method: 'PATCH',
        body: JSON.stringify({ read: true }),
      }))
    })

    fireEvent.click(screen.getByRole('button', { name: 'Example Feed' }))
    await screen.findByRole('heading', { name: 'Example Feed' })
    fireEvent.click(screen.getByRole('button', { name: '将 Example Feed 全部标记为已读' }))
    await waitFor(() => {
      expect(vi.mocked(fetch)).toHaveBeenCalledWith('/api/v1/stories?source_id=source-1', expect.objectContaining({
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

  it('shows unfiled sources at the root when the folder list is null', async () => {
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

    expect(await screen.findByRole('button', { name: 'Example Feed' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Tech，1 个订阅源' })).not.toBeInTheDocument()
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

  it('shows an unread count next to each source and hides it when zero', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/sources') && !init?.method) {
        return new Response(JSON.stringify([
          { ...source, unread_count: 7 },
          { ...source, id: 'source-2', name: 'Quiet Feed', unread_count: 0 },
        ]), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      return defaultFetch(input, init)
    })

    render(<App />)
    const sidebar = document.querySelector('aside')!
    const feed = await within(sidebar).findByRole('button', { name: /Example Feed/ })
    expect(within(feed).getByText('7')).toBeInTheDocument()
    const quiet = within(sidebar).getByRole('button', { name: /Quiet Feed/ })
    expect(within(quiet).queryByText('0')).not.toBeInTheDocument()
  })

  it('shows the total unread count on the all-articles item', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/sources') && !init?.method) {
        return new Response(JSON.stringify([
          { ...source, unread_count: 3 },
          { ...source, id: 'source-2', name: 'Second Feed', unread_count: 4 },
        ]), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      return defaultFetch(input, init)
    })

    render(<App />)
    const sidebar = document.querySelector('aside')!
    const allArticles = await within(sidebar).findByRole('link', { name: /全部文章/ })
    expect(within(allArticles).getByText('7')).toBeInTheDocument()
    await waitFor(() => expect(document.title).toBe('(7) Pulse'))
  })

  it('refreshes source counts after marking an entry read', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/sources') && !init?.method) {
        return new Response(JSON.stringify([{ ...source, unread_count: 5 }]), {
          status: 200, headers: { 'Content-Type': 'application/json' },
        })
      }
      return defaultFetch(input, init)
    })

    const sourcesGets = () => vi.mocked(fetch).mock.calls.filter(([url, init]) =>
      String(url).endsWith('/api/v1/sources') && !init?.method).length

    render(<App />)
    await screen.findByText('Reader article')
    expect(sourcesGets()).toBe(1)

    fireEvent.click(screen.getByText('Reader article'))
    await waitFor(() => {
      expect(vi.mocked(fetch).mock.calls.some(([url, init]) =>
        String(url).endsWith('/api/v1/stories/story-1') && init?.method === 'PATCH')).toBe(true)
    })
    await waitFor(() => expect(sourcesGets()).toBeGreaterThanOrEqual(2))
  })

  it('applies a visible library change without showing a prompt when the list is at the top', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    let includeNewStory = false
    vi.stubGlobal('EventSource', FakeEventSource)
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes('/api/v1/stories') && !init?.method) {
        const stories = includeNewStory
          ? [realtimeStory('story-2', 'New article'), realtimeStory('story-1', 'Existing article')]
          : [realtimeStory('story-1', 'Existing article')]
        return new Response(JSON.stringify(realtimePage(stories)), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return defaultFetch(input, init)
    })

    render(<App />)
    expect(await screen.findByText('Existing article')).toBeInTheDocument()
    const eventSource = FakeEventSource.instances.at(-1)!
    eventSource.open()
    includeNewStory = true
    eventSource.emit('library-change', JSON.stringify({ source_id: 'source-1' }))

    expect(await screen.findByText('New article')).toBeInTheDocument()
    expect(screen.queryByText('有 1 条新内容')).not.toBeInTheDocument()
  })

  it('does not count a new Source Entry joined to an existing Story as new content', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    let includeJoinedEntry = false
    vi.stubGlobal('EventSource', FakeEventSource)
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes('/api/v1/sources/source-1/entries') && !init?.method) {
        const makeEntry = (id: string, title: string) => ({
          entry: {
            id,
            source_id: 'source-1',
            identity_key: `external:${id}`,
            source_title: title,
            content_html: `<p>${title} body</p>`,
            discovered_at: '2026-07-25T00:00:00Z',
          },
          story: { id: 'story-1', entry_count: includeJoinedEntry ? 2 : 1, source_count: 1 },
        })
        const entries = includeJoinedEntry
          ? [makeEntry('entry-2', 'Joined version'), makeEntry('entry-1', 'Existing article')]
          : [makeEntry('entry-1', 'Existing article')]
        return new Response(JSON.stringify({
          entries,
          total_entries: entries.length,
          reader_counts: {
            inbox_stories: 1,
            unread_stories: 1,
            starred_stories: 0,
            later_stories: 0,
            hidden_stories: 0,
          },
        }), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      return defaultFetch(input, init)
    })

    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: 'Example Feed' }))
    expect(await screen.findByText('Existing article')).toBeInTheDocument()
    const eventSource = FakeEventSource.instances.at(-1)!
    eventSource.open()
    includeJoinedEntry = true
    eventSource.emit('library-change', JSON.stringify({ source_id: 'source-1' }))

    expect(await screen.findByText('Joined version')).toBeInTheDocument()
    expect(screen.queryByText('有 1 条新内容')).not.toBeInTheDocument()
  })

  it('keeps the reading position stable and prompts for changes while hidden or scrolled', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    let includeNewStory = false
    vi.stubGlobal('EventSource', FakeEventSource)
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes('/api/v1/stories') && !init?.method) {
        const stories = includeNewStory
          ? [realtimeStory('story-2', 'New article'), realtimeStory('story-1', 'Existing article')]
          : [realtimeStory('story-1', 'Existing article')]
        return new Response(JSON.stringify(realtimePage(stories)), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return defaultFetch(input, init)
    })

    render(<App />)
    expect(await screen.findByText('Existing article')).toBeInTheDocument()
    const eventSource = FakeEventSource.instances.at(-1)!
    eventSource.open()
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    includeNewStory = true
    eventSource.emit('library-change', JSON.stringify({ source_id: 'source-1' }))

    expect(await screen.findByText('有 1 条新内容')).toBeInTheDocument()
    expect(screen.queryByText('New article')).not.toBeInTheDocument()

    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    fireEvent.click(screen.getByRole('button', { name: '加载 1 条新内容' }))
    expect(await screen.findByText('New article')).toBeInTheDocument()
  })

  it('does not hot-replace an expanded Story when a new library change arrives', async () => {
    const defaultFetch = vi.mocked(fetch).getMockImplementation()!
    let includeNewStory = false
    vi.stubGlobal('EventSource', FakeEventSource)
    vi.mocked(fetch).mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.includes('/api/v1/stories') && !init?.method) {
        const stories = includeNewStory
          ? [realtimeStory('story-2', 'New article'), realtimeStory('story-1', 'Existing article')]
          : [realtimeStory('story-1', 'Existing article')]
        return new Response(JSON.stringify(realtimePage(stories)), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return defaultFetch(input, init)
    })

    render(<App />)
    fireEvent.click(await screen.findByText('Existing article'))
    expect(screen.getByText('Existing article body')).toBeInTheDocument()
    const eventSource = FakeEventSource.instances.at(-1)!
    eventSource.open()
    includeNewStory = true
    eventSource.emit('library-change', JSON.stringify({ source_id: 'source-1' }))

    expect(await screen.findByText('有 1 条新内容')).toBeInTheDocument()
    expect(screen.getByText('Existing article body')).toBeInTheDocument()
    expect(screen.queryByText('New article')).not.toBeInTheDocument()
  })
})
