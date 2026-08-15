import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/notes')({
  component: () => <AppContent view="notes" sourceID="" />,
})
