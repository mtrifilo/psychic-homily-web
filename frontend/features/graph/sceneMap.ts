/**
 * The Map of the Scene: turning the nightly `GET /graph/overview` snapshot into
 * something a canvas can draw (PSY-1725).
 *
 * The payload is COLUMNAR and quantized — parallel arrays indexed by node
 * position, int16 coordinates, base64 byte columns, CSR edges. That encoding is
 * right for the wire and wrong for a render loop, so this module is the ONE
 * place that speaks it: everything downstream sees plain objects with world
 * coordinates. Pure and framework-free, so the decode rules are unit-tested
 * rather than inferred from a canvas screenshot.
 *
 * Three payload asymmetries this module absorbs, each of which is silent
 * (wrong pixels, no error) if a consumer gets it wrong:
 *
 *  1. `nodes.kind`, `nodes.flags` and `edges.kind` are BASE64 STRINGS, not
 *     arrays — Go encodes any `[]uint8` that way. Indexing them as arrays
 *     yields characters.
 *  2. CSR holds BOTH directions of every edge, so drawing every slot draws
 *     every edge twice. We emit a slot only when `target > source`.
 *  3. Coordinates are int16 in ±32767. Drawn at that scale an 8px node is
 *     invisible dust, so they are rescaled once, here, into the world units
 *     the shared node/label geometry was tuned for.
 */

import type { components } from '@/types/api'

export type GraphOverview = components['schemas']['GraphOverview']

/**
 * Payload schema version this module understands, mirroring the backend's
 * `GraphOverviewVersion`. A snapshot written by a NEWER build is refused
 * outright (see `buildSceneMap`) rather than half-read: the columns whose
 * meaning changed would render as plausible-looking wrong positions.
 */
export const SCENE_MAP_SUPPORTED_VERSION = 1

/** Mirrors the backend `GraphOverviewNode*` constants. */
const NODE_KIND_ARTIST = 0
const NODE_KIND_LABEL_HUB = 1

/** Mirrors the backend `GraphOverviewEdge*` constants. */
const EDGE_KIND_SIMILARITY = 0
const EDGE_KIND_LABEL_SPOKE = 1

/** Mirrors the backend `GraphOverviewFlag*` bits. */
const FLAG_PLAYABLE_AUDIO = 1 << 0
const FLAG_UPCOMING_SHOW = 1 << 1

/** Mirrors the backend `GraphOverviewCoordinateScale`. */
const QUANTIZED_COORDINATE_SCALE = 32767

/**
 * Half-width of the drawn map in graph world units.
 *
 * The snapshot's own `extent` is deliberately ignored: world units are only
 * meaningful relative to each other, and what actually has to line up is the
 * map against the FIXED pixel geometry the shared modules assume (NODE_RADIUS
 * 8, LABEL_HUB_HALF_EXTENT 11, label font sizes). Pinning the half-width to a
 * constant makes node size relative to map size identical for every snapshot,
 * so a night that grows the catalog can't quietly shrink every dot.
 *
 * The VALUE was chosen against a real render, not derived: zoom-to-fit on the
 * shipped canvas box lands the map at roughly 0.4x, which is where an artist
 * dot is a legible few pixels and a label hub still reads as a square. Raising
 * it shrinks every dot; lowering it packs the dense communities into blobs.
 */
export const SCENE_MAP_WORLD_HALF_EXTENT = 550

const WORLD_UNITS_PER_QUANT = SCENE_MAP_WORLD_HALF_EXTENT / QUANTIZED_COORDINATE_SCALE

export type SceneMapNodeKind = 'artist' | 'label'

/** One drawn node, in world coordinates. */
export interface SceneMapNode {
  /** Entity id — an artist id, or a label-hub id from the reserved hub range. */
  id: number
  kind: SceneMapNodeKind
  name: string
  slug: string
  x: number
  y: number
  /** Leiden community id, or -1 for a node in none (every label hub). */
  community: number
  /** Degree in THIS map's edge set — for a hub, its drawn roster size. */
  degree: number
  /** Centrality rank, 0 = most central. Drives label tiering. */
  rank: number
  hasUpcomingShow: boolean
  hasPlayableAudio: boolean
  /**
   * A label hub's home city, or null (PSY-1736). Null at every ARTIST node —
   * the snapshot's `hub_city` column is the hub caption, not a location column
   * — and null at a hub whose label has no city on file, which is the majority
   * case and reads as no caption rather than a placeholder.
   *
   * City only, by the locked caption rule: no state, no country, no fallback.
   * The backend has already trimmed it, so a non-null value is drawable text.
   */
  homeCity: string | null
  /**
   * When this node entered the catalog, in seconds after `SceneMap.epoch` —
   * the clock the growth replay runs on (PSY-1737). The backend derives it
   * from the entity's `created_at` and its earliest show date, never from a
   * relationship row (which stamps a derive run rather than an event).
   *
   * 0 when the snapshot carries no appear column, which reads as "was always
   * here": a replay built on that data is uninteresting, not wrong, and
   * `buildReplayTimeline` refuses it outright.
   */
  appear: number
}

export type SceneMapEdgeKind = 'similarity' | 'spoke'

/** One drawn edge, already de-duplicated out of the bidirectional CSR slots. */
export interface SceneMapEdge {
  source: number
  target: number
  kind: SceneMapEdgeKind
  /**
   * When the edge appeared, on the same clock as `SceneMapNode.appear`. The
   * backend already resolves it to the LATER of the two endpoints — an edge
   * cannot predate either — so the replay never has to draw a line into a dot
   * that is not on the map yet.
   */
  appear: number
}

/** One community's labelled area, hull points already in world coordinates. */
export interface SceneMapRegion {
  community: number
  /** Display name, "Around {artist}". */
  label: string
  memberCount: number
  /** Closed polygon, counter-clockwise, no repeated final point. May be empty. */
  hull: Array<[number, number]>
  /** Centroid of the hull — where the region caption is drawn. Null when no hull. */
  captionAnchor: [number, number] | null
}

/** The decoded map plus the counts the surrounding card reports. */
export interface SceneMap {
  nodes: SceneMapNode[]
  edges: SceneMapEdge[]
  regions: SceneMapRegion[]
  /** Artists drawn on the map (excludes label hubs). */
  artistCount: number
  /** Label hubs drawn on the map. */
  labelCount: number
  /** Catalog artists with no surviving edge, reported but not drawn. */
  isolateCount: number
  lastMapped: Date
  /** Origin every `appear` value counts seconds from. */
  epoch: Date
}

/**
 * Decode one of the base64 byte columns to the bytes it stands for.
 *
 * `atob` is the browser's own decoder and is present in jsdom and in every
 * runtime target here. A column whose decoded length disagrees with the count
 * it is indexed against is a corrupt snapshot, not a recoverable state — the
 * caller refuses the whole payload rather than rendering a map whose flags are
 * off by one node.
 */
export function decodeByteColumn(encoded: string, expectedLength: number): Uint8Array | null {
  let binary: string
  try {
    binary = atob(encoded)
  } catch {
    return null
  }
  if (binary.length !== expectedLength) return null
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

/**
 * Both kind decoders fall back to the kind the backend numbers 0 rather than
 * rejecting an unrecognised byte: a future build that adds a third kind should
 * put a node on the map looking like an artist, not vanish it.
 */
function nodeKindFromByte(byte: number): SceneMapNodeKind {
  switch (byte) {
    case NODE_KIND_LABEL_HUB:
      return 'label'
    case NODE_KIND_ARTIST:
    default:
      return 'artist'
  }
}

function edgeKindFromByte(byte: number): SceneMapEdgeKind {
  switch (byte) {
    case EDGE_KIND_LABEL_SPOKE:
      return 'spoke'
    case EDGE_KIND_SIMILARITY:
    default:
      return 'similarity'
  }
}

/**
 * Every `Nodes.*` column must have length `node_count` for index i to describe
 * the same node in all of them. A short column would otherwise surface as
 * `undefined` coordinates on the tail nodes — which d3 turns into NaN and the
 * canvas draws as nothing, with no error anywhere.
 */
function nodeColumnsAreWellFormed(overview: GraphOverview): boolean {
  const nodes = overview.nodes
  return [
    nodes.id,
    nodes.name,
    nodes.slug,
    nodes.x,
    nodes.y,
    nodes.community,
    nodes.degree,
    nodes.rank,
  ].every(column => column?.length === overview.node_count)
}

function toWorld(quantized: number): number {
  return quantized * WORLD_UNITS_PER_QUANT
}

/**
 * Mean of the hull vertices — where a region's caption sits.
 *
 * Deliberately the vertex mean and not the area centroid: the hull is a padded
 * convex outline of a blob of artists, so the two land within a few pixels of
 * each other, and the vertex mean cannot divide by a zero area on the
 * degenerate hulls (collinear points) the backend can legitimately emit.
 */
function hullCentroid(hull: Array<[number, number]>): [number, number] | null {
  if (hull.length === 0) return null
  let sumX = 0
  let sumY = 0
  for (const [x, y] of hull) {
    sumX += x
    sumY += y
  }
  return [sumX / hull.length, sumY / hull.length]
}

/**
 * Decode a snapshot into drawable form, or `null` when the payload cannot be
 * trusted (unsupported version, an empty map, or columns that disagree with
 * the counts they are indexed against).
 *
 * `null` is the caller's cue to fall back to the search-first hero — the same
 * branch a pre-first-snapshot 503 takes. That collapse is deliberate: a map we
 * can't read and a map that doesn't exist yet are the same thing to a visitor,
 * and the hero is a working surface rather than an apology.
 */
export function buildSceneMap(overview: GraphOverview): SceneMap | null {
  if (overview.version > SCENE_MAP_SUPPORTED_VERSION) return null
  const nodeCount = overview.node_count
  if (nodeCount <= 0) return null
  if (!nodeColumnsAreWellFormed(overview)) return null

  const kinds = decodeByteColumn(overview.nodes.kind, nodeCount)
  const flags = decodeByteColumn(overview.nodes.flags, nodeCount)
  if (!kinds || !flags) return null

  // Non-null after nodeColumnsAreWellFormed; the locals keep the loop readable.
  const ids = overview.nodes.id!
  const names = overview.nodes.name!
  const slugs = overview.nodes.slug!
  const xs = overview.nodes.x!
  const ys = overview.nodes.y!
  const communities = overview.nodes.community!
  const degrees = overview.nodes.degree!
  const ranks = overview.nodes.rank!
  // Deliberately NOT part of `nodeColumnsAreWellFormed`. Every other column
  // decides where a dot goes or what it says, so a short one means the map is
  // wrong; `appear` only feeds the optional growth replay. A snapshot missing
  // it should still DRAW — it just cannot be replayed, which is the decision
  // `buildReplayTimeline` makes on its own.
  const appears = appearColumn(overview.nodes.appear, nodeCount)
  // Optional for the same reason `appear` is, and then some: a snapshot built
  // before the column existed carries none at all, and the map is worth drawing
  // without its hub captions. Length-checked so a short column cannot caption
  // the wrong hub — an off-by-one here would put a real city under a label that
  // is not from there, which is worse than no caption at all.
  const hubCities = stringColumn(overview.nodes.hub_city, nodeCount)

  const nodes: SceneMapNode[] = new Array(nodeCount)
  let artistCount = 0
  let labelCount = 0
  for (let i = 0; i < nodeCount; i += 1) {
    const kind = nodeKindFromByte(kinds[i])
    if (kind === 'label') labelCount += 1
    else artistCount += 1
    nodes[i] = {
      id: ids[i],
      kind,
      name: names[i],
      slug: slugs[i],
      x: toWorld(xs[i]),
      y: toWorld(ys[i]),
      community: communities[i],
      degree: degrees[i],
      rank: ranks[i],
      hasUpcomingShow: (flags[i] & FLAG_UPCOMING_SHOW) !== 0,
      hasPlayableAudio: (flags[i] & FLAG_PLAYABLE_AUDIO) !== 0,
      homeCity: hubCities?.[i] || null,
      appear: appears ? appears[i] : 0,
    }
  }

  return {
    nodes,
    edges: decodeEdges(overview, nodes),
    regions: decodeRegions(overview),
    artistCount,
    labelCount,
    isolateCount: Math.max(0, overview.isolate_count),
    lastMapped: new Date(overview.last_mapped),
    epoch: new Date(overview.epoch),
  }
}

/**
 * An `appear` column, or null when the snapshot does not carry a usable one.
 *
 * Length is checked against the count the column is indexed by for the same
 * reason every other column is: a short one would surface as `undefined` on the
 * tail entries, and `undefined` in the reveal maths is NaN — which smoothsteps
 * to 0 and hides those nodes for the whole run, silently.
 */
function appearColumn(column: number[] | null | undefined, expectedLength: number): number[] | null {
  if (!column || column.length !== expectedLength) return null
  return column
}

/**
 * An optional string column, or null when the snapshot does not carry a usable
 * one — the `appear` rule applied to `hub_city`.
 *
 * Absent is the NORMAL state for a snapshot older than the column, not a
 * corruption, which is why this returns null instead of failing the payload the
 * way a short `x` column does.
 */
function stringColumn(
  column: string[] | null | undefined,
  expectedLength: number,
): string[] | null {
  if (!column || column.length !== expectedLength) return null
  return column
}

/**
 * Walk the CSR neighbour slots into a de-duplicated edge list.
 *
 * `offsets` has length node_count+1 and `targets` holds both directions, so
 * node i's slots are `targets[offsets[i]..offsets[i+1]]`. Emitting only the
 * slots where `target > source` keeps each edge exactly once AND makes the
 * output order deterministic (ascending source, then payload slot order), so
 * the canvas draws the same edge list every mount.
 *
 * A malformed offsets/targets pair yields fewer edges rather than throwing:
 * the map is still worth drawing without some of its lines, and the node
 * columns have already passed their own integrity gate.
 */
function decodeEdges(overview: GraphOverview, nodes: SceneMapNode[]): SceneMapEdge[] {
  const offsets = overview.edges.offsets
  const targets = overview.edges.targets
  if (!offsets || !targets) return []
  if (offsets.length !== nodes.length + 1) return []

  const kinds = decodeByteColumn(overview.edges.kind, targets.length)
  // Per SLOT, index-aligned with `targets` — not per edge. Reading it with an
  // edge index would date every line by whatever unrelated slot sat there.
  const appears = appearColumn(overview.edges.appear, targets.length)
  const edges: SceneMapEdge[] = []
  for (let source = 0; source < nodes.length; source += 1) {
    const start = offsets[source]
    const end = offsets[source + 1]
    // `end < start` is the guard that makes this loop LINEAR. CSR offsets are
    // non-decreasing by construction, so each slot belongs to exactly one
    // source and the whole walk is O(slots). Offsets that jump backwards —
    // a snapshot-writer regression, not anything a visitor can send — would
    // otherwise let sources re-walk overlapping ranges, turning an O(n + slots)
    // payload into O(n · slots) edge objects and freezing the tab before the
    // page's error boundary can catch anything.
    if (start == null || end == null || start < 0 || end > targets.length) continue
    if (end < start) continue
    for (let slot = start; slot < end; slot += 1) {
      const target = targets[slot]
      // The `>` (not `!==`) is what de-duplicates: the mirror slot on the other
      // endpoint fails this test. It also drops any self-loop.
      if (target <= source || target >= nodes.length) continue
      edges.push({
        source: nodes[source].id,
        target: nodes[target].id,
        // A kind column that failed to decode degrades every edge to the
        // similarity styling rather than dropping the edges entirely.
        kind: kinds ? edgeKindFromByte(kinds[slot]) : 'similarity',
        // The backend already resolved this to the later endpoint, but the
        // floor is kept here too: it is what guarantees the replay never draws
        // a line before both of its dots, whatever the snapshot says.
        appear: Math.max(
          appears ? appears[slot] : 0,
          nodes[source].appear,
          nodes[target].appear,
        ),
      })
    }
  }
  return edges
}

/**
 * Rescale each region's hull into world coordinates and pick its caption
 * anchor. Regions whose hull is empty (too few members to enclose an area)
 * survive with no hull and no anchor — they still count toward the footer's
 * region tally, they just have nothing to outline.
 */
function decodeRegions(overview: GraphOverview): SceneMapRegion[] {
  const regions = overview.regions
  if (!regions) return []
  return regions.map(region => {
    const hull: Array<[number, number]> = []
    for (const point of region.hull ?? []) {
      if (!point || point.length < 2) continue
      hull.push([toWorld(point[0]), toWorld(point[1])])
    }
    return {
      community: region.community,
      label: region.label,
      memberCount: region.member_count,
      hull,
      captionAnchor: hullCentroid(hull),
    }
  })
}

/**
 * Stable per-community color index into the shared `--chart-1..8` ramp.
 *
 * Modulo, not a hash: the backend numbers communities densely from 0, so
 * modulo spreads the biggest few (which are the ones with drawn hulls) across
 * distinct hues, and the SAME community keeps the SAME hue between nightly
 * builds as long as its id is stable. A node in no community (-1, every label
 * hub) returns -1, which `clusterColor` renders as the neutral grey.
 */
export function sceneMapColorIndex(community: number, rampSize: number): number {
  if (community < 0 || rampSize <= 0) return -1
  return community % rampSize
}

/** One region's worth of artists, for the map's list view. */
export interface SceneMapListGroup {
  key: string
  label: string
  nodes: SceneMapNode[]
}

/**
 * Group the map's artists under the regions the canvas names, biggest region
 * first and members by centrality — the same ordering the map expresses
 * visually through hull size and label tier.
 *
 * Label hubs carry no community by design (a hub anchors a roster, it does not
 * live in one), so they are NOT listed: a hub's dot opens a context card rather
 * than re-rooting, and every artist a hub connects is already listed under its
 * own region. Artists whose community has no region entry fall into a trailing
 * group, so the list can never come up short of the map.
 *
 * Lives here beside the decode rather than in the list component: it shapes map
 * data, and keeping it pure is what lets the grouping rules be tested without
 * rendering anything.
 */
export function groupNodesByRegion(map: SceneMap): SceneMapListGroup[] {
  const byCommunity = new Map<number, SceneMapNode[]>()
  for (const node of map.nodes) {
    if (node.kind !== 'artist') continue
    const bucket = byCommunity.get(node.community)
    if (bucket) bucket.push(node)
    else byCommunity.set(node.community, [node])
  }
  const byCentrality = (a: SceneMapNode, b: SceneMapNode) =>
    a.rank - b.rank || a.name.localeCompare(b.name)
  for (const bucket of byCommunity.values()) bucket.sort(byCentrality)

  const groups: SceneMapListGroup[] = []
  for (const region of map.regions) {
    const nodes = byCommunity.get(region.community)
    if (!nodes || nodes.length === 0) continue
    // Claimed as we go, so the leftover pass below needs no second bookkeeping
    // structure to know what it has already emitted.
    byCommunity.delete(region.community)
    groups.push({ key: `region-${region.community}`, label: region.label, nodes })
  }
  groups.sort((a, b) => b.nodes.length - a.nodes.length || a.label.localeCompare(b.label))

  const ungrouped: SceneMapNode[] = []
  for (const nodes of byCommunity.values()) ungrouped.push(...nodes)
  if (ungrouped.length > 0) {
    ungrouped.sort(byCentrality)
    groups.push({ key: 'ungrouped', label: 'Elsewhere on the map', nodes: ungrouped })
  }
  return groups
}
