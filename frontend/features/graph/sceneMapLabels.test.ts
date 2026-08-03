import { describe, expect, it } from 'vitest'

import { TOOL_LABEL_TIERS } from '@/components/graph/graphLabels'

import { cullLabelsToGrid, sceneMapLabelTiers } from './sceneMapLabels'

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

describe('cullLabelsToGrid', () => {
  it('keeps only the first candidate in each cell', () => {
    const kept = cullLabelsToGrid(
      [
        { id: 'a', x: 0, y: 0 },
        { id: 'b', x: 5, y: 5 },
        { id: 'c', x: 120, y: 0 },
      ],
      100,
      100,
    )

    expect(kept.map(c => c.id)).toEqual(['a', 'c'])
  })

  it('treats the caller order as the tie-break', () => {
    const first = cullLabelsToGrid(
      [
        { id: 'winner', x: 1, y: 1 },
        { id: 'loser', x: 2, y: 2 },
      ],
      100,
      100,
    )
    const reversed = cullLabelsToGrid(
      [
        { id: 'loser', x: 2, y: 2 },
        { id: 'winner', x: 1, y: 1 },
      ],
      100,
      100,
    )

    expect(first.map(c => c.id)).toEqual(['winner'])
    expect(reversed.map(c => c.id)).toEqual(['loser'])
  })

  it('anchors cells at the origin so negative coordinates get their own cells', () => {
    const kept = cullLabelsToGrid(
      [
        { id: 'left', x: -50, y: 0 },
        { id: 'right', x: 50, y: 0 },
      ],
      100,
      100,
    )

    expect(kept.map(c => c.id)).toEqual(['left', 'right'])
  })

  it('drops candidates with non-finite coordinates', () => {
    const kept = cullLabelsToGrid(
      [
        { id: 'ok', x: 0, y: 0 },
        { id: 'nan', x: Number.NaN, y: 0 },
      ],
      100,
      100,
    )

    expect(kept.map(c => c.id)).toEqual(['ok'])
  })

  it('passes everything through when the cell size is degenerate', () => {
    const candidates = [
      { id: 'a', x: 0, y: 0 },
      { id: 'b', x: 1, y: 1 },
    ]

    expect(cullLabelsToGrid(candidates, 0, 100)).toHaveLength(2)
  })
})
