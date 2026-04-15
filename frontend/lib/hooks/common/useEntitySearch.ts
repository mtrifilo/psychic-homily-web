'use client'

import { useQuery } from '@tanstack/react-query'
import { useDebounce } from 'use-debounce'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { artistEndpoints } from '@/features/artists/api'
import { venueEndpoints } from '@/features/venues/api'
import { releaseEndpoints } from '@/features/releases/api'
import { labelEndpoints } from '@/features/labels/api'
import { festivalEndpoints } from '@/features/festivals/api'

// ============================================================================
// Types
// ============================================================================

export interface EntitySearchResult {
  id: number
  slug: string
  name: string
  /** Subtitle info (e.g., city/state, release type, year) */
  subtitle: string | null
  entityType: 'artist' | 'venue' | 'release' | 'label' | 'festival' | 'tag'
  href: string
}

export interface EntitySearchResults {
  artists: EntitySearchResult[]
  venues: EntitySearchResult[]
  releases: EntitySearchResult[]
  labels: EntitySearchResult[]
  festivals: EntitySearchResult[]
  tags: EntitySearchResult[]
}

// Response shapes from the backend search endpoints
interface ArtistSearchItem {
  id: number
  slug: string
  name: string
  city?: string | null
  state?: string | null
}

interface VenueSearchItem {
  id: number
  slug: string
  name: string
  city?: string
  state?: string
}

interface ReleaseSearchItem {
  id: number
  slug: string
  title: string
  release_type?: string
  release_year?: number | null
}

interface LabelSearchItem {
  id: number
  slug: string
  name: string
  city?: string | null
  state?: string | null
}

interface FestivalSearchItem {
  id: number
  slug: string
  name: string
  city?: string | null
  state?: string | null
  edition_year?: number
}

interface TagSearchItem {
  id: number
  slug: string
  name: string
  category: string
  usage_count: number
}

// ============================================================================
// Mappers
// ============================================================================

function mapArtist(a: ArtistSearchItem): EntitySearchResult {
  const parts: string[] = []
  if (a.city && a.state) parts.push(`${a.city}, ${a.state}`)
  else if (a.city) parts.push(a.city)
  else if (a.state) parts.push(a.state)

  return {
    id: a.id,
    slug: a.slug,
    name: a.name,
    subtitle: parts.length > 0 ? parts.join(' ') : null,
    entityType: 'artist',
    href: `/artists/${a.slug}`,
  }
}

function mapVenue(v: VenueSearchItem): EntitySearchResult {
  const loc = v.city && v.state ? `${v.city}, ${v.state}` : v.city || v.state || null
  return {
    id: v.id,
    slug: v.slug,
    name: v.name,
    subtitle: loc,
    entityType: 'venue',
    href: `/venues/${v.slug}`,
  }
}

function mapRelease(r: ReleaseSearchItem): EntitySearchResult {
  const parts: string[] = []
  if (r.release_type) parts.push(r.release_type)
  if (r.release_year) parts.push(String(r.release_year))
  return {
    id: r.id,
    slug: r.slug,
    name: r.title,
    subtitle: parts.length > 0 ? parts.join(' · ') : null,
    entityType: 'release',
    href: `/releases/${r.slug}`,
  }
}

function mapLabel(l: LabelSearchItem): EntitySearchResult {
  const loc = l.city && l.state ? `${l.city}, ${l.state}` : l.city || l.state || null
  return {
    id: l.id,
    slug: l.slug,
    name: l.name,
    subtitle: loc,
    entityType: 'label',
    href: `/labels/${l.slug}`,
  }
}

function mapFestival(f: FestivalSearchItem): EntitySearchResult {
  const parts: string[] = []
  if (f.city && f.state) parts.push(`${f.city}, ${f.state}`)
  if (f.edition_year) parts.push(String(f.edition_year))
  return {
    id: f.id,
    slug: f.slug,
    name: f.name,
    subtitle: parts.length > 0 ? parts.join(' · ') : null,
    entityType: 'festival',
    href: `/festivals/${f.slug}`,
  }
}

function mapTag(t: TagSearchItem): EntitySearchResult {
  const category = t.category.charAt(0).toUpperCase() + t.category.slice(1)
  return {
    id: t.id,
    slug: t.slug,
    name: t.name,
    subtitle: category,
    entityType: 'tag',
    href: `/tags/${t.slug}`,
  }
}

// ============================================================================
// Query function
// ============================================================================

const MAX_RESULTS_PER_TYPE = 5

async function fetchEntitySearch(query: string): Promise<EntitySearchResults> {
  const encoded = encodeURIComponent(query)

  // Fire all requests in parallel; if individual ones fail, return empty arrays
  const [artists, venues, releases, labels, festivals, tags] = await Promise.all([
    apiRequest<{ artists: ArtistSearchItem[]; count: number }>(
      `${artistEndpoints.SEARCH}?q=${encoded}`
    ).catch(() => ({ artists: [], count: 0 })),
    apiRequest<{ venues: VenueSearchItem[]; count: number }>(
      `${venueEndpoints.SEARCH}?q=${encoded}`
    ).catch(() => ({ venues: [], count: 0 })),
    apiRequest<{ releases: ReleaseSearchItem[]; count: number }>(
      `${releaseEndpoints.SEARCH}?q=${encoded}`
    ).catch(() => ({ releases: [], count: 0 })),
    apiRequest<{ labels: LabelSearchItem[]; count: number }>(
      `${labelEndpoints.SEARCH}?q=${encoded}`
    ).catch(() => ({ labels: [], count: 0 })),
    apiRequest<{ festivals: FestivalSearchItem[]; count: number }>(
      `${festivalEndpoints.SEARCH}?q=${encoded}`
    ).catch(() => ({ festivals: [], count: 0 })),
    apiRequest<{ tags: TagSearchItem[] }>(
      `${API_ENDPOINTS.TAGS.SEARCH}?q=${encoded}`
    ).catch(() => ({ tags: [] })),
  ])

  return {
    artists: (artists.artists || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapArtist),
    venues: (venues.venues || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapVenue),
    releases: (releases.releases || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapRelease),
    labels: (labels.labels || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapLabel),
    festivals: (festivals.festivals || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapFestival),
    tags: (tags.tags || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapTag),
  }
}

// ============================================================================
// Hook
// ============================================================================

const EMPTY_RESULTS: EntitySearchResults = {
  artists: [],
  venues: [],
  releases: [],
  labels: [],
  festivals: [],
  tags: [],
}

/**
 * Hook for searching entities across all types (artists, venues, releases, labels, festivals, tags).
 * Used in the Cmd+K command palette to provide entity results alongside page navigation.
 *
 * Returns results grouped by entity type, limited to 5 per type.
 * Debounces input by default (300ms) and requires at least 2 characters.
 */
export function useEntitySearch(options: {
  query: string
  enabled?: boolean
  debounceMs?: number
}) {
  const { query, enabled = true, debounceMs = 300 } = options
  const [debouncedQuery] = useDebounce(query.trim(), debounceMs)

  const isQueryLongEnough = debouncedQuery.length >= 2

  const result = useQuery({
    queryKey: ['entity-search', debouncedQuery],
    queryFn: () => fetchEntitySearch(debouncedQuery),
    enabled: enabled && isQueryLongEnough,
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    placeholderData: EMPTY_RESULTS,
  })

  const totalResults =
    (result.data?.artists.length ?? 0) +
    (result.data?.venues.length ?? 0) +
    (result.data?.releases.length ?? 0) +
    (result.data?.labels.length ?? 0) +
    (result.data?.festivals.length ?? 0) +
    (result.data?.tags.length ?? 0)

  return {
    ...result,
    totalResults,
    isSearching: isQueryLongEnough && result.isFetching,
  }
}
