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
import {
  formatLocalTimeRange,
  formatTimeRangeInZone,
  isValidWindow,
} from './stationOverview'

/**
 * Whether an episode is live RIGHT NOW: `now` falls within its frozen air
 * window [starts_at, ends_at] (PSY-1152). Replaces the old air-date-equality
 * heuristic (isAirDateToday), which treated any episode dated "today" as live
 * all day — the PSY-1128 false-ON-AIR bug (a Tuesday-morning show read "live"
 * at 5:54 PM). A null/absent or unbounded window (WFMU until PSY-1159, or any
 * provider with no time) is never live — the conservative default.
 */
export function isLiveNow(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined,
  now: Date = new Date()
): boolean {
  if (!startsAt || !endsAt) return false
  const start = new Date(startsAt).getTime()
  const end = new Date(endsAt).getTime()
  if (isNaN(start) || isNaN(end)) return false
  const t = now.getTime()
  return t >= start && t <= end
}

/**
 * Whether the episode's air window is still ahead of or containing `now` —
 * i.e. the page may still need to FLIP regimes (upcoming→live at starts_at,
 * live→archive at ends_at). Drives the minute tick: without a pre-window
 * tick, a page opened before starts_at would show "airs …" through the
 * whole broadcast (nothing else re-renders it in production, where focus
 * refetch is disabled). Same conservative null/NaN handling as isLiveNow.
 */
export function isWindowLiveOrPending(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined,
  now: Date = new Date()
): boolean {
  if (!startsAt || !endsAt) return false
  const start = new Date(startsAt).getTime()
  const end = new Date(endsAt).getTime()
  if (isNaN(start) || isNaN(end)) return false
  return now.getTime() <= end
}

/** Poll cadence for a live episode's playlist (the live ledger regime). */
export const LIVE_EPISODE_POLL_MS = 60 * 1000

/**
 * refetchInterval gate for the episode playlist query: poll only while the
 * episode is genuinely live (inside its frozen air window), and stop on a
 * failing query — a function refetchInterval keeps firing on a persistently
 * failing query otherwise (the PSY-1136 infinite-poll class). Past ends_at
 * (or for windowless episodes) the page is a static archive: no polling.
 */
export function liveEpisodePollMs(
  error: unknown,
  startsAt: string | null | undefined,
  endsAt: string | null | undefined,
  now: Date = new Date()
): number | false {
  if (error) return false
  return isLiveNow(startsAt, endsAt, now) ? LIVE_EPISODE_POLL_MS : false
}

/**
 * Relative TIME label for the live ledger's older rows: "now" under a
 * minute, then minute-granular "2m" / "14m" / "1h 2m" since the play's
 * air_timestamp. Null when the feed carried no timestamp (the cell renders
 * blank rather than fabricating a time, matching the archive rendering).
 * A slightly-future timestamp (clock skew) clamps to "now".
 *
 * Deliberately distinct from the consolidated lib/formatRelativeTime +
 * lib/formatTimeAgo helpers: the ledger's compact mono grammar has no
 * " ago" suffix, combines "1h 2m", and takes an injected `now` for the
 * render-pulse/test pattern. Don't fold them together in a drift audit.
 */
export function formatRelativeMinutes(
  isoString: string | null | undefined,
  now: Date = new Date()
): string | null {
  if (!isoString) return null
  const t = new Date(isoString).getTime()
  if (isNaN(t)) return null
  const mins = Math.max(0, Math.floor((now.getTime() - t) / 60_000))
  if (mins < 1) return 'now'
  if (mins < 60) return `${mins}m`
  const hours = Math.floor(mins / 60)
  const rest = mins % 60
  return rest === 0 ? `${hours}h` : `${hours}h ${rest}m`
}

/**
 * "updated 40s ago" / "updated 2m ago" for the live band's right edge, from
 * the query's dataUpdatedAt (ms epoch). Null before the first fetch resolves
 * (dataUpdatedAt 0) — the band renders without the aside rather than
 * claiming an update time it doesn't have.
 */
export function formatUpdatedAgo(
  updatedAtMs: number,
  now: Date = new Date()
): string | null {
  if (!updatedAtMs) return null
  const secs = Math.max(0, Math.floor((now.getTime() - updatedAtMs) / 1000))
  if (secs < 60) return `updated ${secs}s ago`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `updated ${mins}m ago`
  // A long-backgrounded tab can wake hours behind — "187m" reads wrong.
  return `updated ${Math.floor(mins / 60)}h ago`
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
 * by artist name (case-insensitive, trimmed); a name counts as matched when
 * any of its plays carries an artist_id.
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
 *
 * Two hardenings (adversarial review):
 * - DESC-order early exit: if the page didn't contain the date but the date
 *   sorts AFTER the page tail, it can't be on a later (older) page — stop
 *   instead of walking the whole archive for a bogus URL date.
 * - Same-date siblings (the unique index allows two episodes on one date via
 *   distinct external_ids) are never returned as neighbors — the by-date
 *   route can only show one episode per date, so such a link would point at
 *   the current page itself.
 */
export async function walkEpisodeNeighbors(
  date: string,
  fetchPage: EpisodePageFetcher
): Promise<EpisodeNeighbors> {
  let prevPageTail: RadioEpisodeListItem | null = null

  for (let page = 0; page < NEIGHBOR_MAX_PAGES; page++) {
    const offset = page * NEIGHBOR_PAGE_SIZE
    const response = await fetchPage(offset, NEIGHBOR_PAGE_SIZE)
    const all = response.episodes ?? []
    // Upcoming (not-yet-aired) episodes aren't navigable (PSY-1205): the archive
    // doesn't link them, so the prev/next nav must not surface a "newer ▶" arrow
    // that lands on an empty future page either. Filter them out before picking
    // neighbors; pagination math below stays on raw `all`/`total`.
    const aired = all.filter(e => !e.is_upcoming)
    // Collapse same-date siblings so a neighbor never self-links.
    const episodes = aired.filter(
      (e, i) => i === 0 || e.air_date !== aired[i - 1].air_date
    )
    const index = episodes.findIndex(e => e.air_date === date)

    if (index >= 0) {
      const tailIsSameDate = prevPageTail?.air_date === date
      const newer = index > 0 ? episodes[index - 1] : tailIsSameDate ? null : prevPageTail
      let older = index < episodes.length - 1 ? episodes[index + 1] : null
      const moreBeyondPage = offset + all.length < response.total
      if (older === null && moreBeyondPage) {
        const next = await fetchPage(offset + all.length, 1)
        const candidate = next.episodes?.[0] ?? null
        older = candidate && candidate.air_date !== date ? candidate : null
      }
      return { newer, older }
    }

    if (all.length < NEIGHBOR_PAGE_SIZE) break
    // ISO dates compare lexicographically: list is DESC, so a date newer
    // than this page's tail can't appear on any later page.
    if (date > all[all.length - 1].air_date) break
    prevPageTail = episodes[episodes.length - 1]
  }

  return { newer: null, older: null }
}

/**
 * Viewer-local "aired ..." body for the playlist detail page (PSY-1306):
 * "Wed 6–9 AM your time (9 AM–12 PM EDT)" — weekday + range in the VIEWER's
 * timezone from the frozen air window, with a station-local aside when the
 * station's IANA zone is known and is NOT the viewer's zone (zone identity —
 * a same-offset zone still gets the aside, its name is the information).
 * Returns null when
 * the window is missing/degenerate (caller falls back to the station-dated
 * air_time line, exactly the pre-PSY-1306 rendering). The caller prefixes
 * "aired"/"airs" (PSY-1205 upcoming semantics are unchanged).
 */
export function formatViewerAiredLine(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined,
  stationTimezone: string | null | undefined
): string | null {
  const viewerRange = formatLocalTimeRange(startsAt, endsAt)
  if (!viewerRange || !startsAt) return null
  const weekday = new Date(startsAt).toLocaleDateString('en-US', {
    weekday: 'short',
  })
  const stationRange = formatTimeRangeInZone(startsAt, endsAt, stationTimezone)
  // Skip the aside only when the viewer IS in the station's zone (zone
  // equality, not clock equality — two zones sharing a UTC offset still get
  // the aside, because the zone name itself is the information).
  const viewerZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const aside =
    stationRange && stationTimezone !== viewerZone ? ` (${stationRange})` : ''
  return `${weekday} ${viewerRange} your time${aside}`
}

/**
 * Window-aware verb for the detail page's aired line (PSY-1306): the
 * day-granular is_upcoming can't see that a TODAY-dated episode's window is
 * still in the future (or in progress) — pairing "aired" with an explicit
 * future clock range would lie. Falls back to is_upcoming for windowless
 * episodes (PSY-1205 semantics unchanged there).
 */
export function airedVerbForWindow(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined,
  isUpcoming: boolean,
  now: Date = new Date()
): 'airs' | 'airing' | 'aired' {
  // Same validity bar as every rendered time block (isValidWindow): a
  // degenerate window must not drive the verb either — otherwise a corrupt
  // wrong-day ends_at reads "airing" for weeks next to a line that rejected
  // that very window.
  if (isValidWindow(startsAt, endsAt) && startsAt && endsAt) {
    const start = new Date(startsAt)
    const end = new Date(endsAt)
    if (start > now) return 'airs'
    if (end > now) return 'airing'
    return 'aired'
  }
  return isUpcoming ? 'airs' : 'aired'
}
