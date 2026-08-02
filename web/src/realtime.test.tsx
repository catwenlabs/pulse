import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'

import { useLibraryRealtime } from './realtime'

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

  fail() {
    this.onerror?.()
  }

  emit(type: string, data: string) {
    const event = new MessageEvent<string>(type, { data })
    for (const listener of this.listeners.get(type) ?? []) listener(event)
  }
}

class FakeBroadcastChannel {
  static instances: FakeBroadcastChannel[] = []
  onmessage: ((event: MessageEvent<{ type: string; source_id?: string }>) => void) | null = null
  readonly postMessage = vi.fn()
  readonly close = vi.fn()

  constructor(readonly name: string) {
    FakeBroadcastChannel.instances.push(this)
  }

  receive(data: { type: string; source_id?: string }) {
    this.onmessage?.(new MessageEvent('message', { data }))
  }
}

function Probe({ client }: { client: QueryClient }) {
  const { connectionState, signal } = useLibraryRealtime(client)
  return (
    <output>
      {connectionState}|{signal?.kind ?? ''}|{signal?.sourceId ?? ''}
    </output>
  )
}

describe('useLibraryRealtime', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    FakeEventSource.instances = []
    FakeBroadcastChannel.instances = []
  })

  it('invalidates HTTP caches for SSE and cross-tab signals without copying data', async () => {
    vi.stubGlobal('EventSource', FakeEventSource)
    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel)
    const client = new QueryClient()
    const invalidateQueries = vi.spyOn(client, 'invalidateQueries')

    render(<Probe client={client} />)
    const eventSource = FakeEventSource.instances[0]
    const channel = FakeBroadcastChannel.instances[0]
    act(() => eventSource.open())
    expect(screen.getByText('connected||')).toBeInTheDocument()

    act(() => eventSource.emit('library-change', JSON.stringify({ source_id: 'source-1' })))
    expect(screen.getByText('connected|change|source-1')).toBeInTheDocument()
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['sources'] })
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['folders'] })
    expect(channel.postMessage).toHaveBeenCalledWith({ type: 'library-change', source_id: 'source-1' })

    act(() => channel.receive({ type: 'library-change', source_id: 'source-2' }))
    expect(screen.getByText('connected|cross-tab|source-2')).toBeInTheDocument()
    expect(channel.postMessage).toHaveBeenCalledTimes(1)
  })

  it('shows degraded status after 30 seconds and reconciles on EventSource reconnect', () => {
    vi.useFakeTimers()
    vi.stubGlobal('EventSource', FakeEventSource)
    vi.stubGlobal('BroadcastChannel', FakeBroadcastChannel)
    const client = new QueryClient()
    const refetchQueries = vi.spyOn(client, 'refetchQueries').mockResolvedValue()

    render(<Probe client={client} />)
    const eventSource = FakeEventSource.instances[0]
    act(() => {
      eventSource.open()
      eventSource.fail()
      vi.advanceTimersByTime(30_000)
    })
    expect(screen.getByText('degraded||')).toBeInTheDocument()

    act(() => eventSource.open())
    expect(screen.getByText('connected|reconnected|')).toBeInTheDocument()
    expect(refetchQueries).toHaveBeenCalledWith({ queryKey: ['sources'], type: 'active' })
    expect(refetchQueries).toHaveBeenCalledWith({ queryKey: ['folders'], type: 'active' })
  })
})
