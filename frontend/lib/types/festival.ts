/**
 * Festival-related TypeScript types
 *
 * These types match the backend API response structures
 * for festival endpoints.
 */

export type FestivalStatus = 'announced' | 'confirmed' | 'cancelled' | 'completed'

/** Labels for display */
export const FESTIVAL_STATUS_LABELS: Record<FestivalStatus, string> = {
  announced: 'Announced',
  confirmed: 'Confirmed',
  cancelled: 'Cancelled',
  completed: 'Completed',
}

/** All valid festival statuses for filter dropdowns */
export const FESTIVAL_STATUSES: FestivalStatus[] = [
  'announced',
  'confirmed',
  'cancelled',
  'completed',
]

/** Badge variant mapping for festival statuses */
export function getFestivalStatusVariant(
  status: string
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'confirmed':
      return 'default'
    case 'announced':
      return 'secondary'
    case 'cancelled':
      return 'destructive'
    case 'completed':
      return 'outline'
    default:
      return 'secondary'
  }
}

/**
 * Get a display label for a festival status
 */
export function getFestivalStatusLabel(status: string): string {
  return (
    FESTIVAL_STATUS_LABELS[status as FestivalStatus] ??
    status.charAt(0).toUpperCase() + status.slice(1)
  )
}

export type BillingTier =
  | 'headliner'
  | 'sub_headliner'
  | 'mid_card'
  | 'undercard'
  | 'local'
  | 'dj'
  | 'host'

/** Labels for display */
export const BILLING_TIER_LABELS: Record<BillingTier, string> = {
  headliner: 'Headliner',
  sub_headliner: 'Sub-Headliner',
  mid_card: 'Mid Card',
  undercard: 'Undercard',
  local: 'Local',
  dj: 'DJ',
  host: 'Host',
}

/** All valid billing tiers for dropdowns */
export const BILLING_TIERS: BillingTier[] = [
  'headliner',
  'sub_headliner',
  'mid_card',
  'undercard',
  'local',
  'dj',
  'host',
]

/**
 * Get a display label for a billing tier
 */
export function getBillingTierLabel(tier: string): string {
  return (
    BILLING_TIER_LABELS[tier as BillingTier] ??
    tier
      .split('_')
      .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
      .join(' ')
  )
}

export interface FestivalSocial {
  instagram?: string | null
  facebook?: string | null
  twitter?: string | null
  youtube?: string | null
  spotify?: string | null
  website?: string | null
}

export interface FestivalDetail {
  id: number
  name: string
  slug: string
  series_slug: string
  edition_year: number
  description: string | null
  location_name: string | null
  city: string | null
  state: string | null
  country: string | null
  start_date: string
  end_date: string
  website: string | null
  ticket_url: string | null
  flyer_url: string | null
  status: string
  social: FestivalSocial | null
  artist_count: number
  venue_count: number
  created_at: string
  updated_at: string
}

export interface FestivalListItem {
  id: number
  name: string
  slug: string
  series_slug: string
  edition_year: number
  city: string | null
  state: string | null
  start_date: string
  end_date: string
  status: string
  artist_count: number
  venue_count: number
}

export interface FestivalsListResponse {
  festivals: FestivalListItem[]
  count: number
}

export interface FestivalArtist {
  id: number
  artist_id: number
  artist_slug: string
  artist_name: string
  billing_tier: string
  position: number
  day_date: string | null
  stage: string | null
  set_time: string | null
  venue_id: number | null
}

export interface FestivalArtistsResponse {
  artists: FestivalArtist[]
  count: number
}

export interface FestivalVenue {
  id: number
  venue_id: number
  venue_name: string
  venue_slug: string
  city: string
  state: string
  is_primary: boolean
}

export interface FestivalVenuesResponse {
  venues: FestivalVenue[]
  count: number
}

export interface ArtistFestivalListItem extends FestivalListItem {
  billing_tier: string
  day_date: string | null
  stage: string | null
}

export interface ArtistFestivalsResponse {
  festivals: ArtistFestivalListItem[]
  count: number
}

/**
 * Format a festival's location string
 */
export function formatFestivalLocation(festival: {
  city: string | null
  state: string | null
}): string | null {
  if (festival.city && festival.state) return `${festival.city}, ${festival.state}`
  if (festival.city) return festival.city
  if (festival.state) return festival.state
  return null
}

/**
 * Format a festival's date range string
 */
export function formatFestivalDates(startDate: string, endDate: string): string {
  try {
    // Parse as local dates (YYYY-MM-DD format, avoid timezone issues)
    const [startYear, startMonth, startDay] = startDate.split('-').map(Number)
    const [endYear, endMonth, endDay] = endDate.split('-').map(Number)
    const start = new Date(startYear, startMonth - 1, startDay)
    const end = new Date(endYear, endMonth - 1, endDay)

    const startStr = start.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
    })
    const endStr = end.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    })
    return `${startStr} - ${endStr}`
  } catch {
    return `${startDate} - ${endDate}`
  }
}
