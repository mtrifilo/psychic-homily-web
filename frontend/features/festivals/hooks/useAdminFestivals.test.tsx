import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

// The endpoint builders embed API_BASE_URL; pin it so the assertions below
// match the exact URLs the hooks pass to apiRequest.
const BASE = 'http://localhost:8080'

import {
  useCreateFestival,
  useUpdateFestival,
  useDeleteFestival,
  useAddFestivalArtist,
  useUpdateFestivalArtist,
  useRemoveFestivalArtist,
  useAddFestivalVenue,
  useRemoveFestivalVenue,
} from './useAdminFestivals'

let queryClient: QueryClient
let invalidateSpy: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  vi.clearAllMocks()
  mockApiRequest.mockReset()
  queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  })
  invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
})

function wrapper() {
  return createWrapperWithClient(queryClient)
}

describe('useCreateFestival', () => {
  it('POSTs to the create endpoint and invalidates the festivals key', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'M3F' })

    const { result } = renderHook(() => useCreateFestival(), {
      wrapper: wrapper(),
    })

    result.current.mutate({
      name: 'M3F',
      series_slug: 'm3f',
      edition_year: 2026,
      start_date: '2026-03-06',
      end_date: '2026-03-08',
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/festivals`, {
      method: 'POST',
      body: JSON.stringify({
        name: 'M3F',
        series_slug: 'm3f',
        edition_year: 2026,
        start_date: '2026-03-06',
        end_date: '2026-03-08',
      }),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['festivals'] })
  })

  it('surfaces the error and does not invalidate when the request fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('boom'))

    const { result } = renderHook(() => useCreateFestival(), {
      wrapper: wrapper(),
    })

    result.current.mutate({
      name: 'M3F',
      series_slug: 'm3f',
      edition_year: 2026,
      start_date: '2026-03-06',
      end_date: '2026-03-08',
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toEqual(new Error('boom'))
    expect(invalidateSpy).not.toHaveBeenCalled()
  })
})

describe('useUpdateFestival', () => {
  it('PUTs to the festival endpoint and invalidates on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 42, name: 'Renamed' })

    const { result } = renderHook(() => useUpdateFestival(), {
      wrapper: wrapper(),
    })

    result.current.mutate({ festivalId: 42, data: { name: 'Renamed' } })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/festivals/42`, {
      method: 'PUT',
      body: JSON.stringify({ name: 'Renamed' }),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['festivals'] })
  })
})

describe('useDeleteFestival', () => {
  it('DELETEs the festival endpoint and invalidates on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useDeleteFestival(), {
      wrapper: wrapper(),
    })

    result.current.mutate(42)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/festivals/42`, {
      method: 'DELETE',
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['festivals'] })
  })
})

describe('useAddFestivalArtist', () => {
  it('POSTs to the lineup endpoint and invalidates on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, artist_id: 7 })

    const { result } = renderHook(() => useAddFestivalArtist(), {
      wrapper: wrapper(),
    })

    result.current.mutate({
      festivalId: 1,
      data: { artist_id: 7, billing_tier: 'headliner' },
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/festivals/1/artists`, {
      method: 'POST',
      body: JSON.stringify({ artist_id: 7, billing_tier: 'headliner' }),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['festivals'] })
  })
})

describe('useUpdateFestivalArtist', () => {
  it('PUTs to the lineup-entry endpoint and invalidates on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, artist_id: 7 })

    const { result } = renderHook(() => useUpdateFestivalArtist(), {
      wrapper: wrapper(),
    })

    result.current.mutate({
      festivalId: 1,
      artistId: 7,
      data: { position: 3 },
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/festivals/1/artists/7`,
      { method: 'PUT', body: JSON.stringify({ position: 3 }) }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['festivals'] })
  })
})

describe('useRemoveFestivalArtist', () => {
  it('DELETEs the lineup-entry endpoint and invalidates on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useRemoveFestivalArtist(), {
      wrapper: wrapper(),
    })

    result.current.mutate({ festivalId: 1, artistId: 7 })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/festivals/1/artists/7`,
      { method: 'DELETE' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['festivals'] })
  })
})

describe('useAddFestivalVenue', () => {
  it('POSTs to the venue endpoint and invalidates on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, venue_id: 9 })

    const { result } = renderHook(() => useAddFestivalVenue(), {
      wrapper: wrapper(),
    })

    result.current.mutate({
      festivalId: 1,
      data: { venue_id: 9, is_primary: true },
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(`${BASE}/festivals/1/venues`, {
      method: 'POST',
      body: JSON.stringify({ venue_id: 9, is_primary: true }),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['festivals'] })
  })
})

describe('useRemoveFestivalVenue', () => {
  it('DELETEs the venue endpoint and invalidates on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useRemoveFestivalVenue(), {
      wrapper: wrapper(),
    })

    result.current.mutate({ festivalId: 1, venueId: 9 })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      `${BASE}/festivals/1/venues/9`,
      { method: 'DELETE' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['festivals'] })
  })
})
