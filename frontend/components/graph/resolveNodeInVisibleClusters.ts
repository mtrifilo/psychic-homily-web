/**
 * The standard `resolveNode` body for useArtistPanelSelection on surfaces
 * whose payload carries a cluster legend: the selected node must still be in
 * the payload AND its cluster must not be hidden. The filter mirrors
 * ForceGraphView's own node cull (empty cluster_id falls back to
 * OTHER_CLUSTER_ID) — single-sourced here so the cull-mirror rule can't
 * drift between the scene and station adapters.
 *
 * Deliberately NOT in useArtistPanelSelection.ts: OTHER_CLUSTER_ID is a
 * VALUE import from ForceGraphView, and the hook must stay importable by
 * statically-mounted surfaces (HomeSceneGraph) without dragging the canvas
 * module into their initial JS (PSY-868). Only callers that already import
 * ForceGraphView statically should import this helper.
 */

import { OTHER_CLUSTER_ID } from './ForceGraphView'
import type { GraphNode } from './ForceGraphView'

export function resolveNodeInVisibleClusters<TNode extends GraphNode>(
  selected: GraphNode,
  nodes: readonly TNode[],
  hiddenClusterIDs: ReadonlySet<string>,
): TNode | null {
  return (
    nodes.find(
      node =>
        node.id === selected.id &&
        !hiddenClusterIDs.has(node.cluster_id || OTHER_CLUSTER_ID),
    ) ?? null
  )
}
