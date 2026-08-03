import { describe, expect, it } from 'vitest'

import { TOOL_LABEL_TIERS } from '@/components/graph/graphLabels'

import {
  MAX_SCENE_MAP_FORCED_LABELS,
  MAX_SCENE_MAP_LABELS,
  selectSceneMapLabels,
  sceneMapLabelTiers,
  type SceneMapLabelCandidate,
} from './sceneMapLabels'

describe('sceneMapLabelTiers', () => {
  it('gives the top tier to the most central node, not the least', () => {
    // rank 0 = most central. A direct labelTierStyles call would invert this.
    const nodes = [
      { id: 1, rank: 0 },
      { id: 2, rank: 1 },
      { id: 3, rank: 2 },
      { id: 4, rank: 3 },
      { id: 5, rank: 4 },
      { id: 6, rank: 5 },
    ]

    const tiers = sceneMapLabelTiers(nodes, TOOL_LABEL_TIERS)

    expect(tiers.get(1)).toEqual(TOOL_LABEL_TIERS[0])
    expect(tiers.get(6)).toEqual(TOOL_LABEL_TIERS[2])
  })

  it('does not floor rank 0 to the bottom tier', () => {
    // The shared floor exists for zero-DEGREE isolates; here score 0 is the
    // single most central artist on the map and must keep the top tier.
    const nodes = [
      { id: 10, rank: 0 },
      { id: 11, rank: 5 },
      { id: 12, rank: 9 },
    ]

    expect(sceneMapLabelTiers(nodes, TOOL_LABEL_TIERS).get(10)).toEqual(TOOL_LABEL_TIERS[0])
  })

  it('assigns equal ranks the same tier regardless of input order', () => {
    const ascending = sceneMapLabelTiers(
      [
        { id: 1, rank: 0 },
        { id: 2, rank: 3 },
        { id: 3, rank: 3 },
        { id: 4, rank: 3 },
        { id: 5, rank: 9 },
        { id: 6, rank: 12 },
      ],
      TOOL_LABEL_TIERS,
    )
    const shuffled = sceneMapLabelTiers(
      [
        { id: 6, rank: 12 },
        { id: 3, rank: 3 },
        { id: 1, rank: 0 },
        { id: 5, rank: 9 },
        { id: 4, rank: 3 },
        { id: 2, rank: 3 },
      ],
      TOOL_LABEL_TIERS,
    )

    expect(ascending.get(2)).toEqual(ascending.get(3))
    expect(ascending.get(3)).toEqual(ascending.get(4))
    for (const id of [1, 2, 3, 4, 5, 6]) {
      expect(shuffled.get(id)).toEqual(ascending.get(id))
    }
  })

  it('returns no styles for an empty map', () => {
    expect(sceneMapLabelTiers([], TOOL_LABEL_TIERS).size).toBe(0)
  })
})

// ── Per-frame label selection ─────────────────────────────────────────────

function candidate(
  overrides: Partial<SceneMapLabelCandidate> & Pick<SceneMapLabelCandidate, 'id'>,
): SceneMapLabelCandidate {
  return {
    x: 0,
    y: 0,
    radius: 5,
    text: `Node ${overrides.id}`,
    fontSize: 12,
    fontWeight: 400,
    priority: 0,
    force: false,
    ...overrides,
  }
}

/** A viewport big enough that nothing is culled by it. */
const WIDE_OPEN = { minX: -1e6, minY: -1e6, maxX: 1e6, maxY: 1e6 }

describe('selectSceneMapLabels', () => {
  it('keeps one label per grid cell, in the caller\'s priority order', () => {
    const specs = selectSceneMapLabels(
      [
        candidate({ id: 1, x: 0, y: 0 }),
        // Same cell as #1 at scale 1 (cells are 120x34 screen px).
        candidate({ id: 2, x: 5, y: 5 }),
        candidate({ id: 3, x: 400, y: 0 }),
      ],
      [],
      1,
      null,
      WIDE_OPEN,
    )

    expect(specs.map(spec => spec.text)).toEqual(['Node 1', 'Node 3'])
  })

  it('draws forced labels regardless of the grid', () => {
    const specs = selectSceneMapLabels(
      [candidate({ id: 1, x: 0, y: 0 })],
      [candidate({ id: 2, x: 2, y: 2, force: true })],
      1,
      null,
      WIDE_OPEN,
    )

    expect(specs.map(spec => spec.text).sort()).toEqual(['Node 1', 'Node 2'])
  })

  it('drops everything outside the viewport, forced labels included', () => {
    const specs = selectSceneMapLabels(
      [candidate({ id: 1, x: 5000, y: 0 })],
      [candidate({ id: 2, x: 5000, y: 100, force: true })],
      1,
      null,
      { minX: -100, minY: -100, maxX: 100, maxY: 100 },
    )

    expect(specs).toEqual([])
  })

  it('spends the label budget on the viewport, not on the whole map', () => {
    // Off-screen candidates rank ABOVE the on-screen one. Without the viewport
    // bound they would take the budget and the visible artist would go
    // unlabelled — the failure mode the bound exists to prevent.
    const offscreen = Array.from({ length: MAX_SCENE_MAP_LABELS }, (_, i) =>
      candidate({ id: i + 1, x: 100_000 + i * 500, y: 0, priority: 1000 }),
    )
    const specs = selectSceneMapLabels(
      [...offscreen, candidate({ id: 9999, x: 0, y: 0, priority: -1 })],
      [],
      1,
      null,
      { minX: -100, minY: -100, maxX: 100, maxY: 100 },
    )

    expect(specs.map(spec => spec.text)).toEqual(['Node 9999'])
  })

  it('caps how many labels one frame can draw', () => {
    const many = Array.from({ length: MAX_SCENE_MAP_LABELS + 50 }, (_, i) =>
      candidate({ id: i + 1, x: i * 500, y: i * 500 }),
    )

    const specs = selectSceneMapLabels(many, [], 1, null, WIDE_OPEN)

    expect(specs).toHaveLength(MAX_SCENE_MAP_LABELS)
  })

  it('narrows NODE labels to the focused neighbourhood while one is hovered', () => {
    const specs = selectSceneMapLabels(
      [candidate({ id: 1, x: 0, y: 0 }), candidate({ id: 2, x: 500, y: 500 })],
      [],
      1,
      new Set([1]),
      WIDE_OPEN,
    )

    expect(specs.map(spec => spec.text)).toEqual(['Node 1'])
  })

  it('keeps region names through a hover, dimmed rather than deleted', () => {
    // A region is not a node: its synthetic id can never be in the focused
    // set, so filtering forced candidates by focus would delete the map's
    // whole orientation layer the instant a cursor touched any dot.
    const region = candidate({ id: -1, x: 900, y: 900, force: true, alpha: 0.75 })

    const atRest = selectSceneMapLabels([], [region], 1, null, WIDE_OPEN)
    const focused = selectSceneMapLabels([], [region], 1, new Set([42]), WIDE_OPEN)

    expect(atRest[0].alpha).toBe(0.75)
    expect(focused.map(spec => spec.text)).toEqual(['Node -1'])
    expect(focused[0].alpha).toBeLessThan(0.75)
    expect(focused[0].alpha).toBeGreaterThan(0)
  })

  it('counter-scales the font and the gap so labels hold their screen size', () => {
    const [spec] = selectSceneMapLabels(
      [candidate({ id: 1, fontSize: 12, radius: 10 })],
      [],
      2,
      null,
      WIDE_OPEN,
    )

    expect(spec.fontSize).toBe(6)
    // radius (graph units, unscaled) + the 3px gap at zoom 2.
    expect(spec.y).toBeCloseTo(10 + 1.5, 6)
  })

  it('carries a caption\'s own face and ink through to the shared pass', () => {
    const [spec] = selectSceneMapLabels(
      [],
      [candidate({ id: -1, force: true, fontFamily: 'MonoFace', alpha: 0.75 })],
      1,
      null,
      WIDE_OPEN,
    )

    expect(spec.fontFamily).toBe('MonoFace')
    expect(spec.alpha).toBe(0.75)
  })

  it('drops candidates with non-finite coordinates', () => {
    const specs = selectSceneMapLabels(
      [candidate({ id: 1, x: Number.NaN, y: 0 })],
      [],
      1,
      null,
      WIDE_OPEN,
    )

    expect(specs).toEqual([])
  })

  it('still selects when the canvas cannot report a viewport', () => {
    const specs = selectSceneMapLabels([candidate({ id: 1 })], [], 1, null, null)

    expect(specs.map(spec => spec.text)).toEqual(['Node 1'])
  })

  it('caps forced labels too, so their cost tracks the screen not the catalog', () => {
    // Region captions and hubs skip the grid. If they also skipped a ceiling,
    // per-frame cost would grow with the catalog forever — and every one of
    // them joins the set each other label is collision-tested against.
    const manyForced = Array.from({ length: MAX_SCENE_MAP_FORCED_LABELS + 60 }, (_, i) =>
      candidate({ id: i + 1, x: i, y: 0, force: true, priority: -i }),
    )

    const specs = selectSceneMapLabels([], manyForced, 1, null, WIDE_OPEN)

    expect(specs).toHaveLength(MAX_SCENE_MAP_FORCED_LABELS)
    // The ceiling truncates the TAIL — the highest-priority forced labels
    // (region names, then the most central hubs) are the ones kept.
    expect(specs[0].text).toBe('Node 1')
  })
})
