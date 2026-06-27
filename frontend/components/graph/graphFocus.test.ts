import { describe, expect, it } from 'vitest'

import { buildAdjacency, endpointId, focusForeground } from './graphFocus'

// PSY-1210 — the hover-focus neighborhood math (canvas rendering applies the alpha;
// this is the pure set computation, tested in isolation like graphLabels/edgeGrammar).

describe('endpointId', () => {
  it('returns a bare numeric endpoint as-is', () => {
    expect(endpointId(5)).toBe(5)
  })
  it('reads .id from a resolved node object (post first tick)', () => {
    expect(endpointId({ id: 7 })).toBe(7)
  })
})

describe('buildAdjacency', () => {
  it('builds a bidirectional adjacency map from bare-id links', () => {
    // center(1) — 2, center(1) — 3, and a 2—3 cross-connection.
    const adj = buildAdjacency([
      { source: 1, target: 2 },
      { source: 1, target: 3 },
      { source: 2, target: 3 },
    ])
    expect([...(adj.get(1) ?? [])].sort()).toEqual([2, 3])
    expect([...(adj.get(2) ?? [])].sort()).toEqual([1, 3])
    expect([...(adj.get(3) ?? [])].sort()).toEqual([1, 2])
  })

  it('handles resolved {id} node objects (post first tick) the same as bare ids', () => {
    const adj = buildAdjacency([
      { source: { id: 1 }, target: 2 },
      { source: 3, target: { id: 1 } },
    ])
    expect([...(adj.get(1) ?? [])].sort()).toEqual([2, 3])
    expect([...(adj.get(2) ?? [])]).toEqual([1])
    expect([...(adj.get(3) ?? [])]).toEqual([1])
  })

  it('returns an empty map for no links', () => {
    expect(buildAdjacency([]).size).toBe(0)
  })
})

describe('focusForeground', () => {
  const adj = buildAdjacency([
    { source: 1, target: 2 },
    { source: 1, target: 3 },
    { source: 2, target: 3 },
    { source: 1, target: 4 }, // 4 connects only to the center
  ])

  it('returns null when nothing is hovered (resting view = no focus)', () => {
    expect(focusForeground(adj, null)).toBeNull()
    expect(focusForeground(adj, undefined)).toBeNull()
  })

  it('includes the hovered node + its 1-hop neighbors', () => {
    // Hovering satellite 2: foreground = {2, center 1, cross-connected 3}; 4 fades.
    expect([...(focusForeground(adj, 2) ?? [])].sort()).toEqual([1, 2, 3])
  })

  it('returns the hovered hub + its 1-hop neighbors (a hub adjacent to all)', () => {
    expect([...(focusForeground(adj, 1) ?? [])].sort()).toEqual([1, 2, 3, 4])
  })

  it('adds alwaysInclude even when it is NOT a neighbor of the hovered node', () => {
    // The artist graph passes its center here so the page subject stays foreground
    // even when its direct edge to the hovered satellite is filtered out (PSY-1210):
    // node 4's only neighbor is the center 1, and anchor 99 is unrelated to node 4.
    const fg = focusForeground(adj, 4, 99)
    expect(fg?.has(99)).toBe(true)
    expect([...(fg ?? [])].sort()).toEqual([1, 4, 99])
  })

  it('ignores a null/undefined alwaysInclude', () => {
    expect([...(focusForeground(adj, 4, null) ?? [])].sort()).toEqual([1, 4])
    expect([...(focusForeground(adj, 4) ?? [])].sort()).toEqual([1, 4])
  })

  it('returns a singleton for a hovered node with no edges', () => {
    expect([...(focusForeground(adj, 99) ?? [])]).toEqual([99])
  })

  it('includes the hovered id even when its neighbor set is absent', () => {
    expect(focusForeground(new Map(), 7)?.has(7)).toBe(true)
  })
})
