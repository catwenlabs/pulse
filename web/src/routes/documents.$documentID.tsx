import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

function DocumentReaderRoute() {
  const { documentID } = Route.useParams()
  return <AppContent view="document-reader" sourceID="" documentID={documentID} />
}

export const Route = createFileRoute('/documents/$documentID')({
  component: DocumentReaderRoute,
})
