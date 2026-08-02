import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/annotations')({
  component: () => <AppContent view="annotations" sourceID="" />,
})
