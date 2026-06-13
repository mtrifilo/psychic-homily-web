/**
 * Station-overview derivation helpers (PSY-1016; consumed by the Dial
 * surfaces since PSY-1049/1050).
 *
 * Pure, hook-free transforms that turn the radio API responses into the
 * shapes the station surfaces render. The "Now Playing" derivation that
 * originally lived here was superseded by the live now-playing endpoint
 * (PSY-1022) and removed (PSY-1075).
 */

import type { RadioShowListItem } from '../types'

/**
 * A single artist hop (name + optional graph link). `slug` is null when the
 * matching engine hasn't linked the play to a catalog artist yet — the panel
 * renders those as plain text rather than a dead link.
 */
export interface ArtistHop {
  name: string
  slug: string | null
}

/**
 * Pick the station's signature show. Shows have no recency field on the list
 * endpoint (they sort name-ASC), so this uses episode_count as the proxy for
 * "the station's active / signature show" — the show with the most logged
 * episodes. Ties break on the lower id (stable, deterministic). Returns null
 * for a station with no shows.
 *
 * The live on-air lines use PSY-1022's now-playing endpoint instead; this
 * heuristic survives only to anchor the Dial strip's [ live playlist ]
 * archive deep-link (useStationOverview).
 */
export function pickNowPlayingShow(
  shows: RadioShowListItem[] | undefined
): RadioShowListItem | null {
  if (!shows || shows.length === 0) return null
  return shows.reduce((best, show) => {
    if (show.episode_count > best.episode_count) return show
    if (show.episode_count === best.episode_count && show.id < best.id) return show
    return best
  })
}

/**
 * Format a YYYY-MM-DD air-date as a short "Jun 4" (no year), the dense
 * editorial register the radio surfaces share. Parses at local midnight so a date-only string doesn't shift a day
 * in negative-offset timezones.
 */
export function formatShortAirDate(dateStr: string | null | undefined): string {
  if (!dateStr) return ''
  const date = new Date(dateStr + 'T00:00:00')
  if (isNaN(date.getTime())) return ''
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

/**
 * The single-station identity sub-line: "Seattle, WA" / "London, UK" etc.
 * Drops empty parts; returns "" when no location is known.
 */
export function formatStationLocation(
  city: string | null | undefined,
  state: string | null | undefined
): string {
  return [city, state].filter(Boolean).join(', ')
}
