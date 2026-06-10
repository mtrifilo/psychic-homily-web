import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useEpisodeNeighbors } from './useEpisodeNeighbors'

const BASE = 'http://localhost:8080'

function episode(id: number, airDate: string) {
  return {
    id,
    show_id: 1,
    title: null,
    air_date: airDate,
    air_time: null,
    duration_minutes: null,
    archive_url: null,
    play_count: 10,
    created_at: '2026-01-01T00:00:00Z',
    artist_preview: [],
  }
}

describe('useEpisodeNeighbors', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('keys the query per show + date', () => {
    expect(radioQueryKeys.episodeNeighbors('drummer', '2026-06-02')).toEqual([
      'radio-shows',
      'drummer',
      'episode-neighbors',
      '2026-06-02',
    ])
  })

  it('walks the episodes list and returns neighbors', async () => {
    mockApiRequest.mockResolvedValueOnce({
      episodes: [
        episode(3, '2026-06-09'),
        episode(2, '2026-06-02'),
        episode(1, '2026-05-26'),
      ],
      total: 3,
    })

    const { result } = renderHook(
      () => useEpisodeNeighbors('drummer', '2026-06-02'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio-shows/drummer/episodes?limit=100`,
      { method: 'GET' }
    )
    expect(result.current.data?.newer?.air_date).toBe('2026-06-09')
    expect(result.current.data?.older?.air_date).toBe('2026-05-26')
  })

  it('does not fetch when slug or date is empty', () => {
    const { result: noSlug } = renderHook(() => useEpisodeNeighbors('', '2026-06-02'), {
      wrapper: createWrapper(),
    })
    const { result: noDate } = renderHook(() => useEpisodeNeighbors('drummer', ''), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(noSlug.current.fetchStatus).toBe('idle')
    expect(noDate.current.fetchStatus).toBe('idle')
  })
})
