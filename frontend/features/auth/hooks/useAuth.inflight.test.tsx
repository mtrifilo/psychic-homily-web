import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import type { APIToken } from '../types'
import { useAPITokens, useCreateAPIToken } from './useAuth'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

const listUrl = '/admin/tokens'

describe('token writes while the token list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useCreateAPIToken refetches the token list', async () => {
    const created = { id: 3, description: 'discovery' } as APIToken
    const staleList = deferred<{ tokens: APIToken[] }>()
    const gets = routeGets({
      [listUrl]: { stale: staleList.promise, fresh: { tokens: [created] } },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/admin/tokens') && init?.method === 'POST') {
        return Promise.resolve({ id: 3, token: 'phk_x', description: 'discovery' })
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useAPITokens(), create: useCreateAPIToken() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[listUrl]).toBe(1))

    await act(() =>
      result.current.create.mutateAsync({ description: 'discovery' })
    )
    await act(async () => {
      staleList.resolve({ tokens: [] })
      await staleList.promise
    })

    await waitFor(() =>
      expect(result.current.list.data?.tokens.map((t) => t.id)).toEqual([3])
    )
    expect(gets.counts[listUrl]).toBe(2)
  })
})
