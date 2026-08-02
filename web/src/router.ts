import { createRouter } from '@tanstack/react-router'

import { routeTree } from './routeTree.gen'

export function createAppRouter(options?: Parameters<typeof createRouter>[0]) {
  return createRouter({
    ...options,
    routeTree,
    defaultPreload: 'intent',
    scrollRestoration: true,
  })
}

export const router = createAppRouter()

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
