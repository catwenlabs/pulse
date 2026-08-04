import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/digests')({
  component: () => <AppContent view="ai" sourceID="" />,
})
