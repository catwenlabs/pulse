import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/later')({
  component: () => <AppContent view="later" sourceID="" />,
})
