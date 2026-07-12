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

/**
 * Platforms offered when adding an external link to a release. Shared by the
 * admin release editor (`features/releases/admin/ReleaseManagement.tsx`) and
 * the user-facing add-link dialog (`AddReleaseLinkDialog`, PSY-660) so the two
 * surfaces never drift apart. Keep in sync with the backend's accepted
 * platform values.
 */
export const EXTERNAL_LINK_PLATFORMS = [
  { value: 'bandcamp', label: 'Bandcamp' },
  { value: 'spotify', label: 'Spotify' },
  { value: 'apple_music', label: 'Apple Music' },
  { value: 'youtube', label: 'YouTube' },
  { value: 'discogs', label: 'Discogs' },
  { value: 'tidal', label: 'Tidal' },
  { value: 'soundcloud', label: 'SoundCloud' },
] as const

export interface ReleaseLabel {
  id: number
  name: string
  slug: string
  catalog_number?: string | null
}

export interface ReleaseDetail {
  id: number
  title: string
  slug: string
  release_type: string
  release_year: number | null
  release_date: string | null
  cover_art_url: string | null
  cover_art_source: string | null
  cover_art_source_url: string | null
  description: string | null
  artists: ReleaseArtist[]
  labels: ReleaseLabel[]
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
  total: number
  limit: number
  offset: number
}

export interface SavedReleaseResponse extends ReleaseListItem {
  saved_at: string
}

export interface SavedReleasesListResponse {
  releases: SavedReleaseResponse[]
  total: number
  limit: number
  offset: number
}

export interface ReleaseSaveResponse {
  success: boolean
  message: string
}

export interface ReleaseSaveCount {
  release_id: number
  save_count: number
  is_saved: boolean
}

export interface ReleaseSaveCountEntry {
  save_count: number
  is_saved: boolean
}

export interface BatchReleaseSaveCountsResponse {
  saves: Record<string, ReleaseSaveCountEntry>
}

/** Sort options for the releases browse page */
export type ReleaseSortOption =
  | 'newest'
  | 'oldest'
  | 'title_asc'
  | 'title_desc'
  | 'recently_added'

export const RELEASE_SORT_OPTIONS: {
  value: ReleaseSortOption
  label: string
}[] = [
  { value: 'newest', label: 'Newest First' },
  { value: 'oldest', label: 'Oldest First' },
  { value: 'title_asc', label: 'Title A-Z' },
  { value: 'title_desc', label: 'Title Z-A' },
  { value: 'recently_added', label: 'Recently Added' },
]

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
