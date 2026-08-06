import { useEffect, useRef, useState } from 'react'
import type { QueryClient } from '@tanstack/react-query'

import { libraryEventsPath, type LibraryChangeEvent } from './api'

export type RealtimeConnectionState = 'connecting' | 'connected' | 'degraded'

export type LibraryRealtimeSignal = {
  id: number
  kind: 'change' | 'reconnected' | 'cross-tab'
  sourceId?: string
}

type BroadcastPayload = {
  type: 'library-change'
  source_id?: string
}

const channelName = 'pulse-library'
const disconnectedNoticeDelay = 30_000

export function useLibraryRealtime(queryClient: QueryClient) {
  const [signal, setSignal] = useState<LibraryRealtimeSignal | null>(null)
  const [connectionState, setConnectionState] = useState<RealtimeConnectionState>('connecting')
  const signalID = useRef(0)

  useEffect(() => {
    let active = true
    let eventSource: EventSource | null = null
    let channel: BroadcastChannel | null = null
    let degradedTimer: number | undefined
    let disconnected = false

    const emit = (kind: LibraryRealtimeSignal['kind'], sourceId?: string) => {
      if (!active) return
      signalID.current += 1
      setSignal({ id: signalID.current, kind, sourceId })
    }

    const refreshLibraryChrome = () => {
      void queryClient.invalidateQueries({ queryKey: ['sources'] })
      void queryClient.invalidateQueries({ queryKey: ['folders'] })
    }

    const refreshChatChrome = () => {
      void queryClient.invalidateQueries({ queryKey: ['chat-tools'] })
      void queryClient.invalidateQueries({ queryKey: ['chat-conversations'] })
    }

    const reconcileAfterReconnect = () => {
      refreshLibraryChrome()
      void queryClient.refetchQueries({ queryKey: ['sources'], type: 'active' })
      void queryClient.refetchQueries({ queryKey: ['folders'], type: 'active' })
      emit('reconnected')
    }

    const broadcast = (sourceId?: string) => {
      if (!channel) return
      const message: BroadcastPayload = { type: 'library-change', source_id: sourceId }
      channel.postMessage(message)
    }

    const handleLibraryChange = (event: MessageEvent<string>) => {
      let payload: LibraryChangeEvent = {}
      try {
        payload = JSON.parse(event.data) as LibraryChangeEvent
      } catch {
        // A malformed invalidation is harmless; the next reconnect/visibility
        // reconciliation still restores the server state.
      }
      if (typeof payload.source_id === 'string' && payload.source_id.startsWith('ai-chat')) {
        refreshChatChrome()
        emit('change', payload.source_id)
        return
      }
      refreshLibraryChrome()
      broadcast(payload.source_id)
      emit('change', payload.source_id)
    }

    const handleVisibilityOrNetworkRestore = () => {
      if (document.visibilityState === 'hidden') return
      refreshLibraryChrome()
      emit('reconnected')
    }

    if (typeof BroadcastChannel !== 'undefined') {
      channel = new BroadcastChannel(channelName)
      channel.onmessage = (event: MessageEvent<BroadcastPayload>) => {
        if (event.data?.type !== 'library-change') return
        refreshLibraryChrome()
        emit('cross-tab', event.data.source_id)
      }
    }

    if (typeof EventSource !== 'undefined') {
      eventSource = new EventSource(libraryEventsPath)
      eventSource.addEventListener('library-change', handleLibraryChange)
      eventSource.onopen = () => {
        const wasDisconnected = disconnected
        disconnected = false
        if (degradedTimer !== undefined) window.clearTimeout(degradedTimer)
        degradedTimer = undefined
        if (!active) return
        setConnectionState('connected')
        if (wasDisconnected) reconcileAfterReconnect()
      }
      eventSource.onerror = () => {
        if (!active) return
        disconnected = true
        setConnectionState('connecting')
        if (degradedTimer !== undefined) return
        degradedTimer = window.setTimeout(() => {
          if (active && disconnected) setConnectionState('degraded')
        }, disconnectedNoticeDelay)
      }
    } else {
      setConnectionState('degraded')
    }

    document.addEventListener('visibilitychange', handleVisibilityOrNetworkRestore)
    window.addEventListener('online', handleVisibilityOrNetworkRestore)

    return () => {
      active = false
      if (degradedTimer !== undefined) window.clearTimeout(degradedTimer)
      eventSource?.close()
      channel?.close()
      document.removeEventListener('visibilitychange', handleVisibilityOrNetworkRestore)
      window.removeEventListener('online', handleVisibilityOrNetworkRestore)
    }
  }, [queryClient])

  return { connectionState, signal }
}
