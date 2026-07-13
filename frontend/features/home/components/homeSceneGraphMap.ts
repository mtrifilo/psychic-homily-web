import type {
  SceneGraphLink,
  SceneGraphNode,
} from '@/features/scenes/types'

export const HOME_GRAPH_MAX_NODES = 20

export interface HomeGraphLabelStyle {
  fontSize: 11 | 13 | 17
  fontWeight: 400 | 500 | 600
}

export interface HomeSceneGraphMap {
  nodes: SceneGraphNode[]
  links: SceneGraphLink[]
  labelStyles: ReadonlyMap<number, HomeGraphLabelStyle>
  showChipNodes: SceneGraphNode[]
}

const LABEL_TIERS: readonly HomeGraphLabelStyle[] = [
  { fontSize: 17, fontWeight: 600 },
  { fontSize: 13, fontWeight: 500 },
  { fontSize: 11, fontWeight: 400 },
]

/**
 * Build the homepage's bounded "map of names" from one settled scene payload.
 * Equal-weight degree + upcoming-show count is the deliberately simple activity
 * blend: connections keep the map graph-shaped while current bookings lift
 * active artists. Every tie-break is explicit so cached scenes lay out from the
 * same ordered input on every visit.
 */
export function buildHomeSceneGraphMap(
  nodes: readonly SceneGraphNode[],
  links: readonly SceneGraphLink[],
): HomeSceneGraphMap {
  const degrees = new Map<number, number>()
  for (const link of links) {
    degrees.set(link.source_id, (degrees.get(link.source_id) ?? 0) + 1)
    degrees.set(link.target_id, (degrees.get(link.target_id) ?? 0) + 1)
  }

  const rankedNodes = nodes
    .filter(node => !node.is_isolate)
    .map(node => ({
      node,
      degree: degrees.get(node.id) ?? 0,
      activity: (degrees.get(node.id) ?? 0) + node.upcoming_show_count,
    }))
    .sort(
      (a, b) =>
        b.activity - a.activity ||
        b.degree - a.degree ||
        b.node.upcoming_show_count - a.node.upcoming_show_count ||
        a.node.name.localeCompare(b.node.name) ||
        a.node.id - b.node.id,
    )
    .slice(0, HOME_GRAPH_MAX_NODES)

  const selectedIds = new Set(rankedNodes.map(({ node }) => node.id))
  const selectedLinks = links.filter(
    link => selectedIds.has(link.source_id) && selectedIds.has(link.target_id),
  )
  const tierSize = Math.ceil(rankedNodes.length / LABEL_TIERS.length)
  const labelStyles = new Map<number, HomeGraphLabelStyle>()
  rankedNodes.forEach(({ node }, index) => {
    const tierIndex = Math.min(
      LABEL_TIERS.length - 1,
      Math.floor(index / Math.max(1, tierSize)),
    )
    labelStyles.set(node.id, LABEL_TIERS[tierIndex])
  })

  const selectedNodes = rankedNodes.map(({ node }) => node)
  return {
    nodes: selectedNodes,
    links: selectedLinks,
    labelStyles,
    showChipNodes: selectedNodes.filter(node => node.next_show).slice(0, 2),
  }
}
