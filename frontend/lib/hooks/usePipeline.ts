'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'

// --- Types ---

export interface VenueExtractionRun {
  id: number
  venue_id: number
  run_at: string
  render_method?: string
  preferred_source?: string
  events_extracted: number
  events_imported: number
  content_hash?: string
  http_status?: number
  error?: string
  duration_ms: number
  created_at: string
}

export interface PipelineVenueInfo {
  venue_id: number
  venue_name: string
  venue_slug: string
  calendar_url?: string
  preferred_source: string
  render_method?: string
  feed_url?: string
  last_extracted_at?: string
  events_expected: number
  consecutive_failures: number
  strategy_locked: boolean
  auto_approve: boolean
  extraction_notes?: string
  approval_rate?: number
  total_runs: number
  last_run?: VenueExtractionRun
}

export interface VenueRejectionStats {
  total_extracted: number
  approved: number
  rejected: number
  pending: number
  rejection_breakdown: Record<string, number>
  approval_rate: number
  suggested_auto_approve: boolean
}

// --- Queries ---

export function usePipelineVenues(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.pipeline.venues,
    queryFn: async () => {
      const data = await apiRequest<{ venues: PipelineVenueInfo[]; total: number }>(
        API_ENDPOINTS.ADMIN.PIPELINE.VENUES
      )
      return data
    },
    enabled: options?.enabled ?? true,
  })
}

export function useVenueRejectionStats(venueId: number, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.pipeline.venueStats(venueId),
    queryFn: async () => {
      const data = await apiRequest<VenueRejectionStats>(
        API_ENDPOINTS.ADMIN.PIPELINE.VENUE_STATS(venueId)
      )
      return data
    },
    enabled: (options?.enabled ?? true) && venueId > 0,
  })
}

export interface ImportHistoryEntry {
  id: number
  venue_id: number
  venue_name: string
  venue_slug: string
  source_type: string
  render_method?: string
  events_extracted: number
  events_imported: number
  duration_ms: number
  error?: string
  run_at: string
}

export function useImportHistory(
  limit: number = 20,
  offset: number = 0,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: queryKeys.pipeline.imports(limit, offset),
    queryFn: async () => {
      const url = `${API_ENDPOINTS.ADMIN.PIPELINE.IMPORTS}?limit=${limit}&offset=${offset}`
      const data = await apiRequest<{ imports: ImportHistoryEntry[]; total: number }>(url)
      return data
    },
    enabled: options?.enabled ?? true,
  })
}

// --- Mutations ---

export function useUpdateExtractionNotes() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      venueId,
      extractionNotes,
    }: {
      venueId: number
      extractionNotes: string | null
    }) => {
      return apiRequest<{ success: boolean; extraction_notes?: string }>(
        API_ENDPOINTS.ADMIN.PIPELINE.VENUE_NOTES(venueId),
        {
          method: 'PATCH',
          body: JSON.stringify({ extraction_notes: extractionNotes }),
        }
      )
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pipeline.venues })
      queryClient.invalidateQueries({
        queryKey: queryKeys.pipeline.venueStats(variables.venueId),
      })
    },
  })
}

export function useUpdateVenueConfig() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      venueId,
      config,
    }: {
      venueId: number
      config: {
        calendar_url?: string | null
        preferred_source: string
        render_method?: string | null
        feed_url?: string | null
        auto_approve: boolean
        strategy_locked: boolean
        extraction_notes?: string | null
      }
    }) => {
      return apiRequest<PipelineVenueInfo>(
        API_ENDPOINTS.ADMIN.PIPELINE.VENUE_CONFIG(venueId),
        {
          method: 'PUT',
          body: JSON.stringify(config),
        }
      )
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pipeline.venues })
    },
  })
}

export function useVenueExtractionRuns(venueId: number, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: queryKeys.pipeline.venueRuns(venueId),
    queryFn: async () => {
      const data = await apiRequest<{ runs: VenueExtractionRun[]; total: number }>(
        API_ENDPOINTS.ADMIN.PIPELINE.VENUE_RUNS(venueId)
      )
      return data
    },
    enabled: (options?.enabled ?? true) && venueId > 0,
  })
}

export function useResetRenderMethod() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ venueId }: { venueId: number }) => {
      return apiRequest<{ success: boolean }>(
        API_ENDPOINTS.ADMIN.PIPELINE.VENUE_RESET_RENDER(venueId),
        { method: 'POST' }
      )
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pipeline.venues })
    },
  })
}

export function useExtractVenue() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ venueId, dryRun = false }: { venueId: number; dryRun?: boolean }) => {
      const url = `${API_ENDPOINTS.ADMIN.PIPELINE.EXTRACT(venueId)}?dry_run=${dryRun}`
      return apiRequest<{
        venue_id: number
        venue_name: string
        events_extracted: number
        events_imported: number
        events_skipped_non_music: number
        duration_ms: number
        dry_run: boolean
        initial_status: string
        warnings?: string[]
      }>(url, { method: 'POST' })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.pipeline.venues })
    },
  })
}
