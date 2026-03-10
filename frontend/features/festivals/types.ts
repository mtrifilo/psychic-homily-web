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

/** Ordered billing tiers for display (alias for BILLING_TIERS) */
export const BILLING_TIER_ORDER = BILLING_TIERS

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

/** Alias for backward compatibility */
export type ArtistFestival = ArtistFestivalListItem

export interface ArtistFestivalsResponse {
  festivals: ArtistFestivalListItem[]
  count: number
}

/**
 * Format a festival's location string
 */
export function formatFestivalLocation(festival: {
  location_name?: string | null
  city: string | null
  state: string | null
}): string | null {
  const parts: string[] = []
  if (festival.location_name) parts.push(festival.location_name)
  if (festival.city && festival.state) {
    parts.push(`${festival.city}, ${festival.state}`)
  } else if (festival.city) {
    parts.push(festival.city)
  } else if (festival.state) {
    parts.push(festival.state)
  }
  return parts.length > 0 ? parts.join(' — ') : null
}

/**
 * Format festival date range for display
 */
export function formatFestivalDateRange(startDate: string, endDate: string): string {
  const start = new Date(startDate + 'T00:00:00')
  const end = new Date(endDate + 'T00:00:00')

  const startMonth = start.toLocaleDateString('en-US', { month: 'short' })
  const startDay = start.getDate()
  const endMonth = end.toLocaleDateString('en-US', { month: 'short' })
  const endDay = end.getDate()
  const year = start.getFullYear()

  if (startDate === endDate) {
    return `${startMonth} ${startDay}, ${year}`
  }

  if (startMonth === endMonth) {
    return `${startMonth} ${startDay}–${endDay}, ${year}`
  }

  return `${startMonth} ${startDay} – ${endMonth} ${endDay}, ${year}`
}

/** Alias for backward compatibility */
export const formatFestivalDates = formatFestivalDateRange
