/**
 * Release-related TypeScript types
 *
 * These types match the backend API response structures
 * for release endpoints.
 */

export type ReleaseType =
  | 'lp'
  | 'ep'
  | 'single'
  | 'compilation'
  | 'live'
  | 'remix'
  | 'demo'

/** Labels for display */
export const RELEASE_TYPE_LABELS: Record<ReleaseType, string> = {
  lp: 'LP',
  ep: 'EP',
  single: 'Single',
  compilation: 'Compilation',
  live: 'Live',
  remix: 'Remix',
  demo: 'Demo',
}

/** All valid release types for filter dropdowns */
export const RELEASE_TYPES: ReleaseType[] = [
  'lp',
  'ep',
  'single',
  'compilation',
  'live',
  'remix',
  'demo',
]

export interface ReleaseArtist {
  id: number
  slug: string
  name: string
  role: string
}

export interface ReleaseExternalLink {
  id: number
  platform: string
  url: string
}

export interface ReleaseDetail {
  id: number
  title: string
  slug: string
  release_type: string
  release_year: number | null
  release_date: string | null
  cover_art_url: string | null
  description: string | null
  artists: ReleaseArtist[]
  external_links: ReleaseExternalLink[]
  created_at: string
  updated_at: string
}

export interface ReleaseListArtist {
  id: number
  name: string
  slug: string
}

export interface ReleaseListItem {
  id: number
  title: string
  slug: string
  release_type: string
  release_year: number | null
  cover_art_url: string | null
  artist_count: number
  artists: ReleaseListArtist[]
  label_name: string | null
  label_slug: string | null
}

export interface ReleasesListResponse {
  releases: ReleaseListItem[]
  count: number
}

/** Release with the artist's role, returned from GET /artists/:id/releases */
export interface ArtistReleaseListItem extends ReleaseListItem {
  role: string
}

export interface ArtistReleasesResponse {
  releases: ArtistReleaseListItem[]
  count: number
}

/**
 * Get a display label for a release type
 */
export function getReleaseTypeLabel(type: string): string {
  return RELEASE_TYPE_LABELS[type as ReleaseType] ?? type.toUpperCase()
}
