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
  GET: (idOrSlug: string | number) => `${API_BASE_URL}/releases/${idOrSlug}`,
  CREATE: `${API_BASE_URL}/releases`,
  UPDATE: (releaseId: string | number) => `${API_BASE_URL}/releases/${releaseId}`,
  DELETE: (releaseId: string | number) => `${API_BASE_URL}/releases/${releaseId}`,
  ADD_LINK: (releaseId: string | number) =>
    `${API_BASE_URL}/releases/${releaseId}/links`,
  REMOVE_LINK: (releaseId: string | number, linkId: string | number) =>
    `${API_BASE_URL}/releases/${releaseId}/links/${linkId}`,
  ARTIST_RELEASES: (artistIdOrSlug: string | number) =>
    `${API_BASE_URL}/artists/${artistIdOrSlug}/releases`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const releaseQueryKeys = {
  all: ['releases'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['releases', 'list', filters] as const,
  detail: (idOrSlug: string | number) => ['releases', 'detail', String(idOrSlug)] as const,
  artistReleases: (artistIdOrSlug: string | number) =>
    ['releases', 'artist', String(artistIdOrSlug)] as const,
} as const
