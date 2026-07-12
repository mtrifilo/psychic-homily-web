/**
 * Releases API Configuration
 *
 * Co-located endpoint definitions and query keys for the releases feature.
 * Imported by release hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const releaseEndpoints = {
  LIST: `${API_BASE_URL}/releases`,
  SEARCH: `${API_BASE_URL}/releases/search`,
  GET: (idOrSlug: string | number) => `${API_BASE_URL}/releases/${idOrSlug}`,
  CREATE: `${API_BASE_URL}/releases`,
  UPDATE: (releaseId: string | number) =>
    `${API_BASE_URL}/releases/${releaseId}`,
  DELETE: (releaseId: string | number) =>
    `${API_BASE_URL}/releases/${releaseId}`,
  ADD_LINK: (releaseId: string | number) =>
    `${API_BASE_URL}/releases/${releaseId}/links`,
  REMOVE_LINK: (releaseId: string | number, linkId: string | number) =>
    `${API_BASE_URL}/releases/${releaseId}/links/${linkId}`,
  ARTIST_RELEASES: (artistIdOrSlug: string | number) =>
    `${API_BASE_URL}/artists/${artistIdOrSlug}/releases`,
  SAVED_LIST: `${API_BASE_URL}/saved-releases`,
  SAVE: (releaseId: string | number) =>
    `${API_BASE_URL}/saved-releases/${releaseId}`,
  UNSAVE: (releaseId: string | number) =>
    `${API_BASE_URL}/saved-releases/${releaseId}`,
  SAVE_COUNT: (releaseId: string | number) =>
    `${API_BASE_URL}/releases/${releaseId}/saves`,
  SAVE_COUNTS_BATCH: `${API_BASE_URL}/releases/saves/batch`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const releaseQueryKeys = {
  all: ['releases'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['releases', 'list', filters] as const,
  search: (query: string) =>
    ['releases', 'search', query.toLowerCase()] as const,
  detail: (idOrSlug: string | number) =>
    ['releases', 'detail', String(idOrSlug)] as const,
  artistReleases: (artistIdOrSlug: string | number) =>
    ['releases', 'artist', String(artistIdOrSlug)] as const,
  savedList: (limit: number, offset: number) =>
    ['releases', 'saved', 'list', limit, offset] as const,
  saveCount: (releaseId: number, isAuthenticated: boolean) =>
    ['releases', 'save-state', 'count', isAuthenticated, releaseId] as const,
  saveCountBatchPrefix: ['releases', 'save-state', 'batch'] as const,
  saveCountBatch: (releaseIds: number[], isAuthenticated: boolean) =>
    ['releases', 'save-state', 'batch', isAuthenticated, releaseIds] as const,
} as const
