import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

import { useVenueConfirm, formatVenueConfirmError } from './useVenueConfirm'

describe('useVenueConfirm', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
  })

  it('POSTs to the venue-scoped confirm path and returns the aggregate', async () => {
    mockApiRequest.mockResolvedValue({
      confirmation_count: 8,
      last_confirmed_at: '2026-07-27T10:00:00Z',
      viewer_has_confirmed: true,
    })

    const { result } = renderHook(() => useVenueConfirm(), {
      wrapper: createWrapper(),
    })
    result.current.mutate(42)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/venues/42/confirm',
      { method: 'POST' },
    )
    expect(result.current.data?.confirmation_count).toBe(8)
  })

  it('refreshes only the venue queries that carry a provenance stamp', async () => {
    // A confirmation cannot change a venue's shows, genres, or bill network —
    // invalidating the whole ['venues'] prefix would refetch an open panel's
    // show list and graph for nothing.
    mockApiRequest.mockResolvedValue({
      confirmation_count: 1,
      viewer_has_confirmed: true,
    })

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useVenueConfirm(), { wrapper })
    result.current.mutate(42)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const predicate = invalidate.mock.calls[0][0]?.predicate as (q: {
      queryKey: readonly unknown[]
    }) => boolean
    expect(predicate({ queryKey: ['venues', 'list', {}] })).toBe(true)
    expect(predicate({ queryKey: ['venues', 'detail', '42'] })).toBe(true)
    expect(predicate({ queryKey: ['venues', 'shows', '42'] })).toBe(false)
    expect(predicate({ queryKey: ['venues', 'bill-network', '42', 'all', null] })).toBe(false)
    expect(predicate({ queryKey: ['venues', 'genres', '42'] })).toBe(false)
    expect(predicate({ queryKey: ['artists', 'list', {}] })).toBe(false)
  })

  it('surfaces a repeat confirm as an ordinary success, not an error', async () => {
    // The server write is ON CONFLICT DO NOTHING, so the second tap returns
    // 200 with the unchanged aggregate. The hook must not invent a
    // "already confirmed" failure state on top of that.
    mockApiRequest.mockResolvedValue({
      confirmation_count: 8,
      last_confirmed_at: '2026-07-27T10:00:00Z',
      viewer_has_confirmed: true,
    })

    const { result } = renderHook(() => useVenueConfirm(), {
      wrapper: createWrapper(),
    })
    result.current.mutate(42)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    result.current.mutate(42)
    await waitFor(() => expect(mockApiRequest).toHaveBeenCalledTimes(2))

    expect(result.current.isError).toBe(false)
    expect(result.current.data?.confirmation_count).toBe(8)
  })
})

describe('formatVenueConfirmError', () => {
  it('is silent when nothing failed', () => {
    expect(formatVenueConfirmError(null)).toBeNull()
    expect(formatVenueConfirmError(undefined)).toBeNull()
  })

  it('turns a 429 into a wait the user can act on', () => {
    // The whole point of the Retry-After path: a silently swallowed 429 reads
    // as "the button is broken" rather than "slow down".
    expect(formatVenueConfirmError({ status: 429, retryAfter: 47 })).toBe(
      'Too many confirmations — try again in 47s.',
    )
  })

  it('still says "slow down" when the header is missing or unusable', () => {
    expect(formatVenueConfirmError({ status: 429 })).toBe(
      'Too many confirmations — try again in a minute.',
    )
    expect(formatVenueConfirmError({ status: 429, retryAfter: NaN })).toBe(
      'Too many confirmations — try again in a minute.',
    )
  })

  it('names the fixable causes for 401 and 404', () => {
    expect(formatVenueConfirmError({ status: 401 })).toBe(
      'Your session expired.',
    )
    expect(formatVenueConfirmError({ status: 404 })).toBe(
      'This venue no longer exists.',
    )
  })

  it('falls back to a generic retry for anything else', () => {
    expect(formatVenueConfirmError({ status: 500 })).toBe(
      'Couldn’t confirm this venue. Please try again.',
    )
  })
})
