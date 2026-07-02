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

/** The one definition of the "Jul 1" short-date rendering both paths share. */
function shortDate(date: Date): string {
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

/**
 * Viewer-local date line for a feed row (PSY-1298): when the frozen air
 * window exists, the date derives from starts_at in the VIEWER's timezone
 * (fully viewer-local — an 11 PM ET Tuesday broadcast reads as Wednesday in
 * Berlin, locked design decision); windowless rows fall back to the station
 * air_date, date-only.
 */
export function formatLocalAirDate(
  startsAt: string | null | undefined,
  airDate: string | null | undefined
): string {
  if (startsAt) {
    const date = new Date(startsAt)
    if (!isNaN(date.getTime())) {
      return shortDate(date)
    }
  }
  return formatShortAirDate(airDate)
}

/**
 * One end of the air window as compact 12h: drop :00 minutes ("9", "6:30",
 * "12" for noon/midnight), meridiem carried separately so the range renderer
 * decides whether to show it once or twice.
 */
function formatCompactTime(date: Date): { clock: string; meridiem: string } {
  const hours24 = date.getHours()
  const minutes = date.getMinutes()
  const meridiem = hours24 < 12 ? 'AM' : 'PM'
  const hours12 = hours24 % 12 === 0 ? 12 : hours24 % 12
  const clock =
    minutes === 0 ? `${hours12}` : `${hours12}:${String(minutes).padStart(2, '0')}`
  return { clock, meridiem }
}

/** A frozen air window longer than this is corrupt data, not a radio slot. */
const MAX_WINDOW_MS = 24 * 60 * 60 * 1000

/**
 * Viewer-local air-time block (PSY-1298): "3–6 PM", "6:30–9 PM",
 * "9 PM–12 AM" — compact 12h, minutes only when non-zero, single AM/PM
 * suffix when both ends share it (a range crossing noon or midnight always
 * carries both, so "9–12 PM" is deliberately never produced — it would be
 * ambiguous). Returns '' for a windowless row (the date-only rendering is
 * the designed fallback), an unparsable window, or a degenerate one
 * (inverted / ≥24h) — corrupt data must not render as a confident range.
 */
export function formatLocalTimeRange(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined
): string {
  if (!startsAt || !endsAt) return ''
  const start = new Date(startsAt)
  const end = new Date(endsAt)
  if (isNaN(start.getTime()) || isNaN(end.getTime())) return ''
  const span = end.getTime() - start.getTime()
  if (span <= 0 || span >= MAX_WINDOW_MS) return ''
  const s = formatCompactTime(start)
  const e = formatCompactTime(end)
  const startText = s.meridiem === e.meridiem ? s.clock : `${s.clock} ${s.meridiem}`
  return `${startText}–${e.clock} ${e.meridiem}`
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
