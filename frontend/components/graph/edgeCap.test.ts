import { describe, expect, it } from 'vitest'

import { capEdgesPerNode, type CappableLink } from './edgeCap'

// PSY-1258 — the per-node top-k edge cap (the canvas just draws the survivors; this is
// the pure selection, tested in isolation like graphFocus/graphLabels/edgeGrammar).

const link = (source: number, target: number, type: string, score: number): CappableLink => ({
  source,
  target,
  type,
  score,
})

describe('capEdgesPerNode', () => {
  it('leaves uncapped types completely untouched', () => {
    const links = [
      link(1, 2, 'similar', 0.1),
      link(1, 3, 'similar', 0.2),
      link(1, 4, 'similar', 0.3),
    ]
    const { links: kept, cappedTypes } = capEdgesPerNode(links, { radio_cooccurrence: 2 })
    expect(kept).toEqual(links)
    expect(cappedTypes.size).toBe(0)
  })

  it("keeps a capped type's edges down to each node's top-k by score", () => {
    // Hub node 1 has four radio edges; cap k=2 keeps only its two strongest from node 1's
    // perspective — but each satellite's own strongest (which is its single edge to the
    // hub) also survives, so weaker satellites are NOT dropped (either-endpoint rule).
    const links = [
      link(1, 2, 'radio_cooccurrence', 0.9), // strongest for hub 1 → kept
      link(1, 3, 'radio_cooccurrence', 0.8), // 2nd for hub 1 → kept
      link(1, 4, 'radio_cooccurrence', 0.4), // not top-2 for hub 1, but top-1 for node 4 → kept
      link(1, 5, 'radio_cooccurrence', 0.2), // not top-2 for hub 1, but top-1 for node 5 → kept
    ]
    const { links: kept } = capEdgesPerNode(links, { radio_cooccurrence: 2 })
    // All four survive: the two strongest via hub 1, the other two via their satellite endpoint.
    expect(kept).toHaveLength(4)
  })

  it('drops an edge that is top-k for NEITHER endpoint', () => {
    // Two hubs (1 and 2) each already have k=1 stronger edges to other nodes, so the
    // weak 1—2 tie is top-1 for neither and gets cut.
    const links = [
      link(1, 3, 'radio_cooccurrence', 0.9), // top-1 for hub 1 and for node 3
      link(2, 4, 'radio_cooccurrence', 0.8), // top-1 for hub 2 and for node 4
      link(1, 2, 'radio_cooccurrence', 0.1), // top-1 for neither hub → DROPPED
    ]
    const { links: kept, cappedTypes } = capEdgesPerNode(links, { radio_cooccurrence: 1 })
    expect(kept.map(l => [l.source, l.target])).toEqual([
      [1, 3],
      [2, 4],
    ])
    expect(cappedTypes.has('radio_cooccurrence')).toBe(true)
  })

  it('never orphans a previously-connected node (its strongest edge always survives)', () => {
    // A star: hub 1 connected to 2..6, plus a few cross-ties. With k=2 every satellite
    // keeps its single edge to the hub (top-1 for the satellite), so all 5 satellites
    // remain reachable even though the hub itself keeps only its 2 strongest.
    const links = [
      link(1, 2, 'radio_cooccurrence', 0.95),
      link(1, 3, 'radio_cooccurrence', 0.90),
      link(1, 4, 'radio_cooccurrence', 0.50),
      link(1, 5, 'radio_cooccurrence', 0.40),
      link(1, 6, 'radio_cooccurrence', 0.30),
    ]
    const { links: kept } = capEdgesPerNode(links, { radio_cooccurrence: 2 })
    const survivingNodes = new Set(kept.flatMap(l => [l.source, l.target]))
    for (const id of [1, 2, 3, 4, 5, 6]) {
      expect(survivingNodes.has(id)).toBe(true)
    }
  })

  it('reports honest shown/total counts per type and which types were capped', () => {
    // A satellite-to-satellite cross-tie (2—3) that is top-1 for neither of its endpoints
    // (each already has a stronger edge to the hub) gets cut, so radio shown < total.
    const links = [
      link(1, 2, 'radio_cooccurrence', 0.9), // top-1 for hub 1 and node 2 → kept
      link(1, 3, 'radio_cooccurrence', 0.8), // top-1 for node 3 → kept
      link(2, 3, 'radio_cooccurrence', 0.1), // top-1 for neither node 2 nor node 3 → DROPPED
      link(1, 4, 'similar', 0.5), // uncapped type → always kept
    ]
    const { counts, cappedTypes } = capEdgesPerNode(links, { radio_cooccurrence: 1 })
    expect(counts.get('radio_cooccurrence')).toEqual({ shown: 2, total: 3 })
    expect(counts.get('similar')).toEqual({ shown: 1, total: 1 })
    expect(cappedTypes.has('radio_cooccurrence')).toBe(true)
    expect(cappedTypes.has('similar')).toBe(false)
  })

  it('breaks score ties deterministically by original order', () => {
    const links = [
      link(1, 2, 'radio_cooccurrence', 0.5),
      link(1, 3, 'radio_cooccurrence', 0.5),
      link(1, 4, 'radio_cooccurrence', 0.5),
    ]
    // From hub 1's view all three tie at 0.5; k=2 keeps the first two by original index.
    // Each satellite also keeps its own edge though (top-1 for itself) → all survive, so
    // assert the ORDER is stable rather than the count.
    const { links: kept } = capEdgesPerNode(links, { radio_cooccurrence: 2 })
    expect(kept).toEqual(links)
  })

  it('handles an empty link set', () => {
    const { links, counts, cappedTypes } = capEdgesPerNode([], { radio_cooccurrence: 5 })
    expect(links).toEqual([])
    expect(counts.size).toBe(0)
    expect(cappedTypes.size).toBe(0)
  })

  it('passes extra link fields through unchanged (generic over the link shape)', () => {
    const links = [
      { source: 1, target: 2, type: 'radio_cooccurrence', score: 0.9, raw: { id: 'a' } },
      { source: 1, target: 3, type: 'radio_cooccurrence', score: 0.8, raw: { id: 'b' } },
    ]
    const { links: kept } = capEdgesPerNode(links, { radio_cooccurrence: 5 })
    expect(kept.map(l => l.raw.id)).toEqual(['a', 'b'])
  })
})
