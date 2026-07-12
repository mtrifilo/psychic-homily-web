import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import {
  useFreshlyAdded,
  useMostActiveArtists,
  useMostAnticipated,
  useNewReleases,
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
})
