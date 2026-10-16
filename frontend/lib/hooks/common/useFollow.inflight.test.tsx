import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import type { FollowingEntity, FollowingListResponse } from '@/lib/types/follow'
import { useFollow, useMyFollowing } from './useFollow'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: 'authenticated',
    isAuthenticated: true,
    user: { id: 42 },
  }),
}))

const listUrl = '/me/following?'

const page = (following: FollowingEntity[]): FollowingListResponse => ({
  following,
  total: following.length,
  limit: 20,
  offset: 0,
})

describe('follow writes while the following list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useFollow refetches the viewer’s following list', async () => {
    const followed = {
      entity_type: 'artist',
      entity_id: 9,
      name: 'Gatecreeper',
      slug: 'gatecreeper',
      followed_at: '2026-10-16T00:00:00Z',
    } satisfies FollowingEntity
    const staleList = deferred<FollowingListResponse>()
    const gets = routeGets({
      [listUrl]: { stale: staleList.promise, fresh: page([followed]) },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/artists/9/follow') && init?.method === 'POST') {
        return Promise.resolve({ success: true, message: 'followed' })
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useMyFollowing(), follow: useFollow() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[listUrl]).toBe(1))

    await act(() =>
      result.current.follow.mutateAsync({ entityType: 'artists', entityId: 9 })
    )
    await act(async () => {
      staleList.resolve(page([]))
      await staleList.promise
    })

    await waitFor(() =>
      expect(
        result.current.list.data?.following.map((f) => f.entity_id)
      ).toEqual([9])
    )
    expect(gets.counts[listUrl]).toBe(2)
  })
})
