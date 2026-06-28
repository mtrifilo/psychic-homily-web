/**
 * Merge ego graphs for expand-on-demand (PSY-1259)
 *
 * The artist graph dialog opens on one artist's 1-hop ego graph. "Expand-on-demand"
 * (van Ham & Perer, IEEE InfoVis 2009; see docs/open-questions/graph-density-discovery-
 * redesign.md §3.4) grows it: clicking a satellite fetches THAT artist's ego graph and
 * merges it into the current view, so the user walks the knowledge graph outward
 * (Artist → more Artists) without losing context the way re-center does.
 *
 * This is the pure data model behind that: given the base center's ego graph plus a set
 * of fetched expansion payloads (keyed by the expanded node's id), produce one merged
 * graph with each node's shortest-path hop distance from the center. Hop drives the
 * concentric-ring radial layout (rings = depth); collapse is "drop an expansion" — the
 * BFS reachability prune below removes any node that was only reachable through it, so
 * the caller never has to track a dependency tree.
 *
 * Pure + UI-free, so it's unit-tested without the canvas like the other graph helpers.
 */

import type { ArtistGraph, ArtistGraphNode, ArtistGraphLink } from '../types'

export interface MergedEgoGraph {
  /** The base center (hop 0); unchanged by expansions. */
  center: ArtistGraphNode
  /** Non-center nodes reachable from the center, after the collapse-orphan prune. */
  nodes: ArtistGraphNode[]
  /** Links whose BOTH endpoints survived the prune. */
  links: ArtistGraphLink[]
  /** Shortest-path hop distance from the center: center = 0, its neighbors = 1, … */
  hopByNodeId: Map<number, number>
}

export function mergeEgoGraphs(
  base: ArtistGraph,
  expansions: ReadonlyMap<number, ArtistGraph>,
): MergedEgoGraph {
  const centerId = base.center.id

  // 1. Union nodes (dedup by id): base center + base neighbors + each expansion's center
  //    and neighbors. First write wins — payloads for the same artist id are equivalent.
  const nodeById = new Map<number, ArtistGraphNode>()
  const addNode = (n: ArtistGraphNode) => {
    if (!nodeById.has(n.id)) nodeById.set(n.id, n)
  }
  addNode(base.center)
  for (const n of base.nodes) addNode(n)
  for (const ego of expansions.values()) {
    addNode(ego.center)
    for (const n of ego.nodes) addNode(n)
  }

  // 2. Union links (dedup by canonical key). The DB stores relationships with
  //    source_artist_id < target_artist_id, so the same physical edge carries identical
  //    source_id/target_id/type across payloads and collides on this key.
  const linkByKey = new Map<string, ArtistGraphLink>()
  const addLink = (l: ArtistGraphLink) => {
    const key = `${l.source_id}|${l.target_id}|${l.type}`
    if (!linkByKey.has(key)) linkByKey.set(key, l)
  }
  for (const l of base.links) addLink(l)
  for (const ego of expansions.values()) for (const l of ego.links) addLink(l)

  // 3. BFS from the center over the undirected link adjacency → shortest-path hop.
  const adjacency = new Map<number, number[]>()
  const pushAdj = (a: number, b: number) => {
    const arr = adjacency.get(a)
    if (arr) arr.push(b)
    else adjacency.set(a, [b])
  }
  for (const l of linkByKey.values()) {
    pushAdj(l.source_id, l.target_id)
    pushAdj(l.target_id, l.source_id)
  }
  const hopByNodeId = new Map<number, number>([[centerId, 0]])
  let frontier = [centerId]
  let hop = 0
  while (frontier.length > 0) {
    hop += 1
    const next: number[] = []
    for (const id of frontier) {
      for (const neighbor of adjacency.get(id) ?? []) {
        if (!hopByNodeId.has(neighbor)) {
          hopByNodeId.set(neighbor, hop)
          next.push(neighbor)
        }
      }
    }
    frontier = next
  }

  // 4. Keep only nodes reachable from the center (this is the collapse-orphan prune: a
  //    node left dangling after an expansion is removed has no path and drops out), and
  //    links whose both endpoints survived. The center is excluded from `nodes` to match
  //    the ArtistGraph contract (center is carried separately), but stays in hopByNodeId.
  const nodes = [...nodeById.values()].filter(
    n => n.id !== centerId && hopByNodeId.has(n.id),
  )
  const links = [...linkByKey.values()].filter(
    l => hopByNodeId.has(l.source_id) && hopByNodeId.has(l.target_id),
  )

  return { center: base.center, nodes, links, hopByNodeId }
}
