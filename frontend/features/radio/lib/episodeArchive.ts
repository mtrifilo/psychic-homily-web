/**
 * Episode-archive derivation helpers (PSY-1051, radio show + playlist pages).
 *
 * Pure, hook-free transforms for the show page's playlist-archive table and
 * the playlist page's dense track table. Kept out of the components so the
 * ON AIR heuristic, the matched-artist stats, and the prev/next neighbor
 * walk are each unit-testable seams.
 */

import type {
  RadioEpisodeListItem,
  RadioEpisodePreviewArtist,
  RadioEpisodesListResponse,
  RadioPlay,
} from '../types'
import type { ArtistHop } from './stationOverview'

/**
 * v1 "on air" heuristic for a single show (PSY-1051, same register as the
 * PSY-1016 station-overview fallback): the show is treated as live when its
 * most-recent episode aired TODAY in the viewer's local date. Honest enough
 * until PSY-1022 ships a real live now-playing signal — the ON AIR strip is
 * built to swap to that endpoint without layout change.
 */
export function isAirDateToday(
  airDate: string | null | undefined,
  now: Date = new Date()
): boolean {
  if (!airDate) return false
  const today = [
    now.getFullYear(),
    String(now.getMonth() + 1).padStart(2, '0'),
    String(now.getDate()).padStart(2, '0'),
  ].join('-')
  return airDate === today
}

/**
 * Map an episode row's artist preview (PSY-1048) onto the ArtistHops shape so
 * matched artists render as graph links and unmatched names as plain text.
 */
export function previewToHops(
  preview: RadioEpisodePreviewArtist[] | null | undefined
): ArtistHop[] {
  return (preview ?? []).map(p => ({ name: p.artist_name, slug: p.artist_slug }))
}

export interface ArtistMatchStats {
  /** Distinct artist names with a knowledge-graph match (artist_id). */
  matched: number
  /** Total distinct artist names in the playlist. */
  total: number
}

/**
 * "N of M artists matched to the graph" for a playlist's meta line. Distinct
 * by artist name (case-insensitive, trimmed — same dedup rule as
 * recentArtistsFromEpisode); a name counts as matched when any of its plays
 * carries an artist_id.
 */
export function computeArtistMatchStats(
  plays: RadioPlay[] | null | undefined
): ArtistMatchStats {
  const matchedByName = new Map<string, boolean>()
  for (const play of plays ?? []) {
    if (!play.artist_name) continue
    const key = play.artist_name.trim().toLowerCase()
    matchedByName.set(key, (matchedByName.get(key) ?? false) || play.artist_id != null)
  }
  let matched = 0
  for (const isMatched of matchedByName.values()) {
    if (isMatched) matched++
  }
  return { matched, total: matchedByName.size }
}

/**
 * TIME cell for the playlist table: "9:02 PM" from an ISO air_timestamp,
 * or null when the feed didn't carry one (NTS/WFMU) — the cell renders
 * blank rather than fabricating a time; position keeps the row order.
 */
export function formatPlayTime(isoString: string | null | undefined): string | null {
  if (!isoString) return null
  const date = new Date(isoString)
  if (isNaN(date.getTime())) return null
  return date.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * "9:00 PM" from an HH:MM[:SS] air_time string (episode-level scheduled
 * time); null when missing or unparseable.
 */
export function formatTimeOfDay(timeStr: string | null | undefined): string | null {
  if (!timeStr) return null
  const [hoursStr, minutesStr] = timeStr.split(':')
  const hours = parseInt(hoursStr, 10)
  const minutes = parseInt(minutesStr, 10)
  if (isNaN(hours) || isNaN(minutes)) return null
  const period = hours >= 12 ? 'PM' : 'AM'
  const displayHours = hours === 0 ? 12 : hours > 12 ? hours - 12 : hours
  return `${displayHours}:${String(minutes).padStart(2, '0')} ${period}`
}

/** "2h 58m" / "45m" for the playlist meta line; null when unknown. */
export function formatDurationMinutes(
  minutes: number | null | undefined
): string | null {
  if (minutes == null || minutes <= 0) return null
  const hours = Math.floor(minutes / 60)
  const mins = minutes % 60
  if (hours === 0) return `${mins}m`
  if (mins === 0) return `${hours}h`
  return `${hours}h ${mins}m`
}

/**
 * Archive-table date: "Jun 9 2026" (mock register, no comma). Parses at local
 * midnight so a date-only string doesn't shift a day in negative-offset zones.
 */
export function formatArchiveDate(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  if (isNaN(date.getTime())) return dateStr
  const monthDay = date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
  return `${monthDay} ${date.getFullYear()}`
}

/** Short "Jun 2" label for the prev/next episode nav brackets. */
export function formatShortNavDate(dateStr: string): string {
  const date = new Date(dateStr + 'T00:00:00')
  if (isNaN(date.getTime())) return dateStr
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

// ---------------------------------------------------------------------------
// Prev/next episode neighbors
// ---------------------------------------------------------------------------

export interface EpisodeNeighbors {
  /** The next-more-recent episode (rendered on the right, "Jun 9 ▶"). */
  newer: RadioEpisodeListItem | null
  /** The next-older episode (rendered on the left, "◀ May 26"). */
  older: RadioEpisodeListItem | null
}

export type EpisodePageFetcher = (
  offset: number,
  limit: number
) => Promise<RadioEpisodesListResponse>

const NEIGHBOR_PAGE_SIZE = 100
/** Bounds the walk (2,000 episodes) so a bad date can't page forever. */
const NEIGHBOR_MAX_PAGES = 20

/**
 * Find the prev/next neighbors of the episode airing on `date` by walking the
 * show's episodes list (air_date DESC) page by page. Handles page boundaries:
 * a hit at the top of a page takes its newer neighbor from the previous
 * page's tail, and a hit at the bottom fetches one more row for the older
 * neighbor. Returns nulls at the newest/oldest ends and when the date isn't
 * found within the walk cap.
 */
export async function walkEpisodeNeighbors(
  date: string,
  fetchPage: EpisodePageFetcher
): Promise<EpisodeNeighbors> {
  let prevPageTail: RadioEpisodeListItem | null = null

  for (let page = 0; page < NEIGHBOR_MAX_PAGES; page++) {
    const offset = page * NEIGHBOR_PAGE_SIZE
    const response = await fetchPage(offset, NEIGHBOR_PAGE_SIZE)
    const episodes = response.episodes ?? []
    const index = episodes.findIndex(e => e.air_date === date)

    if (index >= 0) {
      const newer = index > 0 ? episodes[index - 1] : prevPageTail
      let older = index < episodes.length - 1 ? episodes[index + 1] : null
      const moreBeyondPage = offset + episodes.length < response.total
      if (older === null && moreBeyondPage) {
        const next = await fetchPage(offset + episodes.length, 1)
        older = next.episodes?.[0] ?? null
      }
      return { newer, older }
    }

    if (episodes.length < NEIGHBOR_PAGE_SIZE) break
    prevPageTail = episodes[episodes.length - 1]
  }

  return { newer: null, older: null }
}
