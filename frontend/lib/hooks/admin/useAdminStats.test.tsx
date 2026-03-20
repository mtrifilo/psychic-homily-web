import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('../../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      STATS: '/admin/stats',
      ACTIVITY: '/admin/activity',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    admin: {
      stats: ['admin', 'stats'],
      activity: ['admin', 'activity'],
    },
  },
}))

// Import hooks after mocks are set up
import { useAdminStats, useAdminActivity } from './useAdminStats'

describe('useAdminStats', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useAdminStats', () => {
    it('fetches admin dashboard stats successfully', async () => {
      const mockResponse = {
        pending_shows: 5,
        pending_venue_edits: 2,
        pending_reports: 1,
        unverified_venues: 3,
        total_shows: 100,
        total_venues: 20,
        total_artists: 50,
        total_users: 30,
        shows_submitted_last_7_days: 10,
        users_registered_last_7_days: 5,
        total_shows_trend: 3,
        total_venues_trend: -1,
        total_artists_trend: 2,
        total_users_trend: 5,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useAdminStats(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/admin/stats', {
        method: 'GET',
      })
      expect(result.current.data?.pending_shows).toBe(5)
      expect(result.current.data?.total_shows).toBe(100)
      expect(result.current.data?.total_shows_trend).toBe(3)
    })

    it('returns all stat fields', async () => {
      const mockResponse = {
        pending_shows: 0,
        pending_venue_edits: 0,
        pending_reports: 0,
        unverified_venues: 0,
        total_shows: 250,
        total_venues: 45,
        total_artists: 120,
        total_users: 80,
        shows_submitted_last_7_days: 0,
        users_registered_last_7_days: 0,
        total_shows_trend: 0,
        total_venues_trend: 0,
        total_artists_trend: 0,
        total_users_trend: 0,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useAdminStats(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.pending_shows).toBe(0)
      expect(result.current.data?.pending_venue_edits).toBe(0)
      expect(result.current.data?.pending_reports).toBe(0)
      expect(result.current.data?.unverified_venues).toBe(0)
      expect(result.current.data?.total_shows).toBe(250)
      expect(result.current.data?.total_venues).toBe(45)
      expect(result.current.data?.total_artists).toBe(120)
      expect(result.current.data?.total_users).toBe(80)
      expect(result.current.data?.shows_submitted_last_7_days).toBe(0)
      expect(result.current.data?.users_registered_last_7_days).toBe(0)
    })

    it('handles negative trend values', async () => {
      const mockResponse = {
        pending_shows: 0,
        pending_venue_edits: 0,
        pending_reports: 0,
        unverified_venues: 0,
        total_shows: 100,
        total_venues: 20,
        total_artists: 50,
        total_users: 30,
        shows_submitted_last_7_days: 3,
        users_registered_last_7_days: 1,
        total_shows_trend: -5,
        total_venues_trend: -2,
        total_artists_trend: -1,
        total_users_trend: -3,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useAdminStats(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.total_shows_trend).toBe(-5)
      expect(result.current.data?.total_venues_trend).toBe(-2)
    })

    it('handles API error', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useAdminStats(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles authentication error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useAdminStats(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })
  })

  describe('useAdminActivity', () => {
    it('fetches activity events successfully', async () => {
      const mockResponse = {
        events: [
          {
            id: 1,
            event_type: 'show_approved',
            description: 'Show "Metal Monday" was approved',
            entity_type: 'show',
            entity_slug: 'metal-monday-2026-03-20',
            timestamp: '2026-03-19T10:30:00Z',
            actor_name: 'admin',
          },
          {
            id: 2,
            event_type: 'venue_verified',
            description: 'Venue "The Rebel Lounge" was verified',
            entity_type: 'venue',
            entity_slug: 'the-rebel-lounge',
            timestamp: '2026-03-19T09:15:00Z',
            actor_name: 'admin',
          },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useAdminActivity(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/admin/activity', {
        method: 'GET',
      })
      expect(result.current.data?.events).toHaveLength(2)
      expect(result.current.data?.events[0].event_type).toBe('show_approved')
      expect(result.current.data?.events[1].entity_slug).toBe(
        'the-rebel-lounge'
      )
    })

    it('handles empty activity feed', async () => {
      mockApiRequest.mockResolvedValueOnce({ events: [] })

      const { result } = renderHook(() => useAdminActivity(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.events).toHaveLength(0)
    })

    it('handles events with optional fields missing', async () => {
      const mockResponse = {
        events: [
          {
            id: 3,
            event_type: 'user_registered',
            description: 'New user registered',
            timestamp: '2026-03-19T08:00:00Z',
            // entity_type, entity_slug, actor_name are optional
          },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useAdminActivity(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.events[0].entity_type).toBeUndefined()
      expect(result.current.data?.events[0].entity_slug).toBeUndefined()
      expect(result.current.data?.events[0].actor_name).toBeUndefined()
    })

    it('handles API error', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useAdminActivity(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles authentication error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useAdminActivity(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })
  })
})
