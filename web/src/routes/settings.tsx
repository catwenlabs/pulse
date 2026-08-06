import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/settings')({
  component: () => <AppContent view="settings" sourceID="" />,
})
