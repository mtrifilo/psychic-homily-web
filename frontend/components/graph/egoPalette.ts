/**
 * Ego-graph relationship-type node fills (locked design: "Option B").
 *
 * Ego payloads carry no cluster_id, so ego neighbor fills key to the
 * relationship TYPE of the node's connecting edge(s) instead — mapped onto
 * the same `--chart-*` tokens the cluster surfaces use, so the whole app
 * speaks one color language (locked grammar decision: one shared palette
 * on every surface):
 *
 *   bills   → --chart-1   (shared_bills)
 *   label   → --chart-6   (shared_label)
 *   members → --chart-7   (member_of, side_project)
 *   radio   → --chart-8   (radio_cooccurrence)
 *
 * side_project rides with member_of: both are binary membership facts (the
 * edge grammar already groups them — same dash pattern, same uniform
 * stroke), and "X is a side project of Y" is a shared-members claim.
 * Types outside the four families (similar, festival_cobill, any future
 * type) fall back to the neutral "other" fill — visibly unclassified, same
 * neutral ForceGraphView uses for ungrouped nodes — rather than borrowing
 * a family hue they don't mean.
 *
 * Multi-type connections tie-break on a fixed priority: bills > members >
 * label > radio — bills is the product's differentiator and the most
 * specific relationship.
 */

import { clusterColor, clusterColorCSS, type GraphPalette } from './graphPalette'

/** Fill families in TIE-BREAK PRIORITY order (first match wins). */
export const EGO_FAMILY_PRIORITY = ['bills', 'members', 'label', 'radio'] as const
export type EgoFillFamily = (typeof EGO_FAMILY_PRIORITY)[number]

/** 0-based `--chart-{i+1}` token index per family (locked mapping). */
export const EGO_FAMILY_CHART_INDEX: Record<EgoFillFamily, number> = {
  bills: 0, // --chart-1
  label: 5, // --chart-6
  members: 6, // --chart-7
  radio: 7, // --chart-8
}

/**
 * Legend display order: chart-index ascending (bills, label, members,
 * radio — the mocked canvas-foot order). Derived from the chart mapping so
 * a future family can't be colored on canvas yet forgotten here.
 */
export const EGO_FAMILY_LEGEND_ORDER: readonly EgoFillFamily[] = [...EGO_FAMILY_PRIORITY].sort(
  (a, b) => EGO_FAMILY_CHART_INDEX[a] - EGO_FAMILY_CHART_INDEX[b],
)

const FAMILY_BY_EDGE_TYPE: Record<string, EgoFillFamily> = {
  shared_bills: 'bills',
  shared_label: 'label',
  member_of: 'members',
  side_project: 'members',
  radio_cooccurrence: 'radio',
}

/** Family for a single edge type; null for types outside the four families. */
export function egoFamilyForEdgeType(type: string): EgoFillFamily | null {
  return FAMILY_BY_EDGE_TYPE[type] ?? null
}

/**
 * Resolve a node's fill family from ALL its connecting edge types via the
 * fixed tie-break priority. Null when no type maps (neutral fill).
 */
export function egoFamilyForTypes(types: Iterable<string>): EgoFillFamily | null {
  const families = new Set<EgoFillFamily>()
  for (const type of types) {
    const family = FAMILY_BY_EDGE_TYPE[type]
    if (family) families.add(family)
  }
  for (const family of EGO_FAMILY_PRIORITY) {
    if (families.has(family)) return family
  }
  return null
}

/** Canvas fill for a family (theme-resolved); neutral grey for null. */
export function egoFamilyFill(palette: GraphPalette, family: EgoFillFamily | null): string {
  if (family === null) return palette.otherCluster
  return clusterColor(palette, EGO_FAMILY_CHART_INDEX[family])
}

/** `var()` expression for a family swatch on DOM surfaces (legend pills). */
export function egoFamilyFillCSS(family: EgoFillFamily | null): string {
  if (family === null) return clusterColorCSS(-1)
  return clusterColorCSS(EGO_FAMILY_CHART_INDEX[family])
}

/** Minimal typed-edge shape for fill assignment (bare numeric endpoints). */
export interface EgoTypedEdge {
  sourceId: number
  targetId: number
  type: string
}

/**
 * Assign each non-center node its fill family from the edge(s) CONNECTING it
 * into the ego — the edges to its parent ring (one hop closer to the
 * center), per the locked "fill keys to the relationship type of its
 * connecting edge" rule. Same-hop cross-connections deliberately don't
 * color either endpoint: a radio neighbor that happens to share a bill
 * with ANOTHER neighbor is still a radio neighbor of the center.
 *
 * `hopByNodeId` is the merged-ego hop map (expand-on-demand); absent (the
 * bill-composition mini graph) every non-center node is hop 1, so only
 * edges touching the center assign fills. Multi-type ties resolve via
 * egoFamilyForTypes' fixed priority. Nodes whose connecting edges all
 * fall outside the four families map to null (neutral fill).
 */
export function egoFamilyByNodeId(
  edges: Iterable<EgoTypedEdge>,
  centerId: number,
  hopByNodeId?: ReadonlyMap<number, number>,
): Map<number, EgoFillFamily | null> {
  const hopOf = (id: number): number =>
    id === centerId ? 0 : (hopByNodeId?.get(id) ?? 1)

  const typesByNode = new Map<number, Set<string>>()
  const addType = (id: number, type: string) => {
    const set = typesByNode.get(id)
    if (set) set.add(type)
    else typesByNode.set(id, new Set([type]))
  }
  for (const edge of edges) {
    const sourceHop = hopOf(edge.sourceId)
    const targetHop = hopOf(edge.targetId)
    if (sourceHop === targetHop + 1) addType(edge.sourceId, edge.type)
    else if (targetHop === sourceHop + 1) addType(edge.targetId, edge.type)
  }

  const out = new Map<number, EgoFillFamily | null>()
  for (const [id, types] of typesByNode) {
    out.set(id, egoFamilyForTypes(types))
  }
  return out
}

export interface EgoLegendRow {
  /** Stable row key (family name, or 'other' for the neutral bucket). */
  key: EgoFillFamily | 'other'
  label: string
  /** CSS color expression for the swatch (theme-reactive var()). */
  swatchCSS: string
}

/**
 * Legend rows for the fill families actually assigned to rendered nodes:
 * one row per family present, in the mocked display order, plus a single
 * trailing neutral "other" row when any node renders the neutral fill.
 * Keyed off assignments (not raw edge types) so the legend never names a
 * color no node is wearing — the EdgeLegend (top-right) already itemizes
 * the raw edge types.
 */
export function egoLegendRows(
  familiesPresent: Iterable<EgoFillFamily | null>,
): EgoLegendRow[] {
  const families = new Set<EgoFillFamily>()
  let hasNeutral = false
  for (const family of familiesPresent) {
    if (family === null) hasNeutral = true
    else families.add(family)
  }
  const rows: EgoLegendRow[] = EGO_FAMILY_LEGEND_ORDER.filter(f => families.has(f)).map(
    family => ({
      key: family,
      label: family,
      swatchCSS: egoFamilyFillCSS(family),
    }),
  )
  if (hasNeutral) {
    rows.push({ key: 'other', label: 'other', swatchCSS: egoFamilyFillCSS(null) })
  }
  return rows
}
