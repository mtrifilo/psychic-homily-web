import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { queryKeys } from '../queryKeys'
import { exportShows, exportArtists, exportVenues, fetchRemoteShows } from '../api'
import type {
  ExportShowsResult,
  ExportArtistsResult,
  ExportVenuesResult,
  AppSettings,
} from '../types'

export interface UseExportShowsParams {
  limit?: number
  offset?: number
  status?: string
  fromDate?: string
  city?: string
  state?: string
}

export function useExportShows(params: UseExportShowsParams, enabled = true) {
  return useQuery<ExportShowsResult, Error>({
    queryKey: queryKeys.export.shows(params),
    queryFn: () => exportShows(params),
    enabled,
  })
}

export interface UseExportArtistsParams {
  limit?: number
  offset?: number
  search?: string
}

export function useExportArtists(params: UseExportArtistsParams, enabled = true) {
  return useQuery<ExportArtistsResult, Error>({
    queryKey: queryKeys.export.artists(params),
    queryFn: () => exportArtists(params),
    enabled,
  })
}

export interface UseExportVenuesParams {
  limit?: number
  offset?: number
  search?: string
  verified?: string
  city?: string
  state?: string
}

export function useExportVenues(params: UseExportVenuesParams, enabled = true) {
  return useQuery<ExportVenuesResult, Error>({
    queryKey: queryKeys.export.venues(params),
    queryFn: () => exportVenues(params),
    enabled,
  })
}

// Check which shows exist on Stage and Production
export function useRemoteShowExistence(settings: AppSettings, enabled: boolean) {
  const params = { limit: 500, status: 'all' }
  const hasStageToken = Boolean(settings.stageToken?.length)
  const hasProductionToken = Boolean(settings.productionToken?.length)

  const stageQuery = useQuery<ExportShowsResult, Error>({
    queryKey: queryKeys.remote.shows('stage', params),
    queryFn: () => fetchRemoteShows(settings.stageUrl, settings.stageToken, params),
    enabled: enabled && hasStageToken,
    staleTime: 5 * 60 * 1000,
  })

  const productionQuery = useQuery<ExportShowsResult, Error>({
    queryKey: queryKeys.remote.shows('production', params),
    queryFn: () => fetchRemoteShows(settings.productionUrl, settings.productionToken, params),
    enabled: enabled && hasProductionToken,
    staleTime: 5 * 60 * 1000,
  })

  const stageShowIds = useMemo(() => {
    const set = new Set<string>()
    for (const show of stageQuery.data?.shows ?? []) {
      set.add(`${show.title}-${show.eventDate}`)
    }
    return set
  }, [stageQuery.data?.shows])

  const productionShowIds = useMemo(() => {
    const set = new Set<string>()
    for (const show of productionQuery.data?.shows ?? []) {
      set.add(`${show.title}-${show.eventDate}`)
    }
    return set
  }, [productionQuery.data?.shows])

  return {
    stageShowIds,
    productionShowIds,
    isLoading: stageQuery.isLoading || productionQuery.isLoading,
  }
}
