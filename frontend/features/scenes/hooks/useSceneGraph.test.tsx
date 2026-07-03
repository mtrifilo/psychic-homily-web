import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import type { SceneGraphResponse } from '../types'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  }
})

import { useSceneGraph } from './useScenes'

const mockResponse: SceneGraphResponse = {
  scene: {
    slug: 'phoenix-az',
    city: 'Phoenix',
    state: 'AZ',
    artist_count: 0,
    edge_count: 0,
    metro_roster_total: 0,
    roster_truncated: false,
  },
  clusters: [],
  nodes: [],
  links: [],
}

describe('useSceneGraph cluster_by (PSY-1320)', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
    mockApiRequest.mockResolvedValue(mockResponse)
  })

  it('omits cluster_by for the default venue mode', async () => {
    const { result } = renderHook(() => useSceneGraph({ slug: 'phoenix-az' }), {
      wrapper: createWrapper(),
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const endpoint = mockApiRequest.mock.calls[0][0] as string
    expect(endpoint).not.toContain('cluster_by')
  })

  it('omits cluster_by when venue is passed explicitly (backend default)', async () => {
    const { result } = renderHook(
      () => useSceneGraph({ slug: 'phoenix-az', clusterBy: 'venue' }),
      { wrapper: createWrapper() }
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const endpoint = mockApiRequest.mock.calls[0][0] as string
    expect(endpoint).not.toContain('cluster_by')
  })

  it('sends cluster_by=community and keys the cache per mode', async () => {
    const { result } = renderHook(
      () => useSceneGraph({ slug: 'phoenix-az', clusterBy: 'community' }),
      { wrapper: createWrapper() }
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const endpoint = mockApiRequest.mock.calls[0][0] as string
    expect(endpoint).toContain('cluster_by=community')

    // Mode is part of the query key, so venue/community responses never
    // overwrite each other; the venue default and an explicit 'venue' share
    // one key.
    expect(queryKeys.scenes.graph('phoenix-az', undefined, 'community')).not.toEqual(
      queryKeys.scenes.graph('phoenix-az', undefined, 'venue')
    )
    expect(queryKeys.scenes.graph('phoenix-az')).toEqual(
      queryKeys.scenes.graph('phoenix-az', undefined, 'venue')
    )
  })

  it('refetches when the mode changes and keeps previous data while loading', async () => {
    const { result, rerender } = renderHook(
      ({ clusterBy }: { clusterBy: 'venue' | 'community' }) =>
        useSceneGraph({ slug: 'phoenix-az', clusterBy }),
      {
        wrapper: createWrapper(),
        initialProps: { clusterBy: 'venue' } as { clusterBy: 'venue' | 'community' },
      }
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledTimes(1)

    // Hold the community response open so the transition state is observable.
    let resolveSecond: (value: SceneGraphResponse) => void = () => {}
    mockApiRequest.mockImplementationOnce(
      () => new Promise<SceneGraphResponse>(resolve => (resolveSecond = resolve))
    )

    rerender({ clusterBy: 'community' })
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(2))

    // keepPreviousData: the venue payload stays rendered mid-switch (the
    // fullscreen-overlay `available` contract, PSY-1305).
    expect(result.current.data).toEqual(mockResponse)
    expect(result.current.isPlaceholderData).toBe(true)

    resolveSecond(mockResponse)
    await waitFor(() => expect(result.current.isPlaceholderData).toBe(false))
  })
})
