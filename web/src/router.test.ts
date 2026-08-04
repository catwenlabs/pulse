import { createMemoryHistory } from '@tanstack/react-router'
import { describe, expect, it } from 'vitest'

import { createAppRouter } from './router'

describe('file-based router', () => {
  it('matches Reader views and Source parameters from filesystem routes', async () => {
    const router = createAppRouter({
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })

    await router.load()
    await router.navigate({ to: '/sources/$sourceID', params: { sourceID: 'source-1' } })
    expect(router.state.location.pathname).toBe('/sources/source-1')

    await router.navigate({ to: '/starred' })
    expect(router.state.location.pathname).toBe('/starred')

    await router.navigate({ to: '/digests' })
    expect(router.state.location.pathname).toBe('/digests')

    await router.navigate({ to: '/stories/$storyID', params: { storyID: 'story-1' } })
    expect(router.state.location.pathname).toBe('/stories/story-1')
  })
})
