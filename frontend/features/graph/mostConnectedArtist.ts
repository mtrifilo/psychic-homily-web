import { isArtistNode, isArtistSlug } from './graphRootLink'

/**
 * Which artist a surface's `/graph` link opens the map rooted on when the
 * surface ranks its artists by how connected they are on its own canvas.
 *
 * The pool is the drawn payload's node set, so the link names something the
 * reader just saw. Degree is counted over that same payload's links, which
 * means a backend node cap narrows the pool and the ranking together.
 *
 * Distinct from the scene rule, which ranks by upcoming bookings: station,
 * collection and venue payloads carry no booking signal worth ranking on
 * (`upcoming_show_count` is catalog-wide on a station node and zero on a
 * non-artist collection node), while every one of them carries edges.
 *
 * KEEP THIS MODULE DEPENDENCY-FREE beyond the link helper next to it: it is
 * imported by three feature chunks that must not pull the Observatory canvas
 * in through a link helper.
 */

/** The node fields the pick reads, structural so callers keep their own types. */
interface ConnectedCandidateNode {
  /** Matches the `source_id` / `target_id` an edge references. */
  id: number
  slug: string
  name: string
  entity_type?: string
}

/** The edge fields the pick reads. */
interface ConnectedCandidateEdge {
  source_id: number
  target_id: number
}

/**
 * Edges touching each drawn node, counted once per endpoint.
 *
 * Only ids the payload draws are keyed, so an edge naming a node outside the
 * payload counts for nobody.
 */
function countDegrees(
  nodes: readonly ConnectedCandidateNode[],
  links: readonly ConnectedCandidateEdge[],
): Map<number, number> {
  const degrees = new Map<number, number>()
  for (const node of nodes) degrees.set(node.id, 0)
  const bump = (id: number) => {
    const current = degrees.get(id)
    if (current !== undefined) degrees.set(id, current + 1)
  }
  for (const link of links) {
    bump(link.source_id)
    bump(link.target_id)
  }
  return degrees
}

/**
 * The slug of the most connected artist in the payload, ties broken by name.
 *
 * Null when the payload is missing, empty, or holds no artist the Observatory
 * would accept, which leaves the surface's link on the plain map.
 *
 * A node whose slug the Observatory would refuse is not a candidate, so the
 * link falls through to the next artist rather than to nothing: the slug column
 * is a free nullable string, an empty one builds a link that resolves to the
 * artists index rather than a 404, and a value outside the generated shape is
 * one the reader will not honour.
 *
 * An artist no edge touches is still a candidate at degree 0. A surface with a
 * canvas of isolates has a neighbourhood worth opening on; the ranking, not a
 * separate floor, decides that such an artist only wins when nothing on the
 * canvas is connected.
 */
export function pickMostConnectedArtistSlug(
  nodes: readonly ConnectedCandidateNode[] | null | undefined,
  links: readonly ConnectedCandidateEdge[] | null | undefined,
): string | null {
  const candidates = (nodes ?? []).filter(
    node => isArtistSlug(node.slug) && isArtistNode(node),
  )
  if (candidates.length === 0) return null
  const degrees = countDegrees(nodes ?? [], links ?? [])
  const degreeOf = (node: ConnectedCandidateNode) => degrees.get(node.id) ?? 0
  /** The ranking rule, spelled once: most edges first, then by name. */
  const byDegreeThenName = (a: ConnectedCandidateNode, b: ConnectedCandidateNode) =>
    degreeOf(b) - degreeOf(a) || a.name.localeCompare(b.name)
  return candidates.reduce((leader, node) =>
    byDegreeThenName(node, leader) < 0 ? node : leader,
  ).slug
}
