import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useRecentRadioEpisodes } from './useRecentRadioEpisodes'

const BASE = 'http://localhost:8080'

describe('useRecentRadioEpisodes', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('carries limit/offset in the recentEpisodes() query key', () => {
    expect(radioQueryKeys.recentEpisodes({ limit: 12, offset: 0 })).toEqual([
      'radio',
      'episodes',
      'recent',
      { limit: 12, offset: 0 },
    ])
  })

  it('fetches the dial-wide feed with the default limit', async () => {
    const mockResponse = {
      episodes: [
        {
          id: 1,
          title: null,
          air_date: '2026-06-09',
          play_count: 24,
          archive_url: null,
          show_id: 3,
          show_name: 'The Night Owl Show',
          show_slug: 'night-owl',
          station_id: 2,
          station_name: 'WFMU',
          station_slug: 'wfmu',
          artist_preview: [],
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useRecentRadioEpisodes(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio/episodes/recent?limit=20`,
      { method: 'GET' }
    )
    expect(result.current.data).toEqual(mockResponse)
  })

  it('includes offset when paginating', async () => {
    mockApiRequest.mockResolvedValueOnce({ episodes: [], total: 0 })

    const { result } = renderHook(
      () => useRecentRadioEpisodes({ limit: 12, offset: 24 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('limit=12')
    expect(calledUrl).toContain('offset=24')
  })

  it('does not fetch when explicitly disabled', () => {
    const { result } = renderHook(
      () => useRecentRadioEpisodes({ enabled: false }),
      { wrapper: createWrapper() }
    )
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('surfaces API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useRecentRadioEpisodes(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
