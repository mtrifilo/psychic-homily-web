import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the feature api module
vi.mock('@/features/artists/api', () => ({
  artistEndpoints: {
    BILL_COMPOSITION: (artistId: string | number) =>
      `/artists/${artistId}/bill-composition`,
  },
  artistQueryKeys: {
    billComposition: (id: string | number, months: number) => [
      'artists',
      'billComposition',
      String(id),
      months,
    ],
  },
}))

// Import hooks after mocks are set up
import { useArtistBillComposition } from './useArtistBillComposition'
import type { ArtistBillComposition } from '../types'

const mockComposition: ArtistBillComposition = {
  artist: {
    id: 1,
    name: 'Center Artist',
    slug: 'center-artist',
    upcoming_show_count: 2,
  },
  stats: { total_shows: 12, headliner_count: 4, opener_count: 8 },
  opens_with: [],
  closes_with: [],
  graph: { center: { id: 1, name: 'Center Artist', slug: 'center-artist', upcoming_show_count: 2 }, nodes: [], links: [] },
  below_threshold: false,
  time_filter_months: 0,
}

describe('useArtistBillComposition', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches all-time composition (no months param) when months is 0', async () => {
    mockApiRequest.mockResolvedValueOnce(mockComposition)

    const { result } = renderHook(
      () => useArtistBillComposition({ artistId: 1, months: 0 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/artists/1/bill-composition', {
      method: 'GET',
    })
    expect(result.current.data).toEqual(mockComposition)
  })

  it('appends the months query param when a window is set', async () => {
    mockApiRequest.mockResolvedValueOnce(mockComposition)

    const { result } = renderHook(
      () => useArtistBillComposition({ artistId: 1, months: 12 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/1/bill-composition?months=12',
      { method: 'GET' }
    )
  })

  it('starts in a loading state before the request resolves', () => {
    // Never-resolving promise keeps the query pending so we can assert the
    // loading branch the bill-composition panel renders a skeleton for.
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))

    const { result } = renderHook(
      () => useArtistBillComposition({ artistId: 1, months: 0 }),
      { wrapper: createWrapper() }
    )

    expect(result.current.isLoading).toBe(true)
  })

  it('exposes an error when the composition fetch fails', async () => {
    const error = new Error('Artist not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(
      () => useArtistBillComposition({ artistId: 999, months: 0 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Artist not found')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useArtistBillComposition({ artistId: 1, months: 0, enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when artistId is 0 or negative', () => {
    const { result: result0 } = renderHook(
      () => useArtistBillComposition({ artistId: 0, months: 0 }),
      { wrapper: createWrapper() }
    )
    const { result: resultNeg } = renderHook(
      () => useArtistBillComposition({ artistId: -1, months: 0 }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result0.current.fetchStatus).toBe('idle')
    expect(resultNeg.current.fetchStatus).toBe('idle')
  })
})
