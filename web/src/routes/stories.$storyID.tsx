import { createFileRoute } from '@tanstack/react-router'

import { AppContent } from '../App'

function StoryDetailRoute() {
  const { storyID } = Route.useParams()
  return <AppContent view="story" sourceID="" storyID={storyID} />
}

export const Route = createFileRoute('/stories/$storyID')({
  component: StoryDetailRoute,
})
