import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import {
  useBusiestVenues,
  useChartEntityRank,
  useChartScenes,
  useChartsSummary,
  useFreshlyAdded,
  useMostActiveArtists,
  useMostAnticipated,
  useNewReleases,
  useOnTheRadio,
  useOpenersToWatch,
  usePersonalChartsStats,
  useTopTags,
} from './useCharts'

describe('chart hooks', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('passes the selected window and module limit', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [] })
    const { result } = renderHook(() => useMostActiveArtists('all_time', 9), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(
        /\/charts\/most-active-artists\?window=all_time&limit=9$/
      ),
      { method: 'GET' }
    )
  })

  it('includes the drill-down offset in the request and cache key', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [] })
    const { result } = renderHook(
      () => useMostActiveArtists('quarter', 50, { offset: 100 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(
        /\/charts\/most-active-artists\?window=quarter&limit=50&offset=100$/
      ),
      { method: 'GET' }
    )
  })

  it('requests most anticipated for the selected chart window', async () => {
    mockApiRequest.mockResolvedValueOnce({
      mode: 'soonest_upcoming',
      shows: [],
    })
    const { result } = renderHook(() => useMostAnticipated('month'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/charts\/most-anticipated\?window=month&limit=6$/),
      { method: 'GET' }
    )
  })

  it('requests linkable release metadata from the new-releases endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({ releases: [] })
    const { result } = renderHook(() => useNewReleases('quarter'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/charts\/new-releases\?window=quarter&limit=6$/),
      { method: 'GET' }
    )
  })

  it('uses the fixed freshly-added limit without a window', async () => {
    mockApiRequest.mockResolvedValueOnce({ items: [] })
    const { result } = renderHook(() => useFreshlyAdded(8), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/charts\/freshly-added\?limit=8$/),
      { method: 'GET' }
    )
  })

  it('refetches every chart module when the selected scene changes', async () => {
    mockApiRequest.mockResolvedValue({})
    const { result, rerender } = renderHook(
      ({ scene }: { scene: string }) => {
        const options = { scene }
        return [
          useMostActiveArtists('quarter', 7, options),
          useOnTheRadio('quarter', 7, options),
          useMostAnticipated('quarter', 6, options),
          useBusiestVenues('quarter', 7, options),
          useNewReleases('quarter', 6, options),
          useOpenersToWatch('quarter', 6, options),
          useTopTags('quarter', 7, options),
          useChartsSummary('quarter', options),
          useFreshlyAdded(6, options),
        ]
      },
      {
        initialProps: { scene: '' },
        wrapper: createWrapper(),
      }
    )

    await waitFor(() =>
      expect(result.current.every(request => request.isSuccess)).toBe(true)
    )
    expect(mockApiRequest).toHaveBeenCalledTimes(9)
    for (const [url] of mockApiRequest.mock.calls) {
      expect(String(url)).not.toContain('scene=')
    }

    mockApiRequest.mockClear()
    rerender({ scene: '38060' })
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(9))
    for (const [url] of mockApiRequest.mock.calls) {
      expect(String(url)).toContain('scene=38060')
    }
  })

  it('requests activity-weighted top tags with window and limit', async () => {
    mockApiRequest.mockResolvedValueOnce({ tags: [] })
    const { result } = renderHook(() => useTopTags('month', 7), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/charts\/top-tags\?window=month&limit=7$/),
      { method: 'GET' }
    )
  })

  it('refetches the coverage-floored scene list for each window', async () => {
    mockApiRequest.mockResolvedValue({ window: 'quarter', scenes: [] })
    const { result, rerender } = renderHook(
      ({ window }: { window: 'quarter' | 'month' }) => useChartScenes(window),
      {
        initialProps: { window: 'quarter' },
        wrapper: createWrapper(),
      }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenLastCalledWith(
      expect.stringMatching(/\/charts\/scenes\?window=quarter$/),
      { method: 'GET' }
    )

    mockApiRequest.mockResolvedValue({ window: 'month', scenes: [] })
    rerender({ window: 'month' })
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(2))
    expect(mockApiRequest).toHaveBeenLastCalledWith(
      expect.stringMatching(/\/charts\/scenes\?window=month$/),
      { method: 'GET' }
    )
  })

  it('fetches personal stats only after the authenticated user resolves', async () => {
    mockApiRequest.mockResolvedValueOnce({
      saved_shows: 12,
      artists_followed: 34,
      top_venue: null,
      first_activity_at: '2026-03-12T00:00:00Z',
    })
    const { result } = renderHook(() => usePersonalChartsStats('42', true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/charts\/me$/),
      { method: 'GET' }
    )
  })

  it('does not request personal stats for an anonymous visitor', () => {
    const { result } = renderHook(
      () => usePersonalChartsStats(undefined, false),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('requests entity rank with default quarter window', async () => {
    mockApiRequest.mockResolvedValueOnce({
      entity_type: 'show',
      entity_id: 12,
      window: 'quarter',
      module: 'most-anticipated',
      rank: 3,
    })
    const { result } = renderHook(() => useChartEntityRank('show', 12), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(
        /\/charts\/rank\?entity_type=show&entity_id=12&window=quarter$/
      ),
      { method: 'GET' }
    )
    expect(result.current.data?.rank).toBe(3)
  })
})
