import type { Metadata } from 'next'
import { notFound } from 'next/navigation'

import {
  GraphWeekContent,
  buildGraphWeekMetadata,
  getGraphWeek,
} from '@/features/graph/graphWeekPage'

export async function generateMetadata(): Promise<Metadata> {
  return buildGraphWeekMetadata()
}

export default async function GraphThisWeekPage() {
  // `notFound()` must be called HERE, in the page component. Calling it from a
  // helper module rendered the not-found body but left the response at HTTP 200,
  // which unfurlers and search engines read as a valid page (PSY-906).
  //
  // A 404 is the right posture for a snapshot that does not exist or cannot be
  // dated. This page has exactly one fact to report; without it there is nothing
  // here, and the map's own share affordance is gated on the same resolution, so
  // the site never links to this state.
  const view = await getGraphWeek()
  if (!view) notFound()
  return <GraphWeekContent view={view} />
}
