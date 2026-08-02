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
  story: (id: string) => ['story', id] as const,
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
