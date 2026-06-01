import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

// Stub the 7 underlying queue hooks so we can test (a) the aggregation math and
// (b) that the `enabled` gate is threaded through to every one — the contract
// that keeps these admin-only endpoints from firing for non-admins / off-route.
const h = vi.hoisted(() => ({
  usePendingShows: vi.fn(),
  useUnverifiedVenues: vi.fn(),
  usePendingReports: vi.fn(),
  usePendingArtistReports: vi.fn(),
  useAdminPendingEdits: vi.fn(),
  useAdminEntityReports: vi.fn(),
  useAdminPendingComments: vi.fn(),
}))
vi.mock('./useAdminShows', () => ({ usePendingShows: h.usePendingShows }))
vi.mock('./useAdminVenues', () => ({ useUnverifiedVenues: h.useUnverifiedVenues }))
vi.mock('./useAdminReports', () => ({ usePendingReports: h.usePendingReports }))
vi.mock('./useAdminArtistReports', () => ({ usePendingArtistReports: h.usePendingArtistReports }))
vi.mock('./useAdminPendingEdits', () => ({ useAdminPendingEdits: h.useAdminPendingEdits }))
vi.mock('./useAdminEntityReports', () => ({ useAdminEntityReports: h.useAdminEntityReports }))
vi.mock('./useAdminComments', () => ({ useAdminPendingComments: h.useAdminPendingComments }))

import { useAdminNavCounts } from './useAdminNavCounts'

const total = (n: number) => ({ data: { total: n } })

describe('useAdminNavCounts', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    for (const fn of Object.values(h)) fn.mockReturnValue({ data: undefined })
  })

  it('aggregates the four nav counts (moderation = edits + entity reports + comments; reports = show + artist)', () => {
    h.usePendingShows.mockReturnValue(total(2))
    h.useUnverifiedVenues.mockReturnValue(total(1))
    h.usePendingReports.mockReturnValue(total(3))
    h.usePendingArtistReports.mockReturnValue(total(4))
    h.useAdminPendingEdits.mockReturnValue(total(5))
    h.useAdminEntityReports.mockReturnValue(total(6))
    h.useAdminPendingComments.mockReturnValue(total(7))

    const { result } = renderHook(() => useAdminNavCounts({ enabled: true }))

    expect(result.current).toEqual({
      moderation: 18, // 5 + 6 + 7
      pendingShows: 2,
      unverifiedVenues: 1,
      reports: 7, // 3 + 4
    })
  })

  it('treats missing data as zero', () => {
    // all hooks return { data: undefined } from beforeEach
    const { result } = renderHook(() => useAdminNavCounts({ enabled: true }))
    expect(result.current).toEqual({
      moderation: 0,
      pendingShows: 0,
      unverifiedVenues: 0,
      reports: 0,
    })
  })

  it('threads enabled through to every underlying query (the gating contract)', () => {
    renderHook(() => useAdminNavCounts({ enabled: false }))

    expect(h.usePendingShows).toHaveBeenCalledWith({ enabled: false })
    expect(h.useUnverifiedVenues).toHaveBeenCalledWith({ enabled: false })
    expect(h.usePendingReports).toHaveBeenCalledWith({ enabled: false })
    expect(h.usePendingArtistReports).toHaveBeenCalledWith({ enabled: false })
    expect(h.useAdminPendingEdits).toHaveBeenCalledWith({ status: 'pending', enabled: false })
    expect(h.useAdminEntityReports).toHaveBeenCalledWith({ status: 'pending', enabled: false })
    // positional signature: (limit, offset, options)
    expect(h.useAdminPendingComments).toHaveBeenCalledWith(25, 0, { enabled: false })
  })

  it('passes enabled: true through when enabled', () => {
    renderHook(() => useAdminNavCounts({ enabled: true }))
    expect(h.usePendingShows).toHaveBeenCalledWith({ enabled: true })
    expect(h.useAdminPendingComments).toHaveBeenCalledWith(25, 0, { enabled: true })
  })
})
