import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

export const Route = createFileRoute('/')({
  component: () => <AppContent view="inbox" sourceID="" />,
})
