/**
 * Derive in-graph neighbor lists for collection-graph context panels
 * (PSY-1473). "N in this graph" counts and bill/roster name lists come from
 * the already-rendered payload — no fetch.
 */

import type {
  CollectionGraphLink,
  CollectionGraphNode,
} from '../types'

/** Edge types that connect a venue to artists in the collection graph. */
export const VENUE_ARTIST_EDGE_TYPES = ['played_at'] as const
/** Edge types that connect a label to artists. */
export const LABEL_ARTIST_EDGE_TYPES = ['signed_to'] as const
/** Edge types that connect a show to artists (the bill). */
export const SHOW_ARTIST_EDGE_TYPES = ['show_lineup'] as const
/** Edge types that connect a festival to artists. */
export const FESTIVAL_ARTIST_EDGE_TYPES = ['lineup'] as const
/** Edge types that connect a release to artists. */
export const RELEASE_ARTIST_EDGE_TYPES = ['discography'] as const

export function indexCollectionNodes(
  nodes: CollectionGraphNode[],
): Map<number, CollectionGraphNode> {
  const map = new Map<number, CollectionGraphNode>()
  for (const node of nodes) {
    map.set(node.id, node)
  }
  return map
}

/**
 * Return neighbor nodes of `neighborEntityType` linked to `nodeId` via any of
 * `edgeTypes` (either direction). Order follows first-seen link order.
 */
export function collectionGraphNeighbors(
  nodeId: number,
  links: CollectionGraphLink[],
  nodesById: Map<number, CollectionGraphNode>,
  edgeTypes: readonly string[],
  neighborEntityType: string,
): CollectionGraphNode[] {
  const edgeSet = new Set(edgeTypes)
  const seen = new Set<number>()
  const out: CollectionGraphNode[] = []

  for (const link of links) {
    if (!edgeSet.has(link.type)) continue
    const otherId =
      link.source_id === nodeId
        ? link.target_id
        : link.target_id === nodeId
          ? link.source_id
          : null
    if (otherId == null || seen.has(otherId)) continue
    const other = nodesById.get(otherId)
    if (!other || (other.entity_type ?? 'artist') !== neighborEntityType) {
      continue
    }
    seen.add(otherId)
    out.push(other)
  }

  return out
}

export function formatArtistNameList(
  artists: CollectionGraphNode[],
  opts?: { max?: number; joiner?: string },
): string {
  const max = opts?.max ?? 5
  const joiner = opts?.joiner ?? ' · '
  const names = artists.slice(0, max).map(a => a.name)
  if (artists.length > max) {
    return `${names.join(joiner)} +${artists.length - max}`
  }
  return names.join(joiner)
}
