import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import { useShowDelete } from './useShowDelete'
import { useUpcomingShows } from './useShows'
import type { ShowResponse, UpcomingShowsResponse } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

const listUrl = '/shows/upcoming'

const upcoming = (shows: ShowResponse[]): UpcomingShowsResponse =>
  ({
    shows,
    timezone: 'America/Phoenix',
    total: shows.length,
    pagination: { next_cursor: null, has_more: false, limit: 20 },
  }) as unknown as UpcomingShowsResponse

describe('show writes while the upcoming list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useShowDelete refetches the upcoming shows', async () => {
    const kept = { id: 8 } as ShowResponse
    const deleted = { id: 9 } as ShowResponse
    const staleList = deferred<UpcomingShowsResponse>()
    const gets = routeGets({
      [listUrl]: { stale: staleList.promise, fresh: upcoming([kept]) },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/shows/9') && init?.method === 'DELETE') {
        return Promise.resolve(undefined)
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useUpcomingShows(), remove: useShowDelete() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[listUrl]).toBe(1))

    await act(() => result.current.remove.mutateAsync(9))
    await act(async () => {
      staleList.resolve(upcoming([kept, deleted]))
      await staleList.promise
    })

    await waitFor(() =>
      expect(result.current.list.data?.shows.map((s) => s.id)).toEqual([8])
    )
    expect(gets.counts[listUrl]).toBe(2)
  })
})
