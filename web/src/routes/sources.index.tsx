import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/sources/')({
  component: () => <AppContent view="sources" sourceID="" />,
})
