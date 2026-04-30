'use client'

/**
 * VenueBillNetworkAdapter (PSY-365) — venue analog of
 * `SceneGraphVisualization`.
 *
 * Thin shape adapter that maps the backend `VenueBillNetworkResponse` onto
 * the shared `ForceGraphView`'s generic node/cluster/link shape. Keeps the
 * router dep + venue-specific aria-label phrasing out of the canvas
 * primitive, mirroring the pattern in
 * `features/scenes/components/SceneGraphVisualization.tsx`.
 *
 * Click semantics: PSY-361 inheritance — clicking a node navigates to that
 * artist's page (the recentering UX lives on the artist side, where the
 * URL `?center=` query param drives the breadcrumb).
 *
 * The "StyleAdapter" suffix in the export name is intentional: it signals
 * that this component does *no* layout / canvas work itself, only data
 * shaping, so future readers don't expect to find d3-force config here.
 */

import { useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { ForceGraphView } from '@/components/graph/ForceGraphView'
import type { GraphNode } from '@/components/graph/ForceGraphView'
import type { VenueBillNetworkResponse } from '../types'

interface SceneGraphVisualizationStyleAdapterProps {
  data: VenueBillNetworkResponse
  venueName: string
  containerWidth: number
  height?: number
}

export function SceneGraphVisualizationStyleAdapter({
  data,
  venueName,
  containerWidth,
  height,
}: SceneGraphVisualizationStyleAdapterProps) {
  const router = useRouter()

  const handleNodeClick = useCallback(
    (node: GraphNode) => {
      router.push(`/artists/${node.slug}`)
    },
    [router],
  )

  // a11y: surface artist + edge counts in the canvas description so screen
  // reader users get the same scale info that sighted users see in the
  // header. Window phrasing matches the filter labels.
  const windowPhrase =
    data.venue.window === 'last_12m'
      ? 'last 12 months'
      : data.venue.window === 'year' && data.venue.year
        ? `year ${data.venue.year}`
        : 'all time'
  const ariaLabel = `Co-bill network for ${venueName} (${windowPhrase}): ${data.venue.artist_count} artists, ${data.venue.edge_count} co-bills.`

  return (
    <ForceGraphView
      nodes={data.nodes}
      links={data.links}
      clusters={data.clusters}
      containerWidth={containerWidth}
      height={height}
      ariaLabel={ariaLabel}
      onNodeClick={handleNodeClick}
    />
  )
}
