import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import { useCancelPendingEdit } from './useCancelPendingEdit'
import { useMyPendingEdits } from './useMyPendingEdits'
import type { PendingEditResponse } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

const listUrl = '/my/pending-edits?'

type PendingEditList = { edits: PendingEditResponse[]; total: number }

const edits = (...ids: number[]): PendingEditList => ({
  edits: ids.map((id) => ({ id }) as PendingEditResponse),
  total: ids.length,
})

describe('cancels while the pending-edit list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useCancelPendingEdit refetches the viewer’s pending edits', async () => {
    const staleList = deferred<PendingEditList>()
    const gets = routeGets({
      [listUrl]: { stale: staleList.promise, fresh: edits(4) },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/my/pending-edits/5') && init?.method === 'DELETE') {
        return Promise.resolve({ success: true })
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useMyPendingEdits(), cancel: useCancelPendingEdit() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[listUrl]).toBe(1))

    await act(() => result.current.cancel.mutateAsync(5))
    await act(async () => {
      staleList.resolve(edits(4, 5))
      await staleList.promise
    })

    await waitFor(() =>
      expect(result.current.list.data?.edits.map((e) => e.id)).toEqual([4])
    )
    expect(gets.counts[listUrl]).toBe(2)
  })
})
