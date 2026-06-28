import { describe, it, expect } from 'vitest'

import { computeGraphDoi, selectSuggestedExpansions, doiWeightsForBias, DOI_WEIGHTS } from './graphDoi'
import { mergeEgoGraphs } from './mergeEgoGraphs'
import { capEdgesPerNode, EDGE_CAP_BY_TYPE } from '@/components/graph/edgeCap'
import type { ArtistGraph, ArtistGraphLink, ArtistGraphNode } from '../types'

// PSY-1273 — the pure DOI ranking behind label priority + suggested expansion directions.
// Inputs are built through mergeEgoGraphs (the real merge + hop assignment) so the scoring
// is exercised against realistic subgraphs, mirroring mergeEgoGraphs.test.ts.

const node = (id: number): ArtistGraphNode => ({ id, name: `a${id}`, slug: `a${id}`, upcoming_show_count: 0 })

const link = (s: number, t: number, type = 'similar', score = 0.5): ArtistGraphLink => ({
  source_id: s,
  target_id: t,
  type,
  score,
  votes_up: 0,
  votes_down: 0,
})

const ego = (centerId: number, neighborIds: number[], links: ArtistGraphLink[]): ArtistGraph => ({
  center: node(centerId),
  nodes: neighborIds.map(node),
  links,
  user_votes: {},
})

const noExpansions = new Map<number, ArtistGraph>()

describe('computeGraphDoi', () => {
  it('returns empty scores for a center with no neighbors', () => {
    const merged = mergeEgoGraphs(ego(1, [], []), noExpansions)
    const { doiByNodeId, ranked } = computeGraphDoi(merged)
    expect(doiByNodeId.size).toBe(0)
    expect(ranked).toEqual([])
  })

  it('never scores the center (it is the anchor, not a suggestion)', () => {
    const merged = mergeEgoGraphs(ego(1, [2, 3], [link(1, 2), link(1, 3)]), noExpansions)
    const { doiByNodeId } = computeGraphDoi(merged)
    expect(doiByNodeId.has(1)).toBe(false)
    expect(doiByNodeId.has(2)).toBe(true)
    expect(doiByNodeId.has(3)).toBe(true)
  })

  it('importance: a node with more DISTINCT neighbors outranks a leaf (relevance + proximity equal)', () => {
    // 2 & 3 cross-connect (2 distinct neighbors each); 4 only touches the center (1). All hop-1,
    // all similar@0.5 so relevance + proximity are constant across them — only degree differs.
    const merged = mergeEgoGraphs(
      ego(1, [2, 3, 4], [link(1, 2), link(1, 3), link(1, 4), link(2, 3)]),
      noExpansions,
    )
    const { doiByNodeId, ranked } = computeGraphDoi(merged)
    expect(doiByNodeId.get(2)!).toBeGreaterThan(doiByNodeId.get(4)!)
    expect(doiByNodeId.get(3)!).toBeGreaterThan(doiByNodeId.get(4)!)
    expect(doiByNodeId.get(2)!).toBeCloseTo(doiByNodeId.get(3)!)
    expect(ranked[ranked.length - 1]).toBe(4) // the leaf ranks last
  })

  it('importance: distinct-neighbor count — a multi-edge-type pair counts the neighbor ONCE', () => {
    // node 2 is tied to the center by BOTH similar AND radio (two edges, ONE neighbor); nodes 3 & 4
    // each reach TWO distinct neighbors (center + each other). With edge-COUNT degree all three
    // would tie at 2 and importance couldn't discriminate; with distinct-NEIGHBOR degree node 2
    // (1 neighbor) ranks below the genuinely-better-connected 3 & 4 (2 neighbors).
    const merged = mergeEgoGraphs(
      ego(
        1,
        [2, 3, 4],
        [
          link(1, 2, 'similar', 0.5),
          link(1, 2, 'radio_cooccurrence', 0.5),
          link(1, 3, 'similar', 0.5),
          link(1, 4, 'similar', 0.5),
          link(3, 4, 'similar', 0.5),
        ],
      ),
      noExpansions,
    )
    const { doiByNodeId } = computeGraphDoi(merged)
    expect(doiByNodeId.get(3)!).toBeGreaterThan(doiByNodeId.get(2)!)
    expect(doiByNodeId.get(4)!).toBeGreaterThan(doiByNodeId.get(2)!)
  })

  it('relevance: per-type normalization keeps a high-count radio tie from outranking a strong similar tie', () => {
    // A naive max-incident-SCORE relevance would rank node 3 (radio score 50) far above node 2
    // (similar score 1.0) purely by scale. Per-type normalization puts a type-leading similar
    // tie and a type-leading radio tie on equal footing — both are "the strongest of their kind".
    const merged = mergeEgoGraphs(
      ego(
        1,
        [2, 3, 4],
        [
          link(1, 2, 'similar', 1.0), // type-leading similar
          link(1, 3, 'radio_cooccurrence', 50), // type-leading radio (huge raw score)
          link(1, 4, 'similar', 0.2), // weak similar
        ],
      ),
      noExpansions,
    )
    const { doiByNodeId } = computeGraphDoi(merged)
    // The strong similar and strong radio nodes tie — scale did NOT inflate the radio node.
    expect(doiByNodeId.get(2)!).toBeCloseTo(doiByNodeId.get(3)!)
    // The weak similar tie ranks below both.
    expect(doiByNodeId.get(2)!).toBeGreaterThan(doiByNodeId.get(4)!)
    expect(doiByNodeId.get(3)!).toBeGreaterThan(doiByNodeId.get(4)!)
  })

  it('relevance: a zero-magnitude MAGNITUDE edge (e.g. unvoted similar) is weak, not full-strength', () => {
    // node 2 has a similar tie with score 0 (e.g. unvoted); node 3 a similar tie with real weight.
    // The fix: only genuinely-binary types are full-strength at score 0 — a magnitude type scored
    // 0 means a weak tie, so node 2 must NOT tie node 3.
    const merged = mergeEgoGraphs(
      ego(1, [2, 3], [link(1, 2, 'similar', 0), link(1, 3, 'similar', 0.8)]),
      noExpansions,
    )
    const { doiByNodeId } = computeGraphDoi(merged)
    expect(doiByNodeId.get(3)!).toBeGreaterThan(doiByNodeId.get(2)!)
    // node 2's relevance term is 0 (degree + proximity are degenerate → full), proving it wasn't
    // elevated to a maximal tie by the all-zero-type shortcut.
    expect(doiByNodeId.get(2)!).toBeCloseTo(DOI_WEIGHTS.importance + DOI_WEIGHTS.proximity)
  })

  it('treats a BINARY (member_of / side_project) edge as a full-strength tie', () => {
    // member_of carries no magnitude (uniform stroke); a present binary edge is a definite
    // relationship, so a node tied only by one should be scored full-strength, not relevance-0.
    const merged = mergeEgoGraphs(ego(1, [2], [link(1, 2, 'member_of', 0)]), noExpansions)
    const { doiByNodeId } = computeGraphDoi(merged)
    expect(doiByNodeId.get(2)!).toBeCloseTo(
      DOI_WEIGHTS.importance + DOI_WEIGHTS.relevance + DOI_WEIGHTS.proximity,
    )
  })

  it('proximity: a nearer node outranks a deeper one when importance + relevance are equal', () => {
    // node 2 is hop-1, node 4 is hop-2 (revealed by expanding node 3). Both have 1 distinct
    // neighbor and a type-leading similar tie, so importance + relevance are equal — only hop differs.
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3)])
    const exp3 = ego(3, [4], [link(3, 4)])
    const merged = mergeEgoGraphs(base, new Map([[3, exp3]]))
    expect(merged.hopByNodeId.get(2)).toBe(1)
    expect(merged.hopByNodeId.get(4)).toBe(2)

    const { doiByNodeId } = computeGraphDoi(merged)
    expect(doiByNodeId.get(2)!).toBeGreaterThan(doiByNodeId.get(4)!)
  })

  it('scopes scoring to activeTypes — a node whose only tie is toggled off is not scored', () => {
    // node 2 reachable via `similar`, node 3 only via `radio_cooccurrence`. With radio toggled
    // off, node 3 has no on-screen edge → it isn't painted → it must not be scored or suggested.
    const merged = mergeEgoGraphs(
      ego(1, [2, 3], [link(1, 2, 'similar', 0.5), link(1, 3, 'radio_cooccurrence', 0.9)]),
      noExpansions,
    )
    const all = computeGraphDoi(merged)
    expect(all.ranked.sort((a, b) => a - b)).toEqual([2, 3]) // both scored with no filter

    const similarOnly = computeGraphDoi(merged, new Set(['similar']))
    expect(similarOnly.ranked).toEqual([2])
    expect(similarOnly.doiByNodeId.has(3)).toBe(false)
  })

  it('scores over the per-node edge cap — a capped-away weak tie does not affect DOI', () => {
    // The canvas trims dense radio edges to each node's top-k (k=5). DOI must score the SAME
    // capped graph, else a node could glow / win a label on ties that aren't drawn. Construct a
    // weak radio edge 2–7 that the cap drops from BOTH endpoints (each already has 5 stronger
    // radio edges), and assert it changes no DOI score vs a graph that never had it.
    const strong = [
      link(1, 2, 'radio_cooccurrence', 0.99), link(2, 3, 'radio_cooccurrence', 0.98),
      link(2, 4, 'radio_cooccurrence', 0.97), link(2, 5, 'radio_cooccurrence', 0.96),
      link(2, 6, 'radio_cooccurrence', 0.95),
      link(1, 7, 'radio_cooccurrence', 0.99), link(3, 7, 'radio_cooccurrence', 0.94),
      link(4, 7, 'radio_cooccurrence', 0.93), link(5, 7, 'radio_cooccurrence', 0.92),
      link(6, 7, 'radio_cooccurrence', 0.91),
    ]
    const ids = [2, 3, 4, 5, 6, 7]
    const without = mergeEgoGraphs(ego(1, ids, strong), noExpansions)
    const withWeak = mergeEgoGraphs(
      ego(1, ids, [...strong, link(2, 7, 'radio_cooccurrence', 0.01)]),
      noExpansions,
    )

    // Premise: the weak 2–7 tie really is dropped by the cap (else this proves nothing).
    const capped = capEdgesPerNode(
      withWeak.links.map(l => ({ source: l.source_id, target: l.target_id, type: l.type, score: l.score })),
      EDGE_CAP_BY_TYPE,
    ).links
    expect(capped.some(l => l.source === 2 && l.target === 7)).toBe(false)

    // The dropped tie sways no node's DOI — scoring matches the graph that never had it.
    const a = computeGraphDoi(without).doiByNodeId
    const b = computeGraphDoi(withWeak).doiByNodeId
    for (const id of ids) expect(b.get(id)!).toBeCloseTo(a.get(id)!)
  })

  it('ranks by DOI desc with a deterministic id tiebreak', () => {
    const merged = mergeEgoGraphs(ego(1, [4, 2, 3], [link(1, 4), link(1, 2), link(1, 3)]), noExpansions)
    const { ranked } = computeGraphDoi(merged)
    // All three are symmetric (hop-1, 1-neighbor, type-leading) → tie → sorted by id asc.
    expect(ranked).toEqual([2, 3, 4])
  })
})

describe('selectSuggestedExpansions', () => {
  it('returns the top `max` ranked nodes, preserving rank order', () => {
    expect(selectSuggestedExpansions([2, 3, 4, 5, 6, 7], new Set(), 3)).toEqual([2, 3, 4])
  })

  it('skips excluded (already-expanded / expanding) nodes', () => {
    expect(selectSuggestedExpansions([2, 3, 4, 5, 6], new Set([3, 5]), 3)).toEqual([2, 4, 6])
  })

  it('caps at `max` so a hub does not flag all its neighbours', () => {
    const ranked = Array.from({ length: 30 }, (_, i) => i + 2) // 30 candidates
    expect(selectSuggestedExpansions(ranked, new Set(), 5)).toHaveLength(5)
  })

  it('returns fewer than `max` when not enough candidates remain', () => {
    expect(selectSuggestedExpansions([2, 3], new Set([2]), 5)).toEqual([3])
  })
})

// PSY-1260 — the discovery-bias slider supplies these weights to computeGraphDoi.
describe('doiWeightsForBias', () => {
  it('bias 0 (Popular) = the default weights — unchanged from PSY-1273', () => {
    expect(doiWeightsForBias(0)).toEqual({
      importance: DOI_WEIGHTS.importance,
      relevance: DOI_WEIGHTS.relevance,
      proximity: DOI_WEIGHTS.proximity,
    })
  })

  it('moves ONLY the importance weight: 0 → +imp, 0.5 → 0, 1 → −imp', () => {
    expect(doiWeightsForBias(0).importance).toBeCloseTo(DOI_WEIGHTS.importance)
    expect(doiWeightsForBias(0.5).importance).toBeCloseTo(0)
    expect(doiWeightsForBias(1).importance).toBeCloseTo(-DOI_WEIGHTS.importance)
    // relevance + proximity never move
    for (const t of [0, 0.5, 1]) {
      expect(doiWeightsForBias(t).relevance).toBe(DOI_WEIGHTS.relevance)
      expect(doiWeightsForBias(t).proximity).toBe(DOI_WEIGHTS.proximity)
    }
  })

  it('clamps out-of-range bias', () => {
    expect(doiWeightsForBias(-1).importance).toBeCloseTo(DOI_WEIGHTS.importance) // clamps to 0
    expect(doiWeightsForBias(2).importance).toBeCloseTo(-DOI_WEIGHTS.importance) // clamps to 1
  })
})

describe('computeGraphDoi — diversity bias re-ranks hub vs niche (PSY-1260)', () => {
  // Node 2 is a hub (center + nodes 4,5 → degree 3); node 3 is a leaf (center only → degree 1).
  // Both reach the center by an equal-strength `similar` tie, so relevance + proximity are equal —
  // only degree (importance) separates them. The bias flips which one the importance term favors.
  const merged = mergeEgoGraphs(
    ego(1, [2, 3, 4, 5], [link(1, 2), link(1, 3), link(2, 4), link(2, 5)]),
    noExpansions,
  )

  it('Popular (bias 0): the hub outranks the leaf', () => {
    const { doiByNodeId } = computeGraphDoi(merged, undefined, doiWeightsForBias(0))
    expect(doiByNodeId.get(2)!).toBeGreaterThan(doiByNodeId.get(3)!)
  })

  it('Niche (bias 1): the leaf outranks the hub (importance term inverted)', () => {
    const { doiByNodeId } = computeGraphDoi(merged, undefined, doiWeightsForBias(1))
    expect(doiByNodeId.get(3)!).toBeGreaterThan(doiByNodeId.get(2)!)
  })
})
