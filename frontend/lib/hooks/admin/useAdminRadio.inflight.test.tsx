import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { createWrapperWithClient } from '@/test/utils'
import {
  clientWithAppDefaults,
  deferred,
  routeGets,
} from '@/test/inflightOrdering'
import {
  useAdminRadioStations,
  useCreateRadioStation,
  type CreateRadioStationInput,
  type RadioStationListItem,
} from './useAdminRadio'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

const listUrl = '/radio-stations'

type StationList = { stations: RadioStationListItem[] | null; count: number }

describe('station writes while the station list is in its first fetch', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('useCreateRadioStation refetches the station list', async () => {
    const created = { id: 5, name: 'WFMU' } as RadioStationListItem
    const staleList = deferred<StationList>()
    const gets = routeGets({
      [listUrl]: {
        stale: staleList.promise,
        fresh: { stations: [created], count: 1 },
      },
    })
    mockApiRequest.mockImplementation((url: string, init?: RequestInit) => {
      const get = gets.handle(url, init)
      if (get) return get
      if (url.endsWith('/admin/radio-stations') && init?.method === 'POST') {
        return Promise.resolve(created)
      }
      return Promise.reject(new Error(`unexpected request ${url}`))
    })

    const client = clientWithAppDefaults()
    const { result } = renderHook(
      () => ({ list: useAdminRadioStations(), create: useCreateRadioStation() }),
      { wrapper: createWrapperWithClient(client) }
    )
    await waitFor(() => expect(gets.counts[listUrl]).toBe(1))

    await act(() =>
      result.current.create.mutateAsync({
        name: 'WFMU',
      } as CreateRadioStationInput)
    )
    await act(async () => {
      staleList.resolve({ stations: [], count: 0 })
      await staleList.promise
    })

    await waitFor(() =>
      expect(result.current.list.data?.stations?.map((s) => s.id)).toEqual([5])
    )
    expect(gets.counts[listUrl]).toBe(2)
  })
})
