/**
 * graphAccessibleTree (PSY-1304) — turns an ego graph + its on-demand
 * expansions into a tree the accessible connections list renders.
 *
 * The canvas is `role="img"` (excluded from tab order); a keyboard / screen-
 * reader user drives the SAME expand-on-demand traversal through a `role="tree"`
 * list built from this data. Pure + framework-free so it's unit-testable and
 * shareable across graph surfaces (the ego dialog first; scene/station later).
 *
 * Structure mirrors the canvas's concentric-ring expand model:
 *   - the center's direct neighbours are the top-level items (level 1);
 *   - expanding an item (fetch + merge its ego) reveals ITS new neighbours as
 *     nested children (level 2+), exactly what a canvas node-click does.
 *
 * Dedup rule: every artist appears exactly ONCE, at its first-encountered
 * position (base neighbours before any expansion reveal; then depth-first).
 * A hop-2 artist reachable from two expanded parents shows under the first —
 * the same "one node, one place" the merged canvas graph enforces.
 */

/** Minimal node shape the tree needs — structural, not tied to a feature type. */
export interface AccessibleTreeGraphNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
}

/** A source ego graph: a center plus its neighbours. */
export interface AccessibleTreeGraph<N extends AccessibleTreeGraphNode> {
  center: { id: number }
  nodes: N[]
}

export interface AccessibleTreeItem<N extends AccessibleTreeGraphNode> {
  node: N
  /** 1 = the center's direct neighbour. */
  level: number
  /** This item's ego has been fetched+merged (children are shown). */
  expanded: boolean
  /** An expand fetch for this item is in flight. */
  expanding: boolean
  children: AccessibleTreeItem<N>[]
}

/** A flattened, currently-visible row (collapsed subtrees omitted). */
export interface AccessibleTreeRow<N extends AccessibleTreeGraphNode> {
  node: N
  level: number
  expanded: boolean
  expanding: boolean
  /** 1-based position among its siblings — for aria-posinset. */
  posInSet: number
  /** Sibling count — for aria-setsize. */
  setSize: number
}

/**
 * Build the tree from a base ego graph and its expansions.
 *
 * @param base         the current center's ego graph (neighbours = level 1).
 * @param expansions   node id → that node's fetched ego graph (its neighbours).
 * @param expandingIds nodes whose expand fetch is in flight.
 * @param rankByNodeId optional score (higher = earlier); ties + absent fall
 *                     back to name order, so the list order is deterministic
 *                     and matches the canvas's DOI ranking when supplied.
 * @param visibleNodeIds optional filter — when given, only these node ids appear
 *                     (others are pruned with their subtree). Pass the canvas's
 *                     activeTypes-filtered node set so the tree can't list (or
 *                     let the user expand) artists the type filter has hidden.
 */
export function buildGraphTree<N extends AccessibleTreeGraphNode>(
  base: AccessibleTreeGraph<N>,
  expansions: ReadonlyMap<number, AccessibleTreeGraph<N>>,
  expandingIds: ReadonlySet<number>,
  rankByNodeId?: ReadonlyMap<number, number>,
  visibleNodeIds?: ReadonlySet<number>,
): AccessibleTreeItem<N>[] {
  const isVisible = (id: number) => visibleNodeIds == null || visibleNodeIds.has(id)

  // Claim the center AND every base neighbour up front: base neighbours are
  // always level-1, so an expansion can never pull one down into a nested
  // child (which would happen if we discovered it mid-DFS before its own root
  // turn came up).
  const seen = new Set<number>([base.center.id])
  for (const n of base.nodes) seen.add(n.id)

  const sortNodes = (nodes: N[]): N[] =>
    [...nodes].sort((a, b) => {
      if (rankByNodeId) {
        const ra = rankByNodeId.get(a.id) ?? -Infinity
        const rb = rankByNodeId.get(b.id) ?? -Infinity
        if (ra !== rb) return rb - ra
      }
      return a.name.localeCompare(b.name)
    })

  const build = (node: N, level: number): AccessibleTreeItem<N> => {
    const ego = expansions.get(node.id)
    const item: AccessibleTreeItem<N> = {
      node,
      level,
      expanded: ego != null,
      expanding: expandingIds.has(node.id),
      children: [],
    }
    if (ego) {
      for (const child of sortNodes(ego.nodes)) {
        if (seen.has(child.id) || !isVisible(child.id)) continue
        seen.add(child.id)
        item.children.push(build(child, level + 1))
      }
    }
    return item
  }

  return sortNodes(base.nodes)
    .filter(n => isVisible(n.id))
    .map(n => build(n, 1))
}

/**
 * Flatten the tree to the rows currently visible (a collapsed item's subtree
 * is omitted), tagging each with posInSet/setSize among its siblings.
 */
export function flattenVisibleTree<N extends AccessibleTreeGraphNode>(
  roots: AccessibleTreeItem<N>[],
): AccessibleTreeRow<N>[] {
  const out: AccessibleTreeRow<N>[] = []
  const walk = (items: AccessibleTreeItem<N>[]) => {
    items.forEach((item, i) => {
      out.push({
        node: item.node,
        level: item.level,
        expanded: item.expanded,
        expanding: item.expanding,
        posInSet: i + 1,
        setSize: items.length,
      })
      // children are only populated for expanded items, so length is sufficient.
      if (item.children.length > 0) walk(item.children)
    })
  }
  walk(roots)
  return out
}
