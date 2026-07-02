import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'
import type { RadioStationGraphResponse } from '../types'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useStationGraph } from './useStationGraph'

const BASE = 'http://localhost:8080'

const mockResponse: RadioStationGraphResponse = {
  station: {
    id: 1,
    slug: 'kexp',
    name: 'KEXP',
    artist_count: 2,
    edge_count: 1,
    window: 'last_12m',
  },
  clusters: [{ id: 'rs_1', label: 'The Morning Show', size: 2, color_index: 0 }],
  nodes: [
    {
      id: 10,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      upcoming_show_count: 0,
      cluster_id: 'rs_1',
      is_isolate: false,
      play_count: 12,
    },
    {
      id: 11,
      name: 'Numb Bats',
      slug: 'numb-bats',
      upcoming_show_count: 1,
      cluster_id: 'rs_1',
      is_isolate: false,
      play_count: 8,
    },
  ],
  links: [
    {
      source_id: 10,
      target_id: 11,
      type: 'radio_cooccurrence',
      score: 0.4,
      detail: { co_occurrence_count: 3, last_co_occurrence: '2026-06-20' },
      is_cross_cluster: false,
    },
  ],
}

describe('useStationGraph', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('uses the stationGraph(slug) query key', () => {
    expect(radioQueryKeys.stationGraph('kexp')).toEqual([
      'radio-stations',
      'kexp',
      'graph',
    ])
  })

  it('fetches the station graph payload by slug', async () => {
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useStationGraph({ slug: 'kexp' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/radio-stations/kexp/graph`, {
      method: 'GET',
    })
    expect(result.current.data).toEqual(mockResponse)
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useStationGraph({ slug: '' }), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when disabled', () => {
    const { result } = renderHook(
      () => useStationGraph({ slug: 'kexp', enabled: false }),
      { wrapper: createWrapper() },
    )
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })
})
