import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/documents/')({
  component: () => <AppContent view="documents" sourceID="" />,
})
