import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/starred')({
  component: () => <AppContent view="starred" sourceID="" />,
})
