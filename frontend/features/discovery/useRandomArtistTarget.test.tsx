import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    DISCOVERY: { RANDOM_ARTIST_TARGET: '/explore/shuffle-target' },
  },
}))

import { useRandomArtistTarget } from './useRandomArtistTarget'

describe('useRandomArtistTarget', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('waits for an explicit refetch and requests a fresh target each time', async () => {
    const firstTarget = {
      artist_id: 2,
      artist_slug: 'playboy-manbaby',
      artist_name: 'Playboy Manbaby',
    }
    const emptyTarget = {
      artist_id: null,
      artist_slug: null,
      artist_name: null,
    }
    mockApiRequest
      .mockResolvedValueOnce(firstTarget)
      .mockResolvedValueOnce(emptyTarget)

    const { result } = renderHook(() => useRandomArtistTarget(), {
      wrapper: createWrapper(),
    })
    expect(mockApiRequest).not.toHaveBeenCalled()

    let firstResult: Awaited<ReturnType<typeof result.current.refetch>> | undefined
    await act(async () => {
      firstResult = await result.current.refetch()
    })
    expect(firstResult?.data).toEqual(firstTarget)

    let secondResult: Awaited<ReturnType<typeof result.current.refetch>> | undefined
    await act(async () => {
      secondResult = await result.current.refetch()
    })
    expect(secondResult?.data).toEqual(emptyTarget)
    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    expect(mockApiRequest).toHaveBeenNthCalledWith(
      1,
      '/explore/shuffle-target',
      { method: 'GET' },
    )
    expect(mockApiRequest).toHaveBeenNthCalledWith(
      2,
      '/explore/shuffle-target',
      { method: 'GET' },
    )
  })
})
