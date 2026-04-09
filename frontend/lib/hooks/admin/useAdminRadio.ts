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
  ADMIN_FETCH_PLAYLISTS: (stationId: number) => `${API_BASE_URL}/admin/radio-stations/${stationId}/fetch`,
  ADMIN_DISCOVER_SHOWS: (stationId: number) => `${API_BASE_URL}/admin/radio-stations/${stationId}/discover`,
  ADMIN_IMPORT_SHOW_EPISODES: (showId: number) => `${API_BASE_URL}/admin/radio-shows/${showId}/import`,
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
}

export interface RadioDiscoverResult {
  shows_discovered: number
  show_names: string[]
  errors?: string[]
}

export interface RadioImportResult {
  shows_discovered: number
  episodes_imported: number
  plays_imported: number
  plays_matched: number
  errors?: string[]
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

/**
 * Hook to trigger playlist fetch for a station
 */
export function useFetchPlaylists() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (stationId: number) => {
      return apiRequest<void>(RADIO_ENDPOINTS.ADMIN_FETCH_PLAYLISTS(stationId), {
        method: 'POST',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}

/**
 * Hook to discover shows for a station
 */
export function useDiscoverShows() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (stationId: number) => {
      return apiRequest<RadioDiscoverResult>(RADIO_ENDPOINTS.ADMIN_DISCOVER_SHOWS(stationId), {
        method: 'POST',
      })
    },
    onSuccess: (_data, stationId) => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.shows(stationId) })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}

/**
 * Hook to import episodes for a specific radio show
 */
export function useImportShowEpisodes() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ showId, since, until }: { showId: number; since: string; until: string }) => {
      return apiRequest<RadioImportResult>(RADIO_ENDPOINTS.ADMIN_IMPORT_SHOW_EPISODES(showId), {
        method: 'POST',
        body: JSON.stringify({ since, until }),
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stations })
      queryClient.invalidateQueries({ queryKey: radioQueryKeys.stats })
    },
  })
}
