import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { radioQueryKeys } from '../api'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

import { useStationNowPlaying } from './useStationNowPlaying'

const BASE = 'http://localhost:8080'

describe('useStationNowPlaying', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('uses the stationNowPlaying(slug) query key', () => {
    expect(radioQueryKeys.stationNowPlaying('kexp')).toEqual([
      'radio-stations',
      'kexp',
      'now-playing',
    ])
  })

  it('fetches the now-playing payload by station slug', async () => {
    const mockResponse = {
      source: 'live',
      on_air: true,
      show: { id: 3, name: 'The Morning Show', slug: 'the-morning-show', host_name: null },
      show_name: 'The Morning Show',
      host_name: null,
      current_track: null,
      recent_artists: [],
      episode_air_date: null,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useStationNowPlaying('kexp'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/radio-stations/kexp/now-playing`,
      { method: 'GET' }
    )
    expect(result.current.data).toEqual(mockResponse)
  })

  it('does not fetch when slug is empty', () => {
    const { result } = renderHook(() => useStationNowPlaying(''), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('surfaces endpoint errors', async () => {
    const error = new Error('Station not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useStationNowPlaying('nope'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Station not found')
  })
})
