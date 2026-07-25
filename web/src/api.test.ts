import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  createSource,
  listEntries,
  listSources,
  previewSource,
  runSource,
  setSourceEnabled,
  updateEntry,
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
      .mockResolvedValueOnce(new Response('{"candidates":[],"diagnostics":{"status":"ok"}}', { status: 200 }))
      .mockResolvedValueOnce(new Response('[]', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"id":"entry-1"}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await listSources()
    await createSource({ name: 'Feed', kind: 'rss', locator: 'https://example.com/feed' })
    await runSource('source-1')
    await setSourceEnabled('source-1', false)
    await previewSource({ name: 'Feed', kind: 'rss', locator: 'https://example.com/feed' })
    await listEntries({ q: 'go', state: 'unread', sourceId: 'source-1' })
    await updateEntry('entry-1', { read: true })

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
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/v1/sources/preview', expect.objectContaining({
      method: 'POST',
    }))
    expect(String(fetchMock.mock.calls[5][0])).toContain('q=go')
    expect(String(fetchMock.mock.calls[5][0])).toContain('source_id=source-1')
    expect(fetchMock).toHaveBeenNthCalledWith(7, '/api/v1/entries/entry-1', expect.objectContaining({
      method: 'PATCH',
    }))
  })
})
