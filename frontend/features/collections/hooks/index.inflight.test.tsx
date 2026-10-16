import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import { useCreateCollection, useMyCollections } from './index'
import type { Collection, CollectionDetail } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: 'authenticated',
    isAuthenticated: true,
    user: { id: 7 },
  }),
}))

const myListUrl = '/auth/collections'

describe('collection writes while the list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useCreateCollection refetches the viewer’s collection list', async () => {
    const created = { id: 3, slug: 'new-one', title: 'New one' } as Collection
    const staleList = deferred<{ collections: Collection[]; total: number }>()
    const gets = routeGets({
      [myListUrl]: {
        stale: staleList.promise,
        fresh: { collections: [created], total: 1 },
      },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/collections') && init?.method === 'POST') {
        return Promise.resolve(created as unknown as CollectionDetail)
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useMyCollections(), create: useCreateCollection() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[myListUrl]).toBe(1))

    await act(() =>
      result.current.create.mutateAsync({
        title: 'New one',
        is_public: true,
        collaborative: false,
      })
    )
    await act(async () => {
      staleList.resolve({ collections: [], total: 0 })
      await staleList.promise
    })

    await waitFor(() =>
      expect(result.current.list.data?.collections.map((c) => c.id)).toEqual([3])
    )
    expect(gets.counts[myListUrl]).toBe(2)
  })
})
