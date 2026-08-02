import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  createManualEntry,
  createSource,
  addStoryTag,
  deleteEntry,
  importAnnotations,
  listSourceEntries,
  listStories,
  listSources,
  getStory,
  mergeStory,
  previewSource,
  reclusterStories,
  runSource,
  removeStoryTag,
  setStoryRepresentative,
  setSourceEnabled,
  splitStory,
  updateSource,
} from './api'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('source API', () => {
  it('uses the expected REST endpoints', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('[]', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"id":"source-1"}', { status: 201 }))
      .mockResolvedValueOnce(new Response('{"id":"run-1","status":"pending"}', { status: 202 }))
      .mockResolvedValueOnce(new Response('{"id":"source-1","enabled":false}', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"id":"source-1","name":"Renamed"}', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"candidates":[],"diagnostics":{"status":"ok"}}', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"entries":[],"total_entries":0,"reader_counts":{"inbox_stories":0,"unread_stories":0,"starred_stories":0,"later_stories":0,"hidden_stories":0}}', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"stories":[],"total_stories":0,"reader_counts":{"inbox_stories":0,"unread_stories":0,"starred_stories":0,"later_stories":0,"hidden_stories":0}}', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"id":"story-1","representative":{"id":"entry-1"}}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await listSources()
    await createSource({ name: 'Feed', kind: 'rss', locator: 'https://example.com/feed' })
    await runSource('source-1')
    await setSourceEnabled('source-1', false)
    await updateSource('source-1', { name: 'Renamed', locator: 'https://example.com/new' })
    await previewSource({ name: 'Feed', kind: 'rss', locator: 'https://example.com/feed' })
    await listSourceEntries('source-1', { q: 'go', state: 'unread', cursor: 'source-cursor' })
    await listStories({ q: 'go', state: 'unread', cursor: 'story-cursor' })
    await getStory('story-1')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/sources', undefined)
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/sources', expect.objectContaining({
      method: 'POST',
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/sources/source-1/runs', {
      method: 'POST',
    })
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/v1/sources/source-1', expect.objectContaining({
      method: 'PATCH',
      body: '{"enabled":false}',
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/v1/sources/source-1', expect.objectContaining({
      method: 'PATCH',
      body: '{"name":"Renamed","locator":"https://example.com/new"}',
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(6, '/api/v1/sources/preview', expect.objectContaining({
      method: 'POST',
    }))
    expect(String(fetchMock.mock.calls[6][0])).toContain('q=go')
    expect(String(fetchMock.mock.calls[6][0])).toContain('/api/v1/sources/source-1/entries')
    expect(String(fetchMock.mock.calls[6][0])).toContain('cursor=source-cursor')
    expect(String(fetchMock.mock.calls[7][0])).toContain('/api/v1/stories?')
    expect(String(fetchMock.mock.calls[7][0])).toContain('cursor=story-cursor')
    expect(fetchMock).toHaveBeenNthCalledWith(9, '/api/v1/stories/story-1', undefined)
  })

  it('enqueues a manually saved web page with an idempotency key', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      '{"id":"acquisition-1","status":"pending"}',
      { status: 202 },
    ))
    vi.stubGlobal('fetch', fetchMock)

    await createManualEntry('manual-source', {
      url: 'https://example.com/article',
      title: 'Saved article',
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/sources/manual-source/entries', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': expect.not.stringContaining('example.com'),
      },
      body: JSON.stringify({
        url: 'https://example.com/article',
        title: 'Saved article',
      }),
    })
    expect(String(fetchMock.mock.calls[0][1]?.headers && (
      fetchMock.mock.calls[0][1]!.headers as Record<string, string>
    )['Idempotency-Key']).length).toBeLessThanOrEqual(64)
  })

  it('enqueues a batch of book annotations', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      '{"id":"acquisition-1","status":"pending"}',
      { status: 202 },
    ))
    vi.stubGlobal('fetch', fetchMock)
    const annotations = [{
      provider: 'apple-books',
      book_identity: 'book-123',
      book_title: '思考，快与慢',
      book_author: 'Daniel Kahneman',
      chapter: '第三章',
      location: '1284',
      highlight_color: 'yellow',
      highlight: '系统一自动而快速地运行。',
      note: '这里对应直觉判断。',
    }]

    await importAnnotations('annotation-source', annotations)

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/sources/annotation-source/annotations', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': expect.any(String),
      },
      body: JSON.stringify({ annotations }),
    })
  })

  it('merges one story into another', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      '{"id":"story-2","entry_count":2}',
      { status: 200 },
    ))
    vi.stubGlobal('fetch', fetchMock)

    await mergeStory('story-1', 'story-2')

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/stories/story-1/merge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ into: 'story-2' }),
    })
  })

  it('splits an entry out of a story', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      '{"id":"story-3","entry_count":1}',
      { status: 200 },
    ))
    vi.stubGlobal('fetch', fetchMock)

    await splitStory('story-1', 'entry-2')

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/stories/story-1/split', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ entry_id: 'entry-2' }),
    })
  })

  it('targets representative changes and permanent Entry deletion explicitly', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('{"id":"story-1"}', { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    await setStoryRepresentative('story-1', 'entry-2')
    await deleteEntry('entry-2')
    await deleteEntry('entry-2', true)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/stories/story-1/representative', expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ entry_id: 'entry-2' }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/entries/entry-2', { method: 'DELETE' })
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/entries/entry-2?confirm=true', { method: 'DELETE' })
  })

  it('keeps tag mutations on the Story endpoints', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('{"id":"tag-1","name":"tech"}', { status: 201 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    await addStoryTag('story-1', 'tech')
    await removeStoryTag('story-1', 'tag-1')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/stories/story-1/tags', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ name: 'tech' }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/stories/story-1/tags/tag-1', { method: 'DELETE' })
  })

  it('reclusters story aggregation on demand', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(
      '{"processed":7}',
      { status: 200 },
    ))
    vi.stubGlobal('fetch', fetchMock)

    const result = await reclusterStories()

    expect(result.processed).toBe(7)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/stories/recluster', {
      method: 'POST',
    })
  })
})
