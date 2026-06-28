/**
 * Degree-of-Interest (DOI) scoring for the artist ego graph (PSY-1273)
 *
 * PSY-1259 shipped expand-on-demand mechanics but treats every neighbor equally — no
 * relevance ordering and no guidance on WHICH nodes are worth expanding. The validated
 * model (van Ham & Perer, "Search, Show Context, Expand on Demand", IEEE InfoVis 2009;
 * see docs/open-questions/graph-density-discovery-redesign.md §3.4) ranks neighbors by a
 * Degree-of-Interest score and highlights a few expansion directions so the user is steered
 * toward hidden-but-interesting artists rather than clicking blindly.
 *
 *   DOI(x) = α·importance(x) + β·relevance(x) + γ·proximity(x)
 *
 * All three terms are computed CLIENT-SIDE from the merged graph the user is already
 * looking at — no backend (the PSY-1259 data-signal spike confirmed in-subgraph degree is
 * free; a richer importance signal — global degree / follower count — is a later backend
 * add). Weights are relevance-dominant per the PSY-1259 decisions (comment D), tuned visually.
 *
 * Scoped to the VISIBLE graph: scoring runs over exactly the edges the canvas draws — the
 * `activeTypes` toggles AND the per-node edge cap (the same two steps ArtistGraph.graphData
 * uses). So toggling a type off re-ranks, a node is only scored when it has an on-screen edge,
 * and a radio-dense node can't glow on capped-away ties — label priority + suggested directions
 * stay consistent with what's actually drawn (degree, not just node membership).
 *
 * Pure + UI-free, so it's unit-tested without the canvas like mergeEgoGraphs / edgeCap.
 *
 * Two consumers (both in RelatedArtists' RecenteringGraph):
 *   - `doiByNodeId` → label collision priority (the most-interesting names survive the cull).
 *   - `ranked` → fed to `selectSuggestedExpansions` to flag the top ≤5 unexpanded nodes.
 */

import { capEdgesPerNode, EDGE_CAP_BY_TYPE } from '@/components/graph/edgeCap'
import type { MergedEgoGraph } from './mergeEgoGraphs'

/**
 * DOI term DEFAULT weights. Relevance-dominant by decision (PSY-1259 comment D): users expect
 * "most-related first"; importance (centrality) breaks ties; proximity keeps nearby nodes
 * ahead of far ones. These are the "Popular" end of the PSY-1260 discovery-bias slider — the
 * default — which only moves the importance weight (see `doiWeightsForBias`).
 * Tuned visually; sum need not be 1 (ranking is scale-invariant), but kept at 1 for clarity.
 */
export const DOI_WEIGHTS = {
  importance: 0.3,
  relevance: 0.5,
  proximity: 0.2,
} as const

export interface DoiWeights {
  importance: number
  relevance: number
  proximity: number
}

/**
 * DOI weights for a "discovery bias" slider position (PSY-1260). `bias` ∈ [0,1]:
 *   0 = Popular (the default — favors high-degree hubs; identical to DOI_WEIGHTS)
 *   1 = Niche   (INVERTS the importance term so low-degree / serendipitous artists rank up)
 * Only the importance weight moves (+importance → 0 → −importance, linearly); relevance +
 * proximity stay fixed, so "most-related-first" still anchors the ranking — the slider re-orders
 * within similar-relevance neighbors, it doesn't abandon relevance. This is the MusicLynx
 * "diversity over accuracy" lever (docs/open-questions/graph-density-discovery-redesign.md §3.5),
 * expressed through the EXISTING importance term rather than a new signal (niche = low degree).
 */
export function doiWeightsForBias(bias: number): DoiWeights {
  const t = Math.min(1, Math.max(0, bias)) // clamp defensively
  return {
    importance: DOI_WEIGHTS.importance * (1 - 2 * t),
    relevance: DOI_WEIGHTS.relevance,
    proximity: DOI_WEIGHTS.proximity,
  }
}

export interface GraphDoi {
  /** DOI score per scored node id (NON-center nodes that still have an active-type edge — i.e.
   *  the nodes the canvas actually paints). The center is the anchor: never a suggestion, always
   *  force-labeled, so it carries no DOI. Range: with the default weights it's `[0, 1]`, but the
   *  PSY-1260 discovery-bias slider can drive the importance weight NEGATIVE (down to −0.3 at full
   *  Niche), so a high-degree/zero-relevance node can score below 0. Only RELATIVE order is
   *  meaningful — do not assume `≥ 0` or a fixed upper bound (see `doiWeightsForBias`). */
  doiByNodeId: Map<number, number>
  /** Scored node ids sorted by DOI desc (ties broken by id asc for determinism). */
  ranked: number[]
}

/**
 * Edge types with no magnitude — `member_of` / `side_project` are binary facts (X is a
 * member of Y, or not), drawn with a uniform stroke (see edgeGrammar.edgeWidth). A present
 * binary edge is therefore a full-strength tie, NOT something to normalize toward zero.
 * Every other type carries a magnitude `score` worth normalizing.
 */
const BINARY_EDGE_TYPES: ReadonlySet<string> = new Set(['member_of', 'side_project'])

/** Min-max normalize `value` into [0, 1] against the observed range. A degenerate range
 *  (all values equal — e.g. every node hop-1, or every node the same degree) maps to 1 so
 *  the term contributes a constant that doesn't perturb the ranking, rather than NaN. */
function normalize(value: number, min: number, max: number): number {
  if (max <= min) return 1
  return (value - min) / (max - min)
}

/**
 * Compute the DOI score for every scored node in the merged ego graph.
 *
 * `activeTypes` (optional) restricts scoring to edges of those types — pass the canvas's
 * active type toggles so DOI reflects the drawn graph (omit to consider every edge). Only
 * nodes that still have an active-type edge are scored; a node whose only ties were toggled
 * off isn't painted, so it gets no DOI and never surfaces as a suggestion.
 *
 * The three terms, each min-max normalized across the scored node set so the weights mean
 * what they say:
 *
 *   - importance = DISTINCT-NEIGHBOR count (in-subgraph degree) over the DRAWN edges. Each
 *     connected artist counts once, regardless of how many edge TYPES tie the pair, so a node
 *     isn't inflated just because the backend emitted both a `similar` and a `radio_cooccurrence`
 *     edge for one relationship; and capped-away ties don't count (degree matches what's drawn).
 *     The spike's free centrality proxy: more distinct neighbors = a richer expansion target.
 *     (The center counts as a neighbor.)
 *
 *   - relevance = strength of the node's STRONGEST tie, normalized PER EDGE TYPE first.
 *     Edge `score` is a per-type magnitude on different scales (similar = Wilson [0,1];
 *     shared_label / festival_cobill normalized [0,1]; shared_bills / radio_cooccurrence =
 *     weighted counts that can be >> 1; member_of / side_project = binary) — see
 *     components/graph/edgeGrammar.ts. So a raw max-incident-score would just rank
 *     radio-connected nodes highest by scale artifact. Dividing each edge by its type's max
 *     in this graph puts a "strong similar tie" and a "strong radio tie" on equal footing;
 *     binary types (no magnitude) are full-strength when present. A magnitude type that is
 *     all-zero in this subgraph (e.g. unvoted `similar`) yields 0 — the absence of magnitude
 *     means a weak tie, NOT a maximal one. For a hop-1 node the strongest tie is dominated
 *     by its edge to the focus (the "relevance to the anchored artist" the model intends);
 *     for a deeper node it generalizes to its strongest tie into the explored subgraph — a
 *     deliberate choice (a node well-integrated into what's shown is a good place to expand).
 *
 *   - proximity = nearness to the center: hop-1 = 1, deeper rings fall off linearly. Uses the
 *     merged (all-types) hop distance, which is the ring the node is actually drawn on —
 *     toggling an edge type off doesn't move a node to a different ring, so neither should it
 *     change the proximity term.
 *
 * Returns empty maps when no node has an active-type edge (e.g. a center with no neighbors,
 * or every type toggled off).
 */
export function computeGraphDoi(
  merged: MergedEgoGraph,
  activeTypes?: ReadonlySet<string>,
  weights: DoiWeights = DOI_WEIGHTS,
): GraphDoi {
  const centerId = merged.center.id
  // Score over exactly the DRAWN graph, in the same two steps the canvas uses
  // (ArtistGraph.graphData): (1) drop edges of toggled-off types, (2) apply the per-node edge
  // cap. Without the cap, a radio-dense node's importance/relevance would count the weak ties
  // the canvas trims away (top-k keeps the strongest), so it could glow / win a label while
  // looking sparse on screen. The cap's no-orphan invariant (k>=1) keeps every still-connected
  // node's strongest edge, so the scored set still equals the painted set. `activeTypes`
  // omitted = consider all edges (the cap still applies — it's display-intrinsic, not a toggle).
  const activeLinks = activeTypes
    ? merged.links.filter(l => activeTypes.has(l.type))
    : merged.links
  const links = capEdgesPerNode(
    activeLinks.map(l => ({ source: l.source_id, target: l.target_id, type: l.type, score: l.score, raw: l })),
    EDGE_CAP_BY_TYPE,
  ).links.map(kept => kept.raw)

  // Pass 1: per-type max score (for per-type relevance normalization) + per-node distinct
  // neighbor sets (degree counts each connected artist once, not once per edge type).
  const typeMaxScore = new Map<string, number>()
  const neighborsById = new Map<number, Set<number>>()
  const addNeighbor = (a: number, b: number) => {
    const set = neighborsById.get(a)
    if (set) set.add(b)
    else neighborsById.set(a, new Set([b]))
  }
  for (const link of links) {
    const prevMax = typeMaxScore.get(link.type) ?? 0
    if (link.score > prevMax) typeMaxScore.set(link.type, link.score)
    addNeighbor(link.source_id, link.target_id)
    addNeighbor(link.target_id, link.source_id)
  }

  // Pass 2: per-node strongest per-type-normalized tie (relevance raw input). NB: per-type
  // min/max normalization means the SOLE surviving edge of a type normalizes to 1.0 regardless
  // of its absolute score — a deliberate property of "strongest of its kind", not a bug to
  // "fix" (a lone weak similar tie reads as a full-strength similar tie). Acceptable: relevance
  // is one of three blended terms, and this only bites on sparse / heavily-toggled subgraphs.
  const relevanceRawById = new Map<number, number>()
  for (const link of links) {
    let strength: number
    if (BINARY_EDGE_TYPES.has(link.type)) {
      strength = 1
    } else {
      const typeMax = typeMaxScore.get(link.type) ?? 0
      strength = typeMax > 0 ? link.score / typeMax : 0
    }
    for (const id of [link.source_id, link.target_id]) {
      if (id === centerId) continue
      const prev = relevanceRawById.get(id) ?? 0
      if (strength > prev) relevanceRawById.set(id, strength)
    }
  }

  // The scored set: non-center nodes that still have a DRAWN edge (`links` is already the
  // filtered + capped set above, so this is exactly the painted node set — the cap's no-orphan
  // invariant keeps every still-connected node's strongest edge). Extracted ONCE into an array
  // so the range pass and the scoring pass read the same triple — no duplicated
  // `.get() ?? fallback` to drift between them.
  const scored: Array<{ id: number; deg: number; rel: number; hop: number }> = []
  for (const node of merged.nodes) {
    const neighbors = neighborsById.get(node.id)
    if (!neighbors || neighbors.size === 0) continue
    scored.push({
      id: node.id,
      deg: neighbors.size,
      rel: relevanceRawById.get(node.id) ?? 0,
      hop: merged.hopByNodeId.get(node.id) ?? 1,
    })
  }
  if (scored.length === 0) {
    return { doiByNodeId: new Map(), ranked: [] }
  }

  // Observed ranges for min-max normalization (over the scored nodes only).
  let minDeg = Infinity
  let maxDeg = -Infinity
  let minRel = Infinity
  let maxRel = -Infinity
  let minHop = Infinity
  let maxHop = -Infinity
  for (const s of scored) {
    if (s.deg < minDeg) minDeg = s.deg
    if (s.deg > maxDeg) maxDeg = s.deg
    if (s.rel < minRel) minRel = s.rel
    if (s.rel > maxRel) maxRel = s.rel
    if (s.hop < minHop) minHop = s.hop
    if (s.hop > maxHop) maxHop = s.hop
  }

  const doiByNodeId = new Map<number, number>()
  for (const s of scored) {
    const importance = normalize(s.deg, minDeg, maxDeg)
    const relevance = normalize(s.rel, minRel, maxRel)
    // Nearness: invert hop so the inner ring (minHop) = 1 and the outermost ring = 0.
    const proximity = normalize(maxHop - s.hop, 0, maxHop - minHop)
    doiByNodeId.set(
      s.id,
      weights.importance * importance +
        weights.relevance * relevance +
        weights.proximity * proximity,
    )
  }

  const ranked = [...doiByNodeId.keys()].sort((a, b) => {
    const diff = (doiByNodeId.get(b) ?? 0) - (doiByNodeId.get(a) ?? 0)
    return diff !== 0 ? diff : a - b // deterministic tiebreak
  })

  return { doiByNodeId, ranked }
}

/**
 * Pick the suggested expansion directions: the top `max` DOI-ranked nodes that the user
 * hasn't already expanded (or isn't mid-expanding). Capped at `max` over the WHOLE graph,
 * which is what keeps a freshly-expanded hub from flagging all of its new neighbors at once
 * (PSY-1273 AC: "a hub doesn't flag all its neighbours") — only the few most-interesting
 * survive, regardless of how many a single expansion revealed.
 */
export function selectSuggestedExpansions(
  ranked: ReadonlyArray<number>,
  excluded: ReadonlySet<number>,
  max: number,
): number[] {
  const out: number[] = []
  for (const id of ranked) {
    if (excluded.has(id)) continue
    out.push(id)
    if (out.length >= max) break
  }
  return out
}
