/**
 * Label hub presentation helpers (PSY-1530, Figma node 1137:2).
 *
 * A label hub stands in for a roster: its spokes replace the C(n,2) pairwise
 * `shared_label` clique those artists would otherwise contribute (measured on
 * Austin: 300 of 302 edges were one label). The hub is a first-class scene node
 * — "25 artists are on 12XU" is the headline fact a visitor wants — so it is
 * drawn larger than an artist and always labeled.
 *
 * Hubs are deliberately NOT gated to scene-local labels (locked decision): a
 * metro gate would strip New York back to two edges and misfire on labels with
 * no city on file. Instead the canvas captions the hub's home, so an
 * out-of-town anchor reads as out-of-town without a fetch.
 *
 * KEEP THIS MODULE DEPENDENCY-FREE. Despite living under `components/graph/`,
 * it is imported by `features/graph/sceneMap.ts`, which is on the EDGE runtime
 * path for the `/graph/this-week` share card (see `graphOverviewApi.ts` — that
 * function already sits near Vercel's 1 MB per-function limit). Today the only
 * import here is `formatLocation`, which imports nothing, and the caption code
 * minifies into that bundle at a few hundred bytes. Adding an import that pulls
 * React, d3, or react-force-graph — all of which sibling modules in this
 * directory do pull — would blow that budget AT DEPLOY, not at build. See
 * `pattern_og_edge_size_and_font_hermeticity`.
 */

import { formatLocation } from '@/lib/formatLocation'

/**
 * The `entity_type` value that renders as a label hub. Mirrors the backend
 * `SceneNodeKindLabel` discriminator.
 *
 * This lives here rather than in ForceGraphView deliberately: consumer tests
 * `vi.mock()` that component wholesale, so a constant exported from it
 * resolves to `undefined` inside those tests unless every mock remembers to
 * re-export it. A plain module nobody mocks has no such trap.
 */
export const LABEL_HUB_ENTITY_TYPE = 'label'

/**
 * The membership edge type between a hub and one of its roster artists.
 * Mirrors the backend `SceneEdgeTypeOnLabel`.
 */
export const LABEL_HUB_SPOKE_EDGE_TYPE = 'on_label'

/**
 * True when a node should render as a label hub rather than an artist circle.
 * Typed structurally so this module stays free of graph-component imports.
 */
export function isLabelHubNode(node: { entity_type?: string }): boolean {
  return node.entity_type === LABEL_HUB_ENTITY_TYPE
}

/**
 * The cluster a node participates in for layout, hulls, and legend filtering.
 *
 * Artists fall back to the "other" bucket when ungrouped, but a label hub gets
 * NO cluster: clusters describe where artists play, and a hub has no primary
 * venue. Inheriting "other" would (a) hand the hub that cluster's centroid
 * force, dragging it off the roster it anchors, (b) place it inside that
 * cluster's hull, and (c) make a cluster-legend toggle able to hide a hub while
 * its roster's spokes remain. An empty cluster resolves to no centroid and no
 * hull (both look the ID up in the cluster map) and is never in the hidden set.
 */
export function graphClusterIdForNode(
  node: { entity_type?: string; cluster_id?: string },
  otherClusterId: string,
): string {
  if (isLabelHubNode(node)) return ''
  return node.cluster_id || otherClusterId
}

/**
 * Half-extent of the hub square, against NODE_RADIUS 8 / ISOLATE_RADIUS 5.
 * Bigger than an artist because it anchors a whole roster, small enough that a
 * dense scene with several hubs doesn't read as a second graph.
 */
export const LABEL_HUB_HALF_EXTENT = 11

/**
 * The hub's home caption, or undefined when the label has no location on file
 * (some labels are known only by name). Undefined — not a placeholder — because
 * a canvas caption reading "Location Unknown" under every unlocated label is
 * noise the panel can state more gracefully.
 *
 * Delegates the city/state/country rule to the shared PSY-558/780 helper, so a
 * hub captions its home exactly the way artist and venue surfaces do.
 *
 * THE ONLY HUB-CAPTION RULE, on every surface (PSY-1792). `/scenes`, the home
 * graph, and the Map of the Scene all caption through this function. The map
 * reads its parts off the nightly snapshot's `hub_city` / `hub_state` /
 * `hub_country` columns rather than off a live node, but it composes them HERE
 * — see `features/graph/sceneMap.ts`. Changing the rule here changes every
 * surface, which is the point; the map additionally needs a nightly rebuild
 * before a newly-emitted part can reach it.
 *
 * THE RULE IS SHARED; TRUNCATION IS NOT. `GraphLabelSpec.caption` requires each
 * caller to truncate for itself, and the callers differ: `SceneMapCanvas` runs
 * the result through `truncateLabel`, `ForceGraphView` does not, and the hub
 * context panels render it in full. Do not read "one rule" as "interchangeable
 * surfaces" — a composed caption is materially longer than the bare city this
 * replaced, and each surface decides what it can fit.
 */
export function labelHubHomeCaption(node: {
  city?: string
  state?: string
  country?: string
}): string | undefined {
  const formatted = formatLocation(node)
  return formatted === 'Location Unknown' ? undefined : formatted
}

/** Ring-radius bounds for a hub's roster, in graph units. */
export const SPOKE_REST_LENGTH_MIN = 60
export const SPOKE_REST_LENGTH_MAX = 170

/**
 * Rest length for a hub's membership spokes, given the hub's roster size.
 *
 * d3's ~30px default packs a roster onto a ring too small for its own labels —
 * the crowding hubs exist to remove. Ring circumference grows linearly with the
 * number of artists on it (2·pi·r >= n · spacing), so the radius scales with n
 * and is clamped: small rosters stay compact instead of flying apart, and a big
 * one stays inside the fitted viewport.
 */
export function spokeRestLength(rosterSize: number): number {
  const perArtist = 7
  return Math.min(
    SPOKE_REST_LENGTH_MAX,
    Math.max(SPOKE_REST_LENGTH_MIN, rosterSize * perArtist),
  )
}
