/**
 * Station-overview derivation helpers (PSY-1016, Radio D2 panel).
 *
 * Pure, hook-free transforms that turn the radio API responses into the
 * shapes the D2 station-overview panel renders. Keeping the derivation here
 * (rather than inline in components) makes the "Now Playing" v1-fallback
 * contract testable and gives a future real now-playing endpoint a single
 * seam to slot into without changing the panel's render shape (PSY-1022).
 */

import type {
  RadioPlay,
  RadioShowListItem,
  RadioEpisodeDetail,
} from '../types'

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
 * The "Now Playing" surface, derived from the most-recent episode of a
 * station's now-playing show (v1 fallback — see pickNowPlayingShow). A real
 * now-playing endpoint (PSY-1022) would populate this same shape from live
 * on-air data without touching the panel components.
 */
export interface NowPlaying {
  /** The track playing "right now" (v1: the latest logged play in the episode). */
  current: RadioPlay | null
  /** Recently-played artists (distinct, most-recent first), excluding `current`. */
  recentArtists: ArtistHop[]
}

/**
 * Pick the show to treat as the station's "current" show for the Now Playing
 * surface. Shows have no recency field on the list endpoint (they sort
 * name-ASC), so v1 uses episode_count as the proxy for "the station's active /
 * signature show" — the show with the most logged episodes. Ties break on the
 * lower id (stable, deterministic). Returns null for a station with no shows.
 *
 * This is the documented v1 heuristic; PSY-1022's live now-playing endpoint
 * supersedes it.
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
 * Order shows for the "Recent shows" list: most episodes first (most active),
 * id-ASC tiebreak. Optionally excludes the now-playing show so it isn't
 * repeated under the Now Playing card. Returns at most `limit` shows.
 */
export function orderRecentShows(
  shows: RadioShowListItem[] | undefined,
  options: { excludeShowId?: number; limit?: number } = {}
): RadioShowListItem[] {
  const { excludeShowId, limit = 3 } = options
  if (!shows) return []
  return shows
    .filter(s => s.id !== excludeShowId && s.episode_count > 0)
    .sort((a, b) => b.episode_count - a.episode_count || a.id - b.id)
    .slice(0, limit)
}

/**
 * Distinct artist hops from an episode's plays, most-recent first.
 *
 * Plays are stored position-ASC (position 1 = first track of the set, highest
 * position = most-recently spun), so we walk the list in reverse to surface
 * the freshest artists. De-duplicates by artist name (case-insensitive) so a
 * back-to-back double-spin doesn't waste a hop slot. `skipPlayId` drops the
 * "currently playing" track so it isn't echoed in the recently-played row.
 */
export function recentArtistsFromEpisode(
  plays: RadioPlay[] | undefined,
  options: { limit?: number; skipPlayId?: number } = {}
): ArtistHop[] {
  const { limit = 4, skipPlayId } = options
  if (!plays || plays.length === 0) return []

  const seen = new Set<string>()
  const hops: ArtistHop[] = []
  for (let i = plays.length - 1; i >= 0; i--) {
    const play = plays[i]
    if (play.id === skipPlayId) continue
    if (!play.artist_name) continue
    const key = play.artist_name.trim().toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    hops.push({ name: play.artist_name, slug: play.artist_slug })
    if (hops.length >= limit) break
  }
  return hops
}

/**
 * Build the Now Playing surface from the now-playing show's most-recent
 * episode (v1 fallback). `current` is the latest logged play (highest
 * position); `recentArtists` are the distinct artists spun just before it.
 */
export function deriveNowPlaying(
  episode: RadioEpisodeDetail | undefined
): NowPlaying {
  const plays = episode?.plays ?? []
  const current = plays.length > 0 ? plays[plays.length - 1] : null
  return {
    current,
    recentArtists: recentArtistsFromEpisode(plays, {
      limit: 4,
      skipPlayId: current?.id,
    }),
  }
}

/**
 * Format a YYYY-MM-DD air-date as a short "Jun 4" (no year), matching the D2
 * design. Parses at local midnight so a date-only string doesn't shift a day
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
