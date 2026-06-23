'use client'

/**
 * Admin Radio Hooks
 *
 * TanStack Query hooks for admin radio station/show CRUD operations,
 * playlist fetch triggering, and radio stats.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import { createInvalidateQueries } from '@/lib/queryClient'

// ============================================================================
// Query Keys
// ============================================================================

export const radioQueryKeys = {
  all: ['radio'] as const,
  stations: ['radio', 'stations'] as const,
  stationDetail: (id: number) => ['radio', 'stations', id] as const,
  shows: (stationId: number) => ['radio', 'shows', stationId] as const,
  stats: ['radio', 'stats'] as const,
  syncRun: (runId: number) => ['radio', 'sync-runs', runId] as const,
  unmatched: (stationId: number) => ['radio', 'unmatched', stationId] as const,
}

// ============================================================================
// Endpoints
// ============================================================================

const RADIO_ENDPOINTS = {
  // Public
  STATIONS: `${API_BASE_URL}/radio-stations`,
  STATION_DETAIL: (slug: string) => `${API_BASE_URL}/radio-stations/${slug}`,
  SHOWS: `${API_BASE_URL}/radio-shows`,
  STATS: `${API_BASE_URL}/radio/stats`,
  // Admin
  ADMIN_CREATE_STATION: `${API_BASE_URL}/admin/radio-stations`,
  ADMIN_UPDATE_STATION: (id: number) => `${API_BASE_URL}/admin/radio-stations/${id}`,
  ADMIN_DELETE_STATION: (id: number) => `${API_BASE_URL}/admin/radio-stations/${id}`,
  ADMIN_CREATE_SHOW: (stationId: number) => `${API_BASE_URL}/admin/radio-stations/${stationId}/shows`,
  ADMIN_UPDATE_SHOW: (showId: number) => `${API_BASE_URL}/admin/radio-shows/${showId}`,
  ADMIN_DELETE_SHOW: (showId: number) => `${API_BASE_URL}/admin/radio-shows/${showId}`,
  // Unified ingestion triggers + run poll/cancel (PSY-1135 backend; PSY-1136 FE).
  // Station-scoped sync (discover|fetch) and show-scoped backfill are async: they
  // return a RadioSyncRun handle the UI polls via ADMIN_GET_SYNC_RUN.
  ADMIN_STATION_SYNC: (stationId: number) => `${API_BASE_URL}/admin/radio-stations/${stationId}/sync`,
  ADMIN_SHOW_BACKFILL: (showId: number) => `${API_BASE_URL}/admin/radio-shows/${showId}/backfill`,
  ADMIN_GET_SYNC_RUN: (runId: number) => `${API_BASE_URL}/admin/radio/sync-runs/${runId}`,
  ADMIN_CANCEL_SYNC_RUN: (runId: number) => `${API_BASE_URL}/admin/radio/sync-runs/${runId}/cancel`,
  // Matching
  ADMIN_UNMATCHED: `${API_BASE_URL}/admin/radio/unmatched`,
  ADMIN_LINK_PLAY: (playId: number) => `${API_BASE_URL}/admin/radio/plays/${playId}/link`,
  ADMIN_BULK_LINK: `${API_BASE_URL}/admin/radio/plays/bulk-link`,
}

// ============================================================================
// Types
// ============================================================================

export interface RadioStationListItem {
  id: number
  name: string
  slug: string
  city: string | null
  state: string | null
  country: string | null
  broadcast_type: string
  frequency_mhz: number | null
  logo_url: string | null
  is_active: boolean
  show_count: number
}

export interface RadioStationDetail {
  id: number
  name: string
  slug: string
  description: string | null
  city: string | null
  state: string | null
  country: string | null
  timezone: string | null
  stream_url: string | null
  stream_urls: Record<string, string> | null
  website: string | null
  donation_url: string | null
  donation_embed_url: string | null
  logo_url: string | null
  social: Record<string, string> | null
  broadcast_type: string
  frequency_mhz: number | null
  playlist_source: string | null
  playlist_config: Record<string, unknown> | null
  last_playlist_fetch_at: string | null
  is_active: boolean
  show_count: number
  created_at: string
  updated_at: string
}

export interface RadioShowListItem {
  id: number
  station_id: number
  station_name: string
  name: string
  slug: string
  host_name: string | null
  genre_tags: string[] | null
  image_url: string | null
  is_active: boolean
  // schedule_locked (PSY-1186/1193): true when the schedule is hand-curated and the
  // weekly WFMU scrape leaves it alone. Surfaced as a badge in the admin list.
  schedule_locked: boolean
  episode_count: number
}

export interface RadioShowDetail {
  id: number
  station_id: number
  station_name: string
  station_slug: string
  name: string
  slug: string
  host_name: string | null
  description: string | null
  schedule_display: string | null
  schedule: Record<string, unknown> | null
  schedule_locked: boolean
  genre_tags: string[] | null
  archive_url: string | null
  image_url: string | null
  is_active: boolean
  episode_count: number
  created_at: string
  updated_at: string
}

export interface RadioStats {
  total_stations: number
  total_shows: number
  total_episodes: number
  total_plays: number
  matched_plays: number
  unique_artists: number
}

export interface CreateRadioStationInput {
  name: string
  slug?: string
  description?: string | null
  city?: string | null
  state?: string | null
  country?: string | null
  timezone?: string | null
  stream_url?: string | null
  website?: string | null
  donation_url?: string | null
  logo_url?: string | null
  broadcast_type: string
  frequency_mhz?: number | null
  playlist_source?: string | null
  playlist_config?: Record<string, unknown> | null
}

export interface UpdateRadioStationInput {
  name?: string
  description?: string | null
  city?: string | null
  state?: string | null
  country?: string | null
  timezone?: string | null
  stream_url?: string | null
  website?: string | null
  donation_url?: string | null
  logo_url?: string | null
  broadcast_type?: string
  frequency_mhz?: number | null
  playlist_source?: string | null
  playlist_config?: Record<string, unknown> | null
  is_active?: boolean
}

export interface CreateRadioShowInput {
  name: string
  slug?: string
  host_name?: string | null
  description?: string | null
  schedule_display?: string | null
  genre_tags?: string[] | null
  archive_url?: string | null
  image_url?: string | null
  external_id?: string | null
}

export interface UpdateRadioShowInput {
  name?: string
  host_name?: string | null
  description?: string | null
  schedule_display?: string | null
  genre_tags?: string[] | null
  archive_url?: string | null
  image_url?: string | null
  is_active?: boolean
  // schedule_locked (PSY-1193): true pins the schedule (protect from the weekly WFMU
  // scrape), false resumes auto-scrape. Omitted leaves the current provenance untouched.
  schedule_locked?: boolean
}

// Status of a radio_sync_runs row (PSY-1135). `partial` = the run imported data
// but hit per-episode/match errors (replaces the old import-job "completed with
// errors" error_log header); `skipped` = the circuit breaker was open.
export type RadioSyncRunStatus =
  | 'running'
  | 'success'
  | 'partial'
  | 'failed'
  | 'skipped'
  | 'cancelled'

// One categorized error recorded against a sync run (radio_sync_run_errors).
export interface RadioSyncRunError {
  category: string
  detail?: string | null
  episode_ref?: string | null
}

// A radio_sync_runs row — the unified trace of any ingestion run, returned by the
// station-sync / show-backfill triggers and the run-poll endpoint (PSY-1135).
// Replaces RadioImportJob. show_id/show_name are set only for backfill runs;
// window_start/window_end (YYYY-MM-DD) only for backfill.
export interface RadioSyncRun {
  id: number
  station_id: number
  station_name: string
  show_id?: number | null
  show_name?: string | null
  run_type: 'discover' | 'fetch' | 'backfill'
  trigger: 'scheduled' | 'manual' | 'auto_backfill'
  status: RadioSyncRunStatus
  window_start?: string | null
  window_end?: string | null
  episodes_found: number
  episodes_imported: number
  plays_imported: number
  plays_matched: number
  plays_unmatched: number
  current_episode_date?: string | null
  breaker_skipped: boolean
  errors?: RadioSyncRunError[]
  started_at: string
  finished_at?: string | null
  created_at: string
  updated_at: string
}

// ──────────────────────────────────────────────
// Matching types
// ──────────────────────────────────────────────

export interface SuggestedMatch {
  artist_id: number
  artist_name: string
  artist_slug: string
}

export interface UnmatchedPlayGroup {
  artist_name: string
  play_count: number
  station_names: string[]
  suggested_matches: SuggestedMatch[]
}

export interface BulkLinkResult {
  updated: number
}

// ============================================================================
// Query Hooks
// ============================================================================

/**
 * Hook to list all radio stations (for admin, no filter)
 */
export function useAdminRadioStations() {
  return useQuery({
    queryKey: radioQueryKeys.stations,
    queryFn: async () => {
      const data = await apiRequest<{ stations: RadioStationListItem[]; count: number }>(
        RADIO_ENDPOINTS.STATIONS
      )
      return data
    },
  })
}

/**
 * Hook to get a single radio station's detail
 */
export function useRadioStationDetail(stationId: number, enabled = true) {
  return useQuery({
    queryKey: radioQueryKeys.stationDetail(stationId),
    queryFn: async () => {
      const data = await apiRequest<RadioStationDetail>(
        RADIO_ENDPOINTS.STATION_DETAIL(String(stationId))
      )
      return data
    },
    enabled: enabled && stationId > 0,
  })
}

/**
 * Hook to list radio shows for a station
 */
export function useRadioShows(stationId: number, enabled = true) {
  return useQuery({
    queryKey: radioQueryKeys.shows(stationId),
    queryFn: async () => {
      const data = await apiRequest<{ shows: RadioShowListItem[]; count: number }>(
        `${RADIO_ENDPOINTS.SHOWS}?station_id=${stationId}`
      )
      return data
    },
    enabled: enabled && stationId > 0,
  })
}

/**
 * Hook to get overall radio stats
 */
export function useRadioStats() {
  return useQuery({
    queryKey: radioQueryKeys.stats,
    queryFn: async () => {
      const data = await apiRequest<RadioStats>(RADIO_ENDPOINTS.STATS)
      return data
    },
  })
}

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to create a new radio station
 */
export function useCreateRadioStation() {
  const queryClient = useQueryClient()
  const invalidate = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (input: CreateRadioStationInput) => {
      return apiRequest<RadioStationDetail>(RADIO_ENDPOINTS.ADMIN_CREATE_STATION, {
        method: 'POST',
        body: JSON.stringify(input),
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}

/**
 * Hook to update a radio station
 */
export function useUpdateRadioStation() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ id, ...input }: UpdateRadioStationInput & { id: number }) => {
      return apiRequest<RadioStationDetail>(RADIO_ENDPOINTS.ADMIN_UPDATE_STATION(id), {
        method: 'PUT',
        body: JSON.stringify(input),
      })
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stationDetail(variables.id) })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}

/**
 * Hook to delete a radio station
 */
export function useDeleteRadioStation() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (stationId: number) => {
      return apiRequest<void>(RADIO_ENDPOINTS.ADMIN_DELETE_STATION(stationId), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}

/**
 * Hook to create a radio show for a station
 */
export function useCreateRadioShow() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ stationId, ...input }: CreateRadioShowInput & { stationId: number }) => {
      return apiRequest<RadioShowDetail>(RADIO_ENDPOINTS.ADMIN_CREATE_SHOW(stationId), {
        method: 'POST',
        body: JSON.stringify(input),
      })
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.shows(variables.stationId) })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}

/**
 * Hook to update a radio show
 */
export function useUpdateRadioShow() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ showId, stationId, ...input }: UpdateRadioShowInput & { showId: number; stationId: number }) => {
      return apiRequest<RadioShowDetail>(RADIO_ENDPOINTS.ADMIN_UPDATE_SHOW(showId), {
        method: 'PUT',
        body: JSON.stringify(input),
      })
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.shows(variables.stationId) })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
    },
  })
}

/**
 * Hook to delete a radio show
 */
export function useDeleteRadioShow() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ showId, stationId }: { showId: number; stationId: number }) => {
      return apiRequest<void>(RADIO_ENDPOINTS.ADMIN_DELETE_SHOW(showId), {
        method: 'DELETE',
      })
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.shows(variables.stationId) })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}

// ============================================================================
// Unified sync triggers + run poll/cancel (PSY-1135 backend, PSY-1136 FE)
// ============================================================================

/**
 * Trigger a manual station-scoped sync (discover|fetch). Async: returns the
 * opened RadioSyncRun (status `running`) which the caller polls via useSyncRun.
 * Replaces useFetchPlaylists + useDiscoverShows.
 */
export function useTriggerStationSync() {
  return useMutation({
    mutationFn: async ({
      stationId,
      // 'fetch' is backend-supported but has no manual UI trigger yet — episode
      // fetching runs on the scheduled ticker; the admin UI only triggers
      // 'discover' (PSY-1136). The arm is kept so a future "Fetch now" button can
      // use this same hook.
      mode,
    }: {
      stationId: number
      mode: 'discover' | 'fetch'
    }) => {
      return apiRequest<RadioSyncRun>(RADIO_ENDPOINTS.ADMIN_STATION_SYNC(stationId), {
        method: 'POST',
        body: JSON.stringify({ mode }),
      })
    },
    // The run executes in the background; the discovered shows / fetched episodes
    // don't exist until it reaches a terminal status, so invalidation happens in
    // the consumer when the polled run settles, NOT here on trigger-accepted.
  })
}

/**
 * Trigger a manual historic backfill of one show over [since, until]. Async:
 * returns the opened RadioSyncRun the caller polls via useSyncRun. Replaces
 * useCreateImportJob + useImportShowEpisodes.
 */
export function useTriggerShowBackfill() {
  return useMutation({
    mutationFn: async ({
      showId,
      since,
      until,
    }: {
      showId: number
      since: string
      until: string
    }) => {
      return apiRequest<RadioSyncRun>(RADIO_ENDPOINTS.ADMIN_SHOW_BACKFILL(showId), {
        method: 'POST',
        body: JSON.stringify({ since, until }),
      })
    },
  })
}

/**
 * Poll a single sync run's status. Refetches every 3 seconds while the run is
 * `running`, and STOPS once a terminal status is observed OR a fetch error lands.
 * Replaces useImportJob (now run-id-keyed, no `pending` state).
 *
 * The `query.state.error` guard is load-bearing: without it, a run whose GET
 * persistently errors (e.g. the run was reaped → 404, which the global QueryClient
 * does NOT retry) would poll every 3s forever, because react-query keeps firing
 * refetchInterval in the error state. Transient errors are absorbed by the global
 * retry config before they ever set state.error, so this stops only on a
 * genuinely stuck poll — the consumer surfaces that via the query's isError.
 */
export function useSyncRun(runId: number, enabled = true) {
  return useQuery({
    queryKey: radioQueryKeys.syncRun(runId),
    queryFn: async () => {
      const data = await apiRequest<RadioSyncRun>(RADIO_ENDPOINTS.ADMIN_GET_SYNC_RUN(runId))
      return data
    },
    enabled: enabled && runId > 0,
    refetchInterval: (query) => {
      if (query.state.error) return false
      return query.state.data?.status === 'running' ? 3000 : false
    },
  })
}

/**
 * Cancel a running sync run. Replaces useCancelImportJob.
 */
export function useCancelSyncRun() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (runId: number) => {
      return apiRequest<{ success: boolean }>(RADIO_ENDPOINTS.ADMIN_CANCEL_SYNC_RUN(runId), {
        method: 'POST',
      })
    },
    onSuccess: (_data, runId) => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.syncRun(runId) })
    },
  })
}

// ============================================================================
// Matching Hooks
// ============================================================================

/**
 * Hook to fetch unmatched plays grouped by artist name.
 * Supports station_id filter (0 = all stations) and pagination.
 */
export function useUnmatchedPlays(stationId: number, limit = 50, offset = 0) {
  return useQuery({
    queryKey: [...radioQueryKeys.unmatched(stationId), limit, offset],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (stationId > 0) params.set('station_id', String(stationId))
      params.set('limit', String(limit))
      params.set('offset', String(offset))
      const url = `${RADIO_ENDPOINTS.ADMIN_UNMATCHED}?${params.toString()}`
      return apiRequest<{ groups: UnmatchedPlayGroup[]; total: number }>(url)
    },
  })
}

/**
 * Hook to bulk-link all plays for an artist_name to an artist_id.
 */
export function useBulkLinkPlays() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ artistName, artistId }: { artistName: string; artistId: number }) => {
      return apiRequest<BulkLinkResult>(RADIO_ENDPOINTS.ADMIN_BULK_LINK, {
        method: 'POST',
        body: JSON.stringify({ artist_name: artistName, artist_id: artistId }),
      })
    },
    onSuccess: () => {
      // Invalidate unmatched queries and stats
      queryClient.invalidateQueries({ queryKey: ['radio', 'unmatched'] })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}
