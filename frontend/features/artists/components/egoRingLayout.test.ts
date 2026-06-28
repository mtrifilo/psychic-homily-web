import { describe, it, expect } from 'vitest'

import {
  pinEgoLayoutPositions,
  EGO_RING_RADIUS,
  RING_GAP,
  type PinnableNode,
} from './egoRingLayout'

// PSY-1275 — the deterministic pinned ring layout behind the crowding fix. The canvas memo
// just calls this on its render nodes; it's the geometry, tested here without the canvas. The
// radii are IMPORTED (not re-declared) so the test can't silently drift from the real layout.

const pinnable = (id: number, isCenter = false): PinnableNode => ({ id, isCenter })
const radius = (n: PinnableNode) => Math.hypot(n.fx ?? NaN, n.fy ?? NaN)

describe('pinEgoLayoutPositions', () => {
  it('pins the center at the origin (both position and force-pin)', () => {
    const center = pinnable(1, true)
    pinEgoLayoutPositions([center], new Map([[1, 0]]), EGO_RING_RADIUS, RING_GAP)
    expect([center.fx, center.fy, center.x, center.y]).toEqual([0, 0, 0, 0])
  })

  it('spreads N hop-1 satellites evenly on the inner ring', () => {
    const nodes = [pinnable(2), pinnable(3), pinnable(4), pinnable(5)]
    const hopByNodeId = new Map([[2, 1], [3, 1], [4, 1], [5, 1]])
    pinEgoLayoutPositions(nodes, hopByNodeId, EGO_RING_RADIUS, RING_GAP)

    // All on the inner ring...
    for (const n of nodes) expect(radius(n)).toBeCloseTo(EGO_RING_RADIUS, 6)
    // ...at even 2π/N angles in array order: first at angle 0, then quarter-turns.
    const expected = nodes.map((_, i) => (2 * Math.PI * i) / nodes.length)
    for (let i = 0; i < nodes.length; i++) {
      expect(Math.atan2(nodes[i].fy!, nodes[i].fx!)).toBeCloseTo(
        Math.atan2(Math.sin(expected[i]), Math.cos(expected[i])),
        6,
      )
    }
    // The first satellite sits at angle 0 → (EGO_RING_RADIUS, 0).
    expect(nodes[0].fx).toBeCloseTo(EGO_RING_RADIUS, 6)
    expect(nodes[0].fy).toBeCloseTo(0, 6)
  })

  it('places a hop-2 node on the outer ring (radius + gap)', () => {
    const hop1 = pinnable(2)
    const hop2 = pinnable(3)
    pinEgoLayoutPositions([hop1, hop2], new Map([[2, 1], [3, 2]]), EGO_RING_RADIUS, RING_GAP)
    expect(radius(hop1)).toBeCloseTo(EGO_RING_RADIUS, 6)
    expect(radius(hop2)).toBeCloseTo(EGO_RING_RADIUS + RING_GAP, 6)
  })

  it('defaults an unknown-hop node to the inner (hop-1) ring', () => {
    const orphan = pinnable(9)
    pinEgoLayoutPositions([orphan], new Map(), EGO_RING_RADIUS, RING_GAP) // not in hopByNodeId
    expect(radius(orphan)).toBeCloseTo(EGO_RING_RADIUS, 6)
  })

  it('re-spreads survivors when a ring shrinks (index-based, not stable across set changes)', () => {
    // Three on the ring → 120° apart. Drop the middle one and re-run on the smaller set:
    // the two survivors must re-spread to 180° apart, not keep their old angles.
    const three = [pinnable(2), pinnable(3), pinnable(4)]
    pinEgoLayoutPositions(three, new Map([[2, 1], [3, 1], [4, 1]]), EGO_RING_RADIUS, RING_GAP)
    const two = [pinnable(2), pinnable(4)]
    pinEgoLayoutPositions(two, new Map([[2, 1], [4, 1]]), EGO_RING_RADIUS, RING_GAP)
    // node 4 was at index 2 of 3 (angle 240°); now index 1 of 2 (angle 180°) → moved.
    expect(Math.atan2(two[1].fy!, two[1].fx!)).toBeCloseTo(Math.PI, 6)
  })

  it('sets x/y in lockstep with the fx/fy pin', () => {
    const nodes = [pinnable(2), pinnable(3)]
    pinEgoLayoutPositions(nodes, new Map([[2, 1], [3, 1]]), EGO_RING_RADIUS, RING_GAP)
    for (const n of nodes) {
      expect(n.x).toBe(n.fx)
      expect(n.y).toBe(n.fy)
    }
  })
})
