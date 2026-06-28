import { describe, it, expect } from 'vitest'

import { mergeEgoGraphs, pinEgoRingPositions, type PinnableNode } from './mergeEgoGraphs'
import type { ArtistGraph, ArtistGraphLink, ArtistGraphNode } from '../types'

// PSY-1259 — the pure merge + hop-assignment behind expand-on-demand. The canvas just
// draws the result; this is the data model, tested in isolation like the other graph helpers.

const node = (id: number): ArtistGraphNode => ({
  id,
  name: `a${id}`,
  slug: `a${id}`,
  upcoming_show_count: 0,
})

const link = (s: number, t: number, type = 'similar', score = 0.5): ArtistGraphLink => ({
  source_id: s,
  target_id: t,
  type,
  score,
  votes_up: 0,
  votes_down: 0,
})

// An ego payload: `centerId`'s graph with the given neighbor ids and links.
const ego = (centerId: number, neighborIds: number[], links: ArtistGraphLink[]): ArtistGraph => ({
  center: node(centerId),
  nodes: neighborIds.map(node),
  links,
  user_votes: {},
})

const noExpansions = new Map<number, ArtistGraph>()

describe('mergeEgoGraphs', () => {
  it('with no expansions, returns the base ego graph with all satellites at hop 1', () => {
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3), link(2, 3)])
    const merged = mergeEgoGraphs(base, noExpansions)

    expect(merged.center.id).toBe(1)
    expect(merged.nodes.map(n => n.id).sort()).toEqual([2, 3]) // center excluded from nodes
    expect(merged.links).toHaveLength(3)
    expect(merged.hopByNodeId.get(1)).toBe(0) // center hop 0 (kept in the map)
    expect(merged.hopByNodeId.get(2)).toBe(1)
    expect(merged.hopByNodeId.get(3)).toBe(1)
  })

  it('places an expanded node\'s new neighbors on the next ring (hop 2)', () => {
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3)])
    const exp2 = ego(2, [4, 5], [link(2, 4), link(2, 5)]) // expand node 2
    const merged = mergeEgoGraphs(base, new Map([[2, exp2]]))

    expect(merged.nodes.map(n => n.id).sort()).toEqual([2, 3, 4, 5])
    expect(merged.hopByNodeId.get(4)).toBe(2)
    expect(merged.hopByNodeId.get(5)).toBe(2)
    expect(merged.links).toHaveLength(4) // 1-2, 1-3, 2-4, 2-5
  })

  it('dedupes a link present in both the base and an expansion', () => {
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3), link(2, 3)])
    const exp2 = ego(2, [3, 4], [link(2, 3), link(2, 4)]) // 2-3 also returned here
    const merged = mergeEgoGraphs(base, new Map([[2, exp2]]))

    const key = (l: ArtistGraphLink) => `${l.source_id}|${l.target_id}|${l.type}`
    const keys = merged.links.map(key)
    expect(new Set(keys).size).toBe(keys.length) // no duplicates
    expect(keys).toContain('2|3|similar')
    expect(merged.links).toHaveLength(4) // 1-2, 1-3, 2-3, 2-4
  })

  it('dedupes a REVERSED-direction edge (query-time festival_cobill ordering)', () => {
    // festival_cobill is a query-time edge the backend orders with the CURRENT center on the
    // source side, so the same X–Y festival edge arrives as (1,2) from X's ego and (2,1) from
    // Y's ego. The canonical min/max key must collapse them to ONE edge, not draw a duplicate.
    const base = ego(1, [2], [link(1, 2, 'festival_cobill')])
    const exp2 = ego(2, [1], [link(2, 1, 'festival_cobill')]) // reversed pair, same physical edge
    const merged = mergeEgoGraphs(base, new Map([[2, exp2]]))
    expect(merged.links.filter(l => l.type === 'festival_cobill')).toHaveLength(1)
  })

  it('drops a link to a dangling endpoint that has no node row in any payload', () => {
    // A malformed expansion: a link references id 99 but no payload supplies a node for it.
    // The link must not survive (it would spawn a phantom node in react-force-graph).
    const base = ego(1, [2], [link(1, 2)])
    const exp2: ArtistGraph = { center: node(2), nodes: [], links: [link(2, 99)], user_votes: {} }
    const merged = mergeEgoGraphs(base, new Map([[2, exp2]]))
    expect(merged.nodes.map(n => n.id)).toEqual([2])
    expect(merged.links.some(l => l.source_id === 99 || l.target_id === 99)).toBe(false)
  })

  it('assigns the MINIMUM hop when a node is reachable by two paths', () => {
    // Node 3 is a direct base neighbor (hop 1) AND reappears as node 2's neighbor (hop 2 path).
    const base = ego(1, [2, 3], [link(1, 2), link(1, 3), link(2, 3)])
    const exp2 = ego(2, [3], [link(2, 3)])
    const merged = mergeEgoGraphs(base, new Map([[2, exp2]]))

    expect(merged.hopByNodeId.get(3)).toBe(1) // stays on the inner ring, not pushed to hop 2
  })

  it('prunes collapse-orphans: removing an expansion drops nodes only reachable through it', () => {
    const base = ego(1, [2], [link(1, 2)])
    const exp2 = ego(2, [3], [link(2, 3)]) // node 3 only reachable via expanding 2

    const expanded = mergeEgoGraphs(base, new Map([[2, exp2]]))
    expect(expanded.nodes.map(n => n.id).sort()).toEqual([2, 3])

    // Collapse node 2 (drop its expansion) → node 3 has no path from center → gone.
    const collapsed = mergeEgoGraphs(base, noExpansions)
    expect(collapsed.nodes.map(n => n.id)).toEqual([2])
    expect(collapsed.hopByNodeId.has(3)).toBe(false)
  })

  it('drops links whose endpoint was pruned', () => {
    // A link 3-4 from an expansion of 2; if 2 is collapsed, 3 and 4 are unreachable and the
    // link must not survive into the merged set.
    const base = ego(1, [2], [link(1, 2)])
    const exp2 = ego(2, [3, 4], [link(2, 3), link(3, 4)])
    const collapsed = mergeEgoGraphs(base, noExpansions)
    expect(collapsed.links.map(l => `${l.source_id}-${l.target_id}`)).toEqual(['1-2'])
    // sanity: with the expansion present, 3-4 survives
    const expanded = mergeEgoGraphs(base, new Map([[2, exp2]]))
    expect(expanded.links.some(l => l.source_id === 3 && l.target_id === 4)).toBe(true)
  })

  it('handles a center with no neighbors', () => {
    const base = ego(1, [], [])
    const merged = mergeEgoGraphs(base, noExpansions)
    expect(merged.nodes).toEqual([])
    expect(merged.links).toEqual([])
    expect(merged.hopByNodeId.get(1)).toBe(0)
    expect(merged.hopByNodeId.size).toBe(1)
  })

  it('supports multi-hop chains (hop 3 via two expansions)', () => {
    const base = ego(1, [2], [link(1, 2)])
    const exp2 = ego(2, [3], [link(2, 3)])
    const exp3 = ego(3, [4], [link(3, 4)])
    const merged = mergeEgoGraphs(base, new Map([[2, exp2], [3, exp3]]))
    expect(merged.hopByNodeId.get(3)).toBe(2)
    expect(merged.hopByNodeId.get(4)).toBe(3)
  })
})

// PSY-1275 — the deterministic pinned ring layout behind the crowding fix. The canvas memo
// just calls this on its render nodes; it's the geometry, tested here without the canvas.
// Constants mirror ArtistGraph.tsx's EGO_RING_RADIUS / RING_GAP.
const RING_RADIUS = 130
const RING_GAP = 120

const pinnable = (id: number, isCenter = false): PinnableNode => ({ id, isCenter })
const radius = (n: PinnableNode) => Math.hypot(n.fx ?? NaN, n.fy ?? NaN)

describe('pinEgoRingPositions', () => {
  it('pins the center at the origin (both position and force-pin)', () => {
    const center = pinnable(1, true)
    pinEgoRingPositions([center], new Map([[1, 0]]), RING_RADIUS, RING_GAP)
    expect([center.fx, center.fy, center.x, center.y]).toEqual([0, 0, 0, 0])
  })

  it('spreads N hop-1 satellites evenly on the inner ring', () => {
    const nodes = [pinnable(2), pinnable(3), pinnable(4), pinnable(5)]
    const hopByNodeId = new Map([[2, 1], [3, 1], [4, 1], [5, 1]])
    pinEgoRingPositions(nodes, hopByNodeId, RING_RADIUS, RING_GAP)

    // All on the inner ring (radius 130)...
    for (const n of nodes) expect(radius(n)).toBeCloseTo(RING_RADIUS, 6)
    // ...at even 2π/N angles in array order: first at angle 0, then quarter-turns.
    const expected = nodes.map((_, i) => (2 * Math.PI * i) / nodes.length)
    for (let i = 0; i < nodes.length; i++) {
      expect(Math.atan2(nodes[i].fy!, nodes[i].fx!)).toBeCloseTo(Math.atan2(Math.sin(expected[i]), Math.cos(expected[i])), 6)
    }
    // The first satellite sits at angle 0 → (130, 0).
    expect(nodes[0].fx).toBeCloseTo(RING_RADIUS, 6)
    expect(nodes[0].fy).toBeCloseTo(0, 6)
  })

  it('places a hop-2 node on the outer ring (radius + gap)', () => {
    const hop1 = pinnable(2)
    const hop2 = pinnable(3)
    pinEgoRingPositions([hop1, hop2], new Map([[2, 1], [3, 2]]), RING_RADIUS, RING_GAP)
    expect(radius(hop1)).toBeCloseTo(RING_RADIUS, 6)
    expect(radius(hop2)).toBeCloseTo(RING_RADIUS + RING_GAP, 6) // 250
  })

  it('defaults an unknown-hop node to the inner (hop-1) ring', () => {
    const orphan = pinnable(9)
    pinEgoRingPositions([orphan], new Map(), RING_RADIUS, RING_GAP) // not in hopByNodeId
    expect(radius(orphan)).toBeCloseTo(RING_RADIUS, 6)
  })

  it('sets x/y in lockstep with the fx/fy pin', () => {
    const nodes = [pinnable(2), pinnable(3)]
    pinEgoRingPositions(nodes, new Map([[2, 1], [3, 1]]), RING_RADIUS, RING_GAP)
    for (const n of nodes) {
      expect(n.x).toBe(n.fx)
      expect(n.y).toBe(n.fy)
    }
  })
})
