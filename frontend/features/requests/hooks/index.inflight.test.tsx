import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import { useCreateRequest, useRequests } from './index'
import type { Request, RequestListResponse } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

const listUrl = '/requests'

describe('request writes while the list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useCreateRequest refetches the requests list', async () => {
    const created = { id: 5, title: 'Add this band' } as Request
    const staleList = deferred<RequestListResponse>()
    const gets = routeGets({
      [listUrl]: {
        stale: staleList.promise,
        fresh: { requests: [created], total: 1 },
      },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith(listUrl) && init?.method === 'POST') {
        return Promise.resolve(created)
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useRequests(), create: useCreateRequest() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[listUrl]).toBe(1))

    await act(() =>
      result.current.create.mutateAsync({
        title: 'Add this band',
        entity_type: 'artist',
      })
    )
    await act(async () => {
      staleList.resolve({ requests: [], total: 0 })
      await staleList.promise
    })

    await waitFor(() =>
      expect(result.current.list.data?.requests.map((r) => r.id)).toEqual([5])
    )
    expect(gets.counts[listUrl]).toBe(2)
  })
})
