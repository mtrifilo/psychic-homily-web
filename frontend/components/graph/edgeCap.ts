/**
 * Per-node top-k edge cap (PSY-1258)
 *
 * The artist ego graph's hairball is an EDGE-density problem, not a node-count one:
 * radio co-occurrence is many-to-many, so ~30 neighbors can carry hundreds of
 * overlapping edges — which specifically degrades the neighborhood-finding and
 * link-counting tasks discovery depends on (Ghoniem, Fekete & Castagliola 2005; see
 * docs/open-questions/graph-density-discovery-redesign.md §3.3). This trims the dense
 * edge types down to each node's k strongest, the immediate client-side untangle.
 * (The principled server-side disparity-filter backbone is the separate P2 ticket.)
 *
 * Survival rule — an edge of a capped type is kept iff it ranks in the top k by score
 * for EITHER of its endpoints. This mirrors the disparity filter's "locally significant
 * for either node" intent: a niche artist's few-but-strong links survive even when the
 * artist on the other end is a hub with many stronger ties.
 *
 * No-orphan invariant (for k >= 1) — because a node's single strongest incident edge is
 * necessarily top-k for that node, capping never removes every edge of a previously-
 * connected node. Callers that prune edgeless satellites (ArtistGraph does) therefore
 * won't see a node vanish under the cap — only its weaker edges thin out. NB: this holds
 * only for k >= 1; a cap of 0 would drop every edge of that type (and orphan its nodes),
 * so keep `EDGE_CAP_BY_TYPE` values >= 1.
 *
 * Pure + shape-generic (operates on numeric endpoints), so it's unit-tested without the
 * canvas like the other components/graph helpers.
 */

/** Minimum link shape the cap needs. Callers pass richer links (the extra fields ride
 * through `L` unchanged) — e.g. the original payload row keyed under `raw`. */
export interface CappableLink {
  source: number
  target: number
  type: string
  /** Edge strength; higher = kept first. */
  score: number
}

/** Per-type kept-vs-total tally so the legend can disclose the cap (no silent caps). */
export interface EdgeTypeTally {
  shown: number
  total: number
}

export interface EdgeCapResult<L> {
  /** The surviving links, in their original order. */
  links: L[]
  /** type → { shown, total } over the input set, for honest legend counts. */
  counts: Map<string, EdgeTypeTally>
  /** Types where shown < total — i.e. the cap actually dropped edges. */
  cappedTypes: Set<string>
}

/**
 * Which dense relationship types get trimmed to each node's k strongest, and the k.
 * Shared single source of truth (PSY-1273): the canvas (ArtistGraph.graphData) AND the
 * Degree-of-Interest scoring (graphDoi.computeGraphDoi) BOTH cap by this, so the ranking
 * is computed over exactly the edges drawn — add a capped type or change a k in one place.
 *
 * Only radio_cooccurrence is dense enough to need it; k=5 is the upper end of the research's
 * 3–5 range (docs/open-questions/graph-density-discovery-redesign.md §3.3). Tune visually on
 * /artists/cola (the canonical dense reference in prod). Keep values >= 1 — a 0 would drop
 * every edge of the type and orphan its nodes (see the no-orphan invariant above).
 */
export const EDGE_CAP_BY_TYPE: Readonly<Record<string, number>> = { radio_cooccurrence: 5 }

export function capEdgesPerNode<L extends CappableLink>(
  links: readonly L[],
  capByType: Readonly<Record<string, number>>,
): EdgeCapResult<L> {
  const counts = new Map<string, EdgeTypeTally>()
  for (const l of links) {
    const tally = counts.get(l.type)
    if (tally) tally.total += 1
    else counts.set(l.type, { shown: 0, total: 1 })
  }

  // Uncapped-type edges are always kept; capped-type edges start excluded and are
  // promoted back in only if they make either endpoint's top-k by score.
  const keep = links.map(l => capByType[l.type] === undefined)

  // Bucket each capped-type edge index under both of its (endpoint, type) keys. The id is
  // digits-only and the type a space-free slug, so the single space splits them
  // unambiguously and the composite key is collision-free.
  const buckets = new Map<string, { type: string; idxs: number[] }>()
  const bucket = (key: string, type: string, i: number) => {
    const b = buckets.get(key)
    if (b) b.idxs.push(i)
    else buckets.set(key, { type, idxs: [i] })
  }
  links.forEach((l, i) => {
    if (capByType[l.type] === undefined) return
    bucket(`${l.source} ${l.type}`, l.type, i)
    bucket(`${l.target} ${l.type}`, l.type, i)
  })

  for (const { type, idxs } of buckets.values()) {
    const k = capByType[type]
    // Strongest first; break score ties by original index so the output is deterministic.
    idxs.sort((a, b) => links[b].score - links[a].score || a - b)
    for (let j = 0; j < Math.min(k, idxs.length); j++) keep[idxs[j]] = true
  }

  const kept = links.filter((_, i) => keep[i])
  for (const l of kept) {
    const tally = counts.get(l.type)
    if (tally) tally.shown += 1
  }

  const cappedTypes = new Set<string>()
  for (const [type, tally] of counts) {
    if (tally.shown < tally.total) cappedTypes.add(type)
  }

  return { links: kept, counts, cappedTypes }
}
