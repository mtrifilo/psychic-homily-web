import { useQuery } from '@tanstack/react-query'
import { queryKeys } from '../queryKeys'
import { exportShows, exportArtists, exportVenues } from '../api'
import type {
  ExportShowsResult,
  ExportArtistsResult,
  ExportVenuesResult,
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
