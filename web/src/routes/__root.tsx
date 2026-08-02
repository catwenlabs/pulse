import { createRootRoute, Outlet } from '@tanstack/react-router'

import { AppProvider } from '../App'

export const Route = createRootRoute({
  component: () => (
    <AppProvider>
      <Outlet />
    </AppProvider>
  ),
})
