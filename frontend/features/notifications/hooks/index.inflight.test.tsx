import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import { useCreateFilter, useNotificationFilters } from './index'
import type { CreateFilterInput, NotificationFilter } from '../types'

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

const listUrl = '/me/notification-filters'

describe('filter writes while the filter list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useCreateFilter refetches the viewer’s filters', async () => {
    const created = { id: 6, name: 'Phoenix punk' } as NotificationFilter
    const staleList = deferred<{ filters: NotificationFilter[] }>()
    const gets = routeGets({
      [listUrl]: { stale: staleList.promise, fresh: { filters: [created] } },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/me/notification-filters') && init?.method === 'POST') {
        return Promise.resolve(created)
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useNotificationFilters(), create: useCreateFilter() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[listUrl]).toBe(1))

    await act(() =>
      result.current.create.mutateAsync({
        name: 'Phoenix punk',
      } as CreateFilterInput)
    )
    await act(async () => {
      staleList.resolve({ filters: [] })
      await staleList.promise
    })

    await waitFor(() =>
      expect(result.current.list.data?.filters.map((f) => f.id)).toEqual([6])
    )
    expect(gets.counts[listUrl]).toBe(2)
  })
})
