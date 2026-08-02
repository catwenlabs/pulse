import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/inbox')({
  component: () => <AppContent view="inbox" sourceID="" />,
})
