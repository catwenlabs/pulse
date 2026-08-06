import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/ai-conversations')({
  component: () => <AppContent view="ai-conversations" sourceID="" />,
})
