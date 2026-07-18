'use client'

/**
 * CollectionGraphVisualization (PSY-1473) — thin shape adapter over
 * ForceGraphView for the collection knowledge graph.
 *
 * Owns the collection surface's node-select → context-panel wiring (the last
 * PSY-1451 surface): click SELECTS into ArtistContextPanel (artist nodes) or
 * EntityContextPanel (venue / label / release / show / festival); navigation
 * only via "Open page →". Selection conventions live in
 * `useArtistPanelSelection`. Esc layering with the fullscreen overlay is
 * handled by GraphPanelShell's Radix DismissableLayer (PSY-1355).
 *
 * Why a wrapper instead of inlining at CollectionGraph: same reasons as
 * SceneGraphVisualization — selection + panel mount inside whichever
 * container renders the canvas (inline section or fullscreen overlay), with
 * instance-local selection that resets on fullscreen toggle.
 */

import { useMemo } from 'react'
import { ForceGraphView } from '@/components/graph/ForceGraphView'
import type { GraphCluster, GraphNode } from '@/components/graph/ForceGraphView'
import { ArtistContextPanel } from '@/components/graph/ArtistContextPanel'
import {
  EntityContextPanel,
  graphEntitySelectGestureHint,
  isEntityPanelType,
} from '@/components/graph/EntityContextPanel'
import { GraphPanelHost } from '@/components/graph/GraphPanelHost'
import { useArtistPanelSelection } from '@/components/graph/useArtistPanelSelection'
import { useCollectionEntityPanelModel } from '../hooks/useCollectionEntityPanelModel'
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
import type {
  CollectionGraphLink,
  CollectionGraphNode,
} from '../types'

interface CollectionGraphVisualizationProps {
  nodes: GraphNode[]
  /** Raw collection nodes (carry entity_type) — parallel to `nodes` by id. */
  sourceNodes: CollectionGraphNode[]
  links: CollectionGraphLink[]
  clusters: GraphCluster[]
  containerWidth: number
  height?: number
  collectionTitle: string
  /**
   * The item-count phrase for the canvas aria-label — "N items" or, when the
   * graph is capped, "top N of M items" (PSY-1476). Built by the parent so the
   * aria-label and the visible header can't state different numbers.
   */
  countPhrase: string
  edgeCount: number
}

export function CollectionGraphVisualization({
  nodes,
  sourceNodes,
  links,
  clusters,
  containerWidth,
  height,
  collectionTitle,
  countPhrase,
  edgeCount,
}: CollectionGraphVisualizationProps) {
  const sourceById = useMemo(
    () => new Map(sourceNodes.map(n => [n.id, n])),
    [sourceNodes],
  )

  const {
    selectedNode,
    canvasWrapRef,
    panelRef,
    handleNodeClick,
    handleBackgroundClick,
    handlePanelClose,
  } = useArtistPanelSelection({
    resolveNode: selected => nodes.find(n => n.id === selected.id) ?? null,
  })

  const selectedSource = selectedNode
    ? (sourceById.get(selectedNode.id) ?? null)
    : null
  const entityType = selectedSource?.entity_type ?? 'artist'
  const isArtist = entityType === 'artist'

  // Collection node IDs are collection_item IDs — fetch the artist card by
  // slug so we don't hit the wrong artist (or 404).
  const cardQuery = useArtistGraphCard({
    artistId: isArtist ? (selectedSource?.slug ?? null) : null,
    enabled: isArtist && selectedSource !== null,
  })

  const entityModel = useCollectionEntityPanelModel({
    selected:
      selectedSource && isEntityPanelType(entityType) ? selectedSource : null,
    nodes: sourceNodes,
    links,
  })

  const ariaLabel = `Knowledge graph for collection ${collectionTitle}: ${countPhrase}, ${edgeCount} connections. ${graphEntitySelectGestureHint}`

  const panel =
    selectedNode && selectedSource && isArtist ? (
      <ArtistContextPanel
        className="absolute top-2 left-2 z-40"
        artistName={selectedSource.name}
        artistSlug={selectedSource.slug}
        card={cardQuery.data}
        isError={cardQuery.isError}
        onClose={handlePanelClose}
        panelRef={panelRef}
      />
    ) : selectedNode && entityModel ? (
      <EntityContextPanel
        className="absolute top-2 left-2 z-40"
        entityType={entityModel.entityType}
        name={entityModel.name}
        slug={entityModel.slug}
        meta={entityModel.meta}
        primary={entityModel.primary}
        facts={entityModel.facts}
        isLoading={entityModel.isLoading}
        isError={entityModel.isError}
        onClose={handlePanelClose}
        panelRef={panelRef}
      />
    ) : null

  return (
    <GraphPanelHost canvasWrapRef={canvasWrapRef} panel={panel}>
      <ForceGraphView
        nodes={nodes}
        links={links}
        clusters={clusters}
        containerWidth={containerWidth}
        height={height}
        ariaLabel={ariaLabel}
        onNodeClick={handleNodeClick}
        onBackgroundClick={handleBackgroundClick}
        focusNodeId={selectedNode?.id ?? null}
        showAccessibleNodeControls
        showEdgeLegend
      />
    </GraphPanelHost>
  )
}
