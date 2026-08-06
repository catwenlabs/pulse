import { QueryClient } from '@tanstack/react-query'

import type { EntryQuery } from './api'

const queryCacheTime = import.meta.env.MODE === 'test' ? Infinity : 5 * 60 * 1000

export function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
        refetchOnReconnect: false,
        staleTime: 30_000,
        gcTime: queryCacheTime,
      },
    },
  })
}

export const queryKeys = {
  sources: ['sources'] as const,
  folders: ['folders'] as const,
  storyRoot: ['story'] as const,
  story: (id: string) => ['story', id] as const,
  digests: ['digests'] as const,
  digest: (id: string) => ['digest', id] as const,
  digestPreview: (scope: { startAt: string; endAt: string; maxStories: string }) => ['digest-preview', scope] as const,
  chatTools: ['chat-tools'] as const,
  chatConversations: ['chat-conversations'] as const,
  chatConversation: (id: string) => ['chat-conversation', id] as const,
  readerRoot: ['reader'] as const,
  reader: (query: EntryQuery & { view: string; state: string; limit: number }) => [
    'reader',
    {
      sourceId: query.sourceId || '',
      view: query.view,
      q: query.q || '',
      state: query.state,
      tag: query.tag || '',
      limit: query.limit,
      cursor: query.cursor || '',
    },
  ] as const,
}
