/**
 * Label-related TypeScript types
 *
 * These types match the backend API response structures
 * for label endpoints.
 */

export type LabelStatus = 'active' | 'inactive' | 'defunct'

/** Labels for display */
export const LABEL_STATUS_LABELS: Record<LabelStatus, string> = {
  active: 'Active',
  inactive: 'Inactive',
  defunct: 'Defunct',
}

/** All valid label statuses for filter dropdowns */
export const LABEL_STATUSES: LabelStatus[] = ['active', 'inactive', 'defunct']

/** Badge variant mapping for label statuses */
export function getLabelStatusVariant(
  status: string
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'active':
      return 'default'
    case 'inactive':
      return 'secondary'
    case 'defunct':
      return 'outline'
    default:
      return 'secondary'
  }
}

export interface LabelSocial {
  instagram: string | null
  facebook: string | null
  twitter: string | null
  youtube: string | null
  spotify: string | null
  soundcloud: string | null
  bandcamp: string | null
  website: string | null
}

export interface LabelDetail {
  id: number
  name: string
  slug: string
  city: string | null
  state: string | null
  country: string | null
  founded_year: number | null
  status: string
  description: string | null
  /** Optional label logo URL (PSY-521). */
  image_url?: string | null
  social: LabelSocial
  artist_count: number
  release_count: number
  created_at: string
  updated_at: string
}

export interface LabelListItem {
  id: number
  name: string
  slug: string
  city: string | null
  state: string | null
  status: string
  artist_count: number
  release_count: number
}

export interface LabelsListResponse {
  labels: LabelListItem[]
  count: number
}

/** Simplified label info for artist labels endpoint */
export interface ArtistLabel {
  id: number
  name: string
  slug: string
  city: string | null
  state: string | null
}

export interface ArtistLabelsResponse {
  labels: ArtistLabel[]
  count: number
}

export interface LabelArtist {
  id: number
  slug: string
  name: string
}

export interface LabelArtistsResponse {
  artists: LabelArtist[]
}

export interface LabelRelease {
  id: number
  title: string
  slug: string
  release_type: string
  release_year: number | null
  cover_art_url: string | null
  catalog_number: string | null
}

export interface LabelReleasesResponse {
  releases: LabelRelease[]
}

/**
 * Get a display label for a label status
 */
export function getLabelStatusLabel(status: string): string {
  return LABEL_STATUS_LABELS[status as LabelStatus] ?? status.charAt(0).toUpperCase() + status.slice(1)
}

/**
 * Format a label's location string
 */
export function formatLabelLocation(label: { city: string | null; state: string | null }): string | null {
  if (label.city && label.state) return `${label.city}, ${label.state}`
  if (label.city) return label.city
  if (label.state) return label.state
  return null
}
