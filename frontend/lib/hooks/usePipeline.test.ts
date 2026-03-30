import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/utils'

// Mock apiRequest
const mockApiRequest = vi.fn()
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      PIPELINE: {
        VENUES: '/admin/pipeline/venues',
        VENUE_STATS: (id: number) => `/admin/pipeline/venues/${id}/stats`,
        VENUE_NOTES: (id: number) => `/admin/pipeline/venues/${id}/notes`,
        VENUE_CONFIG: (id: number) => `/admin/pipeline/venues/${id}/config`,
        VENUE_RUNS: (id: number) => `/admin/pipeline/venues/${id}/runs`,
        VENUE_RESET_RENDER: (id: number) => `/admin/pipeline/venues/${id}/reset-render`,
        EXTRACT: (id: number) => `/admin/pipeline/extract/${id}`,
        IMPORTS: '/admin/pipeline/imports',
      },
    },
  },
}))

vi.mock('../queryClient', () => ({
  queryKeys: {
    pipeline: {
      venues: ['pipeline', 'venues'],
      imports: (limit: number, offset: number) => ['pipeline', 'imports', String(limit), String(offset)],
      venueStats: (id: number) => ['pipeline', 'venueStats', String(id)],
      venueRuns: (id: number) => ['pipeline', 'venueRuns', String(id)],
    },
  },
}))

import {
  usePipelineVenues,
  useVenueRejectionStats,
  useImportHistory,
  useUpdateExtractionNotes,
  useUpdateVenueConfig,
  useVenueExtractionRuns,
  useResetRenderMethod,
  useExtractVenue,
} from './usePipeline'

function createWrapper(queryClient?: QueryClient) {
  const qc = queryClient ?? createTestQueryClient()
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children)
  }
}

describe('usePipeline hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('usePipelineVenues', () => {
    it('fetches pipeline venues', async () => {
      const mockVenues = {
        venues: [{ venue_id: 1, venue_name: 'Test Venue', preferred_source: 'ai' }],
        total: 1,
      }
      mockApiRequest.mockResolvedValueOnce(mockVenues)

      const { result } = renderHook(() => usePipelineVenues(), { wrapper: createWrapper() })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/venues')
    })

    it('respects enabled option', () => {
      renderHook(() => usePipelineVenues({ enabled: false }), { wrapper: createWrapper() })
      expect(mockApiRequest).not.toHaveBeenCalled()
    })

  })

  describe('useVenueRejectionStats', () => {
    it('fetches venue rejection stats', async () => {
      const mockStats = {
        total_extracted: 100,
        approved: 80,
        rejected: 15,
        pending: 5,
        rejection_breakdown: { non_music: 10, duplicate: 5 },
        approval_rate: 0.8,
        suggested_auto_approve: true,
      }
      mockApiRequest.mockResolvedValueOnce(mockStats)

      const { result } = renderHook(() => useVenueRejectionStats(42), { wrapper: createWrapper() })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/venues/42/stats')
    })

    it('does not fetch when venueId is 0', () => {
      renderHook(() => useVenueRejectionStats(0), { wrapper: createWrapper() })
      expect(mockApiRequest).not.toHaveBeenCalled()
    })

    it('does not fetch when enabled is false', () => {
      renderHook(() => useVenueRejectionStats(1, { enabled: false }), { wrapper: createWrapper() })
      expect(mockApiRequest).not.toHaveBeenCalled()
    })
  })

  describe('useImportHistory', () => {
    it('fetches import history with limit and offset', async () => {
      const mockData = {
        imports: [{ id: 1, venue_id: 1, venue_name: 'V1', source_type: 'ai' }],
        total: 1,
      }
      mockApiRequest.mockResolvedValueOnce(mockData)

      const { result } = renderHook(() => useImportHistory(10, 5), { wrapper: createWrapper() })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/imports?limit=10&offset=5')
    })

    it('uses defaults for limit and offset', async () => {
      mockApiRequest.mockResolvedValueOnce({ imports: [], total: 0 })

      const { result } = renderHook(() => useImportHistory(), { wrapper: createWrapper() })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/imports?limit=20&offset=0')
    })

    it('respects enabled option', () => {
      renderHook(() => useImportHistory(20, 0, { enabled: false }), { wrapper: createWrapper() })
      expect(mockApiRequest).not.toHaveBeenCalled()
    })
  })

  describe('useVenueExtractionRuns', () => {
    it('fetches venue extraction runs', async () => {
      const mockRuns = {
        runs: [{ id: 1, venue_id: 5, events_extracted: 10, events_imported: 8 }],
        total: 1,
      }
      mockApiRequest.mockResolvedValueOnce(mockRuns)

      const { result } = renderHook(() => useVenueExtractionRuns(5), { wrapper: createWrapper() })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/venues/5/runs')
    })

    it('does not fetch when venueId is 0', () => {
      renderHook(() => useVenueExtractionRuns(0), { wrapper: createWrapper() })
      expect(mockApiRequest).not.toHaveBeenCalled()
    })
  })

  describe('useUpdateExtractionNotes', () => {
    it('sends PATCH request with notes', async () => {
      mockApiRequest.mockResolvedValueOnce({ success: true, extraction_notes: 'test note' })
      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useUpdateExtractionNotes(), {
        wrapper: createWrapper(queryClient),
      })

      await act(async () => {
        await result.current.mutateAsync({ venueId: 3, extractionNotes: 'test note' })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/venues/3/notes', {
        method: 'PATCH',
        body: JSON.stringify({ extraction_notes: 'test note' }),
      })
      expect(invalidateSpy).toHaveBeenCalled()
    })

    it('sends null extractionNotes', async () => {
      mockApiRequest.mockResolvedValueOnce({ success: true })

      const { result } = renderHook(() => useUpdateExtractionNotes(), { wrapper: createWrapper() })

      await act(async () => {
        await result.current.mutateAsync({ venueId: 1, extractionNotes: null })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/venues/1/notes', {
        method: 'PATCH',
        body: JSON.stringify({ extraction_notes: null }),
      })
    })
  })

  describe('useUpdateVenueConfig', () => {
    it('sends PUT request with config', async () => {
      mockApiRequest.mockResolvedValueOnce({ venue_id: 1, venue_name: 'Test' })
      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useUpdateVenueConfig(), {
        wrapper: createWrapper(queryClient),
      })

      const config = {
        calendar_url: 'https://example.com/events',
        preferred_source: 'ai',
        auto_approve: true,
        strategy_locked: false,
      }

      await act(async () => {
        await result.current.mutateAsync({ venueId: 7, config })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/venues/7/config', {
        method: 'PUT',
        body: JSON.stringify(config),
      })
      expect(invalidateSpy).toHaveBeenCalled()
    })
  })

  describe('useResetRenderMethod', () => {
    it('sends POST request', async () => {
      mockApiRequest.mockResolvedValueOnce({ success: true })
      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useResetRenderMethod(), {
        wrapper: createWrapper(queryClient),
      })

      await act(async () => {
        await result.current.mutateAsync({ venueId: 4 })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/admin/pipeline/venues/4/reset-render', {
        method: 'POST',
      })
      expect(invalidateSpy).toHaveBeenCalled()
    })
  })

  describe('useExtractVenue', () => {
    it('sends POST request with dry_run=false by default', async () => {
      mockApiRequest.mockResolvedValueOnce({
        venue_id: 2,
        venue_name: 'Test',
        events_extracted: 5,
        events_imported: 3,
        events_skipped_non_music: 2,
        duration_ms: 1500,
        dry_run: false,
        initial_status: 'pending',
      })

      const { result } = renderHook(() => useExtractVenue(), { wrapper: createWrapper() })

      await act(async () => {
        await result.current.mutateAsync({ venueId: 2 })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        expect.stringContaining('dry_run=false'),
        { method: 'POST' }
      )
    })

    it('sends POST request with dry_run=true', async () => {
      mockApiRequest.mockResolvedValueOnce({
        venue_id: 2,
        events_extracted: 5,
        dry_run: true,
      })

      const { result } = renderHook(() => useExtractVenue(), { wrapper: createWrapper() })

      await act(async () => {
        await result.current.mutateAsync({ venueId: 2, dryRun: true })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        expect.stringContaining('dry_run=true'),
        { method: 'POST' }
      )
    })

    it('invalidates venues on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ venue_id: 2 })
      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useExtractVenue(), {
        wrapper: createWrapper(queryClient),
      })

      await act(async () => {
        await result.current.mutateAsync({ venueId: 2 })
      })

      expect(invalidateSpy).toHaveBeenCalled()
    })
  })
})
