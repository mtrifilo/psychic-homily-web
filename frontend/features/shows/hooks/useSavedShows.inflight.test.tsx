import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import { useSaveShow, useSavedShows } from './useSavedShows'
import type { SavedShowResponse, SavedShowsListResponse } from '../types'

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

const listUrl = '/saved-shows?'

describe('save writes while the saved list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useSaveShow refetches the viewer’s saved shows', async () => {
    const saved = { id: 9 } as SavedShowResponse
    const staleList = deferred<SavedShowsListResponse>()
    const gets = routeGets({
      [listUrl]: {
        stale: staleList.promise,
        fresh: { shows: [saved], total: 1, limit: 50, offset: 0 },
      },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/saved-shows/9') && init?.method === 'POST') {
        return Promise.resolve({ success: true, message: 'saved' })
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useSavedShows({ userId: '42' }), save: useSaveShow() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[listUrl]).toBe(1))

    await act(() => result.current.save.mutateAsync(9))
    await act(async () => {
      staleList.resolve({ shows: [], total: 0, limit: 50, offset: 0 })
      await staleList.promise
    })

    await waitFor(() =>
      expect(result.current.list.data?.shows.map((s) => s.id)).toEqual([9])
    )
    expect(gets.counts[listUrl]).toBe(2)
  })
})
