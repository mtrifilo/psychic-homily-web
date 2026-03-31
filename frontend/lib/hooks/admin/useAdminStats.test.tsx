import { describe, it, expect } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/mocks/server'
import { TEST_API_BASE } from '@/test/mocks/handlers'
import { createWrapper } from '@/test/utils'
import { useAdminStats, useAdminActivity } from './useAdminStats'

describe('useAdminStats', () => {
  it('fetches admin dashboard stats', async () => {
    const { result } = renderHook(() => useAdminStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Verify data was returned from the MSW handler
    expect(result.current.data?.total_shows).toBe(100)
    expect(result.current.data?.total_artists).toBe(50)
    expect(result.current.data?.total_venues).toBe(20)
    expect(result.current.data?.pending_shows).toBe(5)
  })

  it('returns all expected stat fields', async () => {
    const { result } = renderHook(() => useAdminStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const data = result.current.data!
    expect(data).toEqual(
      expect.objectContaining({
        pending_shows: expect.any(Number),
        pending_venue_edits: expect.any(Number),
        pending_reports: expect.any(Number),
        unverified_venues: expect.any(Number),
        total_shows: expect.any(Number),
        total_venues: expect.any(Number),
        total_artists: expect.any(Number),
        total_users: expect.any(Number),
        total_shows_trend: expect.any(Number),
        total_venues_trend: expect.any(Number),
        total_artists_trend: expect.any(Number),
        total_users_trend: expect.any(Number),
      })
    )
  })

  it('handles API errors', async () => {
    server.use(
      http.get(`${TEST_API_BASE}/admin/stats`, () => {
        return HttpResponse.json({ message: 'Forbidden' }, { status: 403 })
      })
    )

    const { result } = renderHook(() => useAdminStats(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useAdminActivity', () => {
  it('fetches admin activity feed', async () => {
    const { result } = renderHook(() => useAdminActivity(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.events).toHaveLength(2)
    expect(result.current.data?.events[0].event_type).toBe('show_approved')
  })

  it('handles empty activity feed', async () => {
    server.use(
      http.get(`${TEST_API_BASE}/admin/activity`, () => {
        return HttpResponse.json({ events: [] })
      })
    )

    const { result } = renderHook(() => useAdminActivity(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.events).toEqual([])
  })
})
