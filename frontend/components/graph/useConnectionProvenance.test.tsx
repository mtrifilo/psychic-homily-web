import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

vi.mock('@/lib/api-base', () => ({
  API_BASE_URL: 'http://localhost:8080',
}))

import {
  useConnectionProvenance,
  mergeProvenanceEntities,
  connectionProvenanceQueryKey,
  type RelationshipProvenance,
} from './useConnectionProvenance'
import type { EdgeTooltipLink } from './edgeGrammar'

const provenance: RelationshipProvenance = {
  connections: [
    {
      type: 'shared_bills',
      score: 0.8,
      entities: [
        { kind: 'show', id: 7, slug: 'dehd-empty-bottle', name: 'Empty Bottle', date: '2026-05-14' },
      ],
      entity_total: 12,
    },
    { type: 'similar', score: 0.9 },
  ],
}

describe('useConnectionProvenance', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('fetches provenance for the canonical (sorted) pair', async () => {
    mockApiRequest.mockResolvedValue(provenance)

    const { result } = renderHook(
      // Reversed orientation: the URL must still be lo/hi.
      () => useConnectionProvenance({ sourceId: 9, targetId: 4 }),
      { wrapper: createWrapper() },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/artists/4/relationships/9/provenance',
      { method: 'GET' },
    )
    expect(result.current.data).toEqual(provenance)
  })

  it('does not fetch while no pair is selected or when disabled', () => {
    const { result: idle } = renderHook(() => useConnectionProvenance(null), {
      wrapper: createWrapper(),
    })
    const { result: disabled } = renderHook(
      () => useConnectionProvenance({ sourceId: 1, targetId: 2 }, false),
      { wrapper: createWrapper() },
    )

    expect(idle.current.fetchStatus).toBe('idle')
    expect(disabled.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('surfaces a 404 (no stored connection) as an error state, not a crash', async () => {
    // Retry policy is the app-wide default (lib/queryClient.ts skips 4xx);
    // this asserts only that the hook settles into error for the merge's
    // undefined-provenance path.
    const notFound = Object.assign(new Error('Not Found'), { status: 404 })
    mockApiRequest.mockRejectedValue(notFound)

    const { result } = renderHook(
      () => useConnectionProvenance({ sourceId: 1, targetId: 2 }),
      { wrapper: createWrapper() },
    )

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
  })

  it('keys the cache orientation-free', () => {
    expect(connectionProvenanceQueryKey({ sourceId: 9, targetId: 4 })).toEqual(
      connectionProvenanceQueryKey({ sourceId: 4, targetId: 9 }),
    )
  })
})

describe('mergeProvenanceEntities', () => {
  const rows: EdgeTooltipLink[] = [
    { type: 'shared_bills', detail: { shared_count: 12 } },
    { type: 'shared_label', detail: { shared_count: 1 } },
  ]

  it('decorates matching rows by type and passes others through', () => {
    const merged = mergeProvenanceEntities(rows, provenance)

    // The fixture's endpoint-only `similar` connection is appended and the
    // grammar order puts it first (similar < shared_bills < shared_label).
    expect(merged.map(r => r.type)).toEqual(['similar', 'shared_bills', 'shared_label'])
    expect(merged[1].entities).toEqual(provenance.connections[0].entities)
    expect(merged[1].entityTotal).toBe(12)
    // shared_label had no provenance match — untouched text row.
    expect(merged[2].entities).toBeUndefined()
    expect(merged[2]).toBe(rows[1])
  })

  it('returns rows unchanged when provenance is absent or empty', () => {
    expect(mergeProvenanceEntities(rows, undefined)).toBe(rows)
    expect(mergeProvenanceEntities(rows, { connections: [] })).toBe(rows)
  })

  it('ignores provenance connections with empty entities (entity-less types)', () => {
    const merged = mergeProvenanceEntities(
      [{ type: 'similar' }],
      provenance,
    )
    expect(merged[0].entities).toBeUndefined()
  })

  it('appends connection types only the endpoint knows, in grammar order', () => {
    // The backend unions query-time signals (festival_cobill) most surfaces
    // never request — the panel lists ALL connections, so they must appear.
    const merged = mergeProvenanceEntities(rows, {
      connections: [
        {
          type: 'festival_cobill',
          score: 0.5,
          detail: { count: 1, festival_names: 'Desert Daze', most_recent_year: 2025 },
          entities: [{ kind: 'festival', id: 3, slug: 'desert-daze', name: 'Desert Daze', date: '2025' }],
          entity_total: 1,
        },
      ],
    })
    expect(merged.map(r => r.type)).toEqual(['shared_bills', 'shared_label', 'festival_cobill'])
    const appended = merged[2]
    expect(appended.score).toBe(0.5)
    expect(appended.detail).toEqual({ count: 1, festival_names: 'Desert Daze', most_recent_year: 2025 })
    expect(appended.entities).toHaveLength(1)
    expect(appended.entityTotal).toBe(1)
  })

  it('never drops rows the graph payload asserts', () => {
    const merged = mergeProvenanceEntities(rows, {
      connections: [{ type: 'radio_cooccurrence', score: 0.4, entities: [], entity_total: 0 }],
    })
    // The graph rows survive; the endpoint-only radio row is appended
    // (entity-less: renders as a text row from its score/detail).
    expect(merged.map(r => r.type)).toEqual([
      'shared_bills',
      'shared_label',
      'radio_cooccurrence',
    ])
    expect(merged[2].entities).toBeUndefined()
  })
})
