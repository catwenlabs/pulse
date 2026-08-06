import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  APIError,
  consumeChatStream,
  createConversation,
  createSelectionTool,
  deleteConversation,
  deleteSelectionTool,
  getConversation,
  listConversations,
  listSelectionTools,
  reorderSelectionTools,
  sendFollowUp,
  stopGeneration,
  streamAssistant,
  updateSelectionTool,
} from './api'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('selection tool API', () => {
  it('targets the documented endpoints with payloads', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('[]', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"id":"t2"}', { status: 201 }))
      .mockResolvedValueOnce(new Response('{"id":"t2","name":"x"}', { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response('[]', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await listSelectionTools()
    await createSelectionTool({ name: 'AI 翻译', prompt_template: '{{selection}}', enabled: true })
    await updateSelectionTool('t2', { name: 'AI 翻译', prompt_template: '{{selection}}', enabled: true })
    await deleteSelectionTool('t2')
    await reorderSelectionTools(['t1', 't2'])

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/ai/tools', undefined)
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/ai/tools', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ name: 'AI 翻译', prompt_template: '{{selection}}', enabled: true }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/ai/tools/t2', expect.objectContaining({ method: 'PUT' }))
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/v1/ai/tools/t2', { method: 'DELETE' })
    expect(fetchMock).toHaveBeenNthCalledWith(5, '/api/v1/ai/tools/order', expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ tool_ids: ['t1', 't2'] }),
    }))
  })
})

describe('conversation API', () => {
  it('creates conversations and follow-ups with idempotency headers', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('{"conversation":{"id":"c1"},"user_message":{"id":"m1"}}', { status: 201 }))
      .mockResolvedValueOnce(new Response('{"id":"m2"}', { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)

    await createConversation({ tool_id: 't1', selection: 'E=mc^2' }, 'client-key')
    await sendFollowUp('c1', '再说详细点', 'fu-key')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/ai/conversations', expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'Idempotency-Key': 'client-key' }),
      body: JSON.stringify({ tool_id: 't1', selection: 'E=mc^2' }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/ai/conversations/c1/messages', expect.objectContaining({
      headers: expect.objectContaining({ 'Idempotency-Key': 'fu-key' }),
    }))
  })

  it('lists, fetches, deletes, and stops conversations', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('{"items":[],"has_more":false}', { status: 200 }))
      .mockResolvedValueOnce(new Response('{"conversation":{"id":"c1"},"messages":[]}', { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    await listConversations(25, 'cursor-abc')
    await getConversation('c1')
    await deleteConversation('c1')
    await stopGeneration('c1')

    expect(fetchMock).toHaveBeenNthCalledWith(1, expect.stringContaining('/api/v1/ai/conversations?limit=25&cursor=cursor-abc'), undefined)
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/ai/conversations/c1', undefined)
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/ai/conversations/c1', { method: 'DELETE' })
    expect(fetchMock).toHaveBeenNthCalledWith(4, '/api/v1/ai/conversations/c1/stop', { method: 'POST' })
  })
})

describe('chat stream parser', () => {
  function streamOf(chunks: string[]): ReadableStream<Uint8Array> {
    const encoder = new TextEncoder()
    return new ReadableStream({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
        controller.close()
      },
    })
  }

  it('emits metadata, deltas, and a terminal event', async () => {
    const body = streamOf([
      'event: metadata\ndata: {"kind":"metadata","conversation_id":"c1","message_id":"m2"}\n\n',
      'event: delta\ndata: {"kind":"delta","delta":"Hello "}\n\n',
      'event: delta\ndata: {"kind":"delta","delta":"world"}\n\n',
      'event: completed\ndata: {"kind":"completed","content":"Hello world","status":"completed","prompt_tokens":7}\n\n',
    ])
    const events: string[] = []
    const terminal = await consumeChatStream(body, (event) => {
      events.push(event.kind)
    })
    expect(events).toEqual(['metadata', 'delta', 'delta', 'completed'])
    expect(terminal.kind).toBe('completed')
    expect(terminal.prompt_tokens).toBe(7)
  })

  it('surfaces cancelled terminal events', async () => {
    const body = streamOf([
      'event: metadata\ndata: {"kind":"metadata"}\n\n',
      'event: cancelled\ndata: {"kind":"cancelled","content":"partial","status":"cancelled"}\n\n',
    ])
    const terminal = await consumeChatStream(body, () => {})
    expect(terminal.kind).toBe('cancelled')
  })

  it('throws when the stream ends without a terminal event', async () => {
    const body = streamOf(['event: metadata\ndata: {"kind":"metadata"}\n\n'])
    await expect(consumeChatStream(body, () => {})).rejects.toBeInstanceOf(APIError)
  })

  it('streamAssistant parses a real response body and forwards the idempotency key', async () => {
    const sse =
      'event: metadata\ndata: {"kind":"metadata","conversation_id":"c1","message_id":"m2"}\n\n' +
      'event: completed\ndata: {"kind":"completed","content":"ok","status":"completed"}\n\n'
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(sse, { status: 200 })))
    const seen: string[] = []
    const terminal = await streamAssistant('c1', 'generate', 'gen-key', (e) => seen.push(e.kind))
    expect(terminal.kind).toBe('completed')
    expect(seen).toEqual(['metadata', 'completed'])
    expect(vi.mocked(fetch)).toHaveBeenCalledWith(
      '/api/v1/ai/conversations/c1/generate',
      expect.objectContaining({ headers: expect.objectContaining({ 'Idempotency-Key': 'gen-key' }) }),
    )
  })
})
