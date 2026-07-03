import { describe, it, expect } from 'vitest'
import { renderHook, act } from '@testing-library/react'

import { aggregatePairConnections, useConnectionInspect } from './useConnectionInspect'

describe('useConnectionInspect', () => {
  it('opens a pair, re-targets on a second open, and closes', () => {
    const { result } = renderHook(() => useConnectionInspect())
    expect(result.current.pair).toBeNull()

    act(() => result.current.open(1, 2))
    expect(result.current.pair).toEqual({ sourceId: 1, targetId: 2 })

    // One panel at a time — a second open replaces the selection.
    act(() => result.current.open(3, 4))
    expect(result.current.pair).toEqual({ sourceId: 3, targetId: 4 })

    act(() => result.current.close())
    expect(result.current.pair).toBeNull()
  })
})

describe('aggregatePairConnections', () => {
  const links = [
    { source_id: 1, target_id: 2, type: 'shared_label', detail: { label_names: 'Fire Talk' } },
    // Reversed endpoint order must still match the unordered pair.
    { source_id: 2, target_id: 1, type: 'shared_bills', score: 0.3, detail: { shared_count: 3 } },
    // A different pair — never included.
    { source_id: 1, target_id: 3, type: 'shared_bills', score: 0.9 },
    // Untyped links carry no provenance — skipped.
    { source_id: 1, target_id: 2, type: '' },
  ]

  it('collects both directions of the pair, in canonical grammar order', () => {
    const out = aggregatePairConnections(links, { sourceId: 1, targetId: 2 })
    // shared_bills ranks before shared_label in the canonical order even
    // though it appears later in the payload.
    expect(out.map(c => c.type)).toEqual(['shared_bills', 'shared_label'])
    expect(out[0].score).toBe(0.3)
    expect(out[1].detail).toEqual({ label_names: 'Fire Talk' })
  })

  it('dedupes duplicate same-type links (first wins)', () => {
    const dup = [
      { source_id: 1, target_id: 2, type: 'similar', score: 0.8 },
      { source_id: 2, target_id: 1, type: 'similar', score: 0.1 },
    ]
    const out = aggregatePairConnections(dup, { sourceId: 1, targetId: 2 })
    expect(out).toHaveLength(1)
    expect(out[0].score).toBe(0.8)
  })

  it('returns empty for a pair with no typed links', () => {
    expect(aggregatePairConnections(links, { sourceId: 5, targetId: 6 })).toEqual([])
  })
})
