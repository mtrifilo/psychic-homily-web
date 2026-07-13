import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import {
  useBusiestVenues,
  useChartScenes,
  useChartsSummary,
  useFreshlyAdded,
  useMostActiveArtists,
  useMostAnticipated,
  useNewReleases,
  useOnTheRadio,
  useOpenersToWatch,
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

  it('keeps most anticipated independent of the historical window', async () => {
    mockApiRequest.mockResolvedValueOnce({
      mode: 'soonest_upcoming',
      shows: [],
    })
    const { result } = renderHook(() => useMostAnticipated(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/charts\/most-anticipated\?limit=6$/),
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
          useMostAnticipated(6, options),
          useBusiestVenues('quarter', 7, options),
          useNewReleases('quarter', 6, options),
          useOpenersToWatch('quarter', 6, options),
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
    expect(mockApiRequest).toHaveBeenCalledTimes(8)
    for (const [url] of mockApiRequest.mock.calls) {
      expect(String(url)).not.toContain('scene=')
    }

    mockApiRequest.mockClear()
    rerender({ scene: '38060' })
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(8))
    for (const [url] of mockApiRequest.mock.calls) {
      expect(String(url)).toContain('scene=38060')
    }
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
})
