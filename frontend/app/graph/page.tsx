import { Suspense } from 'react'
import type { Metadata } from 'next'

import { GraphObservatory, GraphObservatorySkeleton } from '@/features/graph'

/**
 * The canonical URL is deliberately the bare path.
 *
 * `?artist=<slug>` re-roots the same tool on one artist rather than naming a
 * different page, and the artist's own page is what should rank for that name.
 */
export const metadata: Metadata = {
  title: 'Music Knowledge Graph',
  description: 'Search artists, inspect their connections, and follow a trail through the Psychic Homily knowledge graph.',
  alternates: { canonical: 'https://psychichomily.com/graph' },
  openGraph: {
    title: 'Music Knowledge Graph | Psychic Homily',
    description: 'Search artists, inspect their connections, and follow a trail through the Psychic Homily knowledge graph.',
    url: '/graph',
    type: 'website',
  },
}

export default function GraphPage() {
  // Suspense, not decoration: the Observatory reads `?artist=` through nuqs,
  // which reads `useSearchParams`, and under `cacheComponents` a client
  // component that does so has to sit behind a boundary. The fallback is what
  // the route's static shell paints.
  return (
    <Suspense fallback={<GraphObservatorySkeleton />}>
      <GraphObservatory />
    </Suspense>
  )
}
