import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

function SourceReaderRoute() {
  const { sourceID } = Route.useParams()
  return <AppContent view="inbox" sourceID={sourceID} />
}

export const Route = createFileRoute('/sources/$sourceID')({
  component: SourceReaderRoute,
})
