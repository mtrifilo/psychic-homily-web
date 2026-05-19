'use client'

import { useQuery } from '@tanstack/react-query'
import { useDebounce } from 'use-debounce'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { artistEndpoints } from '@/features/artists/api'
import { venueEndpoints } from '@/features/venues/api'
import { releaseEndpoints } from '@/features/releases/api'
import { labelEndpoints } from '@/features/labels/api'
import { festivalEndpoints } from '@/features/festivals/api'
import { showEndpoints } from '@/features/shows/api'

// ============================================================================
// Types
// ============================================================================

export interface EntitySearchResult {
  id: number
  slug: string
  name: string
  /** Subtitle info (e.g., city/state, release type, year) */
  subtitle: string | null
  entityType: 'artist' | 'venue' | 'show' | 'release' | 'label' | 'festival' | 'tag'
  href: string
  /**
   * Only populated for tag results — surfaces the curated-tag mark in the
   * Cmd+K palette so users can distinguish official tags at a glance.
   */
  isOfficial?: boolean
}

export interface EntitySearchResults {
  artists: EntitySearchResult[]
  venues: EntitySearchResult[]
  shows: EntitySearchResult[]
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

// PSY-372 / PSY-520: GET /shows/search row shape. Field names mirror
// backend `contracts.ShowSearchResult` exactly (snake_case on the wire).
// `event_date` is an ISO 8601 string per Go's time.Time JSON marshalling.
interface ShowSearchItem {
  id: number
  slug: string
  title: string
  headliner_name: string
  venue_name: string
  event_date: string
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
  is_official: boolean
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

// PSY-372: shows are most recognizable by headliner+venue+date. Most shows
// have auto-generated titles, so we synthesize the full identifier label
// here and put it in `name`. Format mirrors the ticket spec exactly:
// "{Headliner} @ {Venue} · {Date}" (e.g. "Faetooth @ Valley Bar · Apr 15, 2026").
//
// Date formatting: the search row only carries the ISO event_date — there's
// no venue timezone in the payload to localize against, and search labels
// are identification, not show-up-time. Using the user's locale here is
// fine; venue-timezone formatting is reserved for show-detail UIs.
function mapShow(s: ShowSearchItem): EntitySearchResult {
  const date = new Date(s.event_date)
  const dateLabel = Number.isNaN(date.getTime())
    ? ''
    : date.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
      })

  // Each segment is conditionally appended so a missing field (sparse data)
  // doesn't leak orphan separators into the label.
  const parts: string[] = []
  if (s.headliner_name) parts.push(s.headliner_name)
  if (s.venue_name) parts.push(`@ ${s.venue_name}`)
  // Join headliner + venue with a space so we get "Faetooth @ Valley Bar".
  const left = parts.join(' ')
  const label =
    left && dateLabel ? `${left} · ${dateLabel}` : left || dateLabel || s.title

  return {
    id: s.id,
    slug: s.slug,
    name: label,
    subtitle: null,
    entityType: 'show',
    href: `/shows/${s.slug}`,
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
    isOfficial: t.is_official,
  }
}

// ============================================================================
// Query function
// ============================================================================

const MAX_RESULTS_PER_TYPE = 5
const TOTAL_ENDPOINTS = 7

/**
 * Internal shape returned by `fetchEntitySearch`. Carries the mapped results
 * plus an `allFailed` flag so the hook can distinguish a true empty
 * (every endpoint returned empty arrays) from a total outage (every
 * endpoint rejected and we swallowed the rejection per partial-failure
 * resilience). The flag is the only thing consumers need to switch the
 * UI from "no results" copy to "search unavailable" copy.
 */
interface FetchEntitySearchResult {
  results: EntitySearchResults
  allFailed: boolean
}

/**
 * Helper: turn a settled promise result into either the resolved value
 * (so it flows through the existing mapper code unchanged) or the fallback
 * shape when the request rejected. We deliberately don't surface per-request
 * errors — partial failure resilience is intentional. The only signal we
 * preserve is the rejected-count tally, used to detect the all-fail case.
 */
function settledValue<T>(settled: PromiseSettledResult<T>, fallback: T): T {
  return settled.status === 'fulfilled' ? settled.value : fallback
}

async function fetchEntitySearch(query: string): Promise<FetchEntitySearchResult> {
  const encoded = encodeURIComponent(query)

  // Fire all requests in parallel via allSettled so we can tally rejections
  // without losing the partial-failure resilience that .catch(() => …) on
  // Promise.all gave us. Individual rejections still degrade silently to an
  // empty group; the only new behaviour is detecting "every endpoint
  // rejected" so the hook can surface a banner instead of a silent zero.
  const settled = await Promise.allSettled([
    apiRequest<{ artists: ArtistSearchItem[]; count: number }>(
      `${artistEndpoints.SEARCH}?q=${encoded}`
    ),
    apiRequest<{ venues: VenueSearchItem[]; count: number }>(
      `${venueEndpoints.SEARCH}?q=${encoded}`
    ),
    apiRequest<{ shows: ShowSearchItem[]; count: number }>(
      `${showEndpoints.SEARCH}?q=${encoded}`
    ),
    apiRequest<{ releases: ReleaseSearchItem[]; count: number }>(
      `${releaseEndpoints.SEARCH}?q=${encoded}`
    ),
    apiRequest<{ labels: LabelSearchItem[]; count: number }>(
      `${labelEndpoints.SEARCH}?q=${encoded}`
    ),
    apiRequest<{ festivals: FestivalSearchItem[]; count: number }>(
      `${festivalEndpoints.SEARCH}?q=${encoded}`
    ),
    apiRequest<{ tags: TagSearchItem[] }>(
      `${API_ENDPOINTS.TAGS.SEARCH}?q=${encoded}`
    ),
  ])

  const rejectedCount = settled.filter(s => s.status === 'rejected').length
  const allFailed = rejectedCount === TOTAL_ENDPOINTS

  const [artistsS, venuesS, showsS, releasesS, labelsS, festivalsS, tagsS] = settled
  const artists = settledValue(artistsS, { artists: [], count: 0 })
  const venues = settledValue(venuesS, { venues: [], count: 0 })
  const shows = settledValue(showsS, { shows: [], count: 0 })
  const releases = settledValue(releasesS, { releases: [], count: 0 })
  const labels = settledValue(labelsS, { labels: [], count: 0 })
  const festivals = settledValue(festivalsS, { festivals: [], count: 0 })
  const tags = settledValue(tagsS, { tags: [] })

  return {
    results: {
      artists: (artists.artists || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapArtist),
      venues: (venues.venues || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapVenue),
      shows: (shows.shows || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapShow),
      releases: (releases.releases || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapRelease),
      labels: (labels.labels || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapLabel),
      festivals: (festivals.festivals || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapFestival),
      tags: (tags.tags || []).slice(0, MAX_RESULTS_PER_TYPE).map(mapTag),
    },
    allFailed,
  }
}

// ============================================================================
// Hook
// ============================================================================

const EMPTY_RESULTS: EntitySearchResults = {
  artists: [],
  venues: [],
  shows: [],
  releases: [],
  labels: [],
  festivals: [],
  tags: [],
}

const EMPTY_FETCH_RESULT: FetchEntitySearchResult = {
  results: EMPTY_RESULTS,
  allFailed: false,
}

/**
 * Hook for searching entities across all types (artists, venues, shows,
 * releases, labels, festivals, tags). Used by the collection-detail
 * "Add Items" search panel and the Cmd+K command palette.
 *
 * Returns results grouped by entity type, limited to 5 per type, plus
 * a `searchError` flag (PSY-725) that's true only when ALL 7 endpoints
 * rejected in the latest fetch. Consumers use the flag to render a
 * "search unavailable" banner instead of the default "no results"
 * empty state — otherwise a total backend outage would look
 * indistinguishable from a typo.
 *
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
    placeholderData: EMPTY_FETCH_RESULT,
  })

  // Unwrap the internal { results, allFailed } envelope so consumers see
  // the same `data: EntitySearchResults` shape as before — only the
  // `searchError` flag is new.
  const data = result.data?.results ?? EMPTY_RESULTS
  const searchError = result.data?.allFailed ?? false

  const totalResults =
    data.artists.length +
    data.venues.length +
    data.shows.length +
    data.releases.length +
    data.labels.length +
    data.festivals.length +
    data.tags.length

  return {
    ...result,
    data,
    totalResults,
    isSearching: isQueryLongEnough && result.isFetching,
    searchError,
  }
}
