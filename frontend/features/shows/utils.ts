import type { ShowTimingInput } from '@/lib/utils/showTiming'
import type { ShowResponse } from './types'

/**
 * The timing view of a show: its start instant and the zone that instant is
 * meant to be read in.
 *
 * One mapping, so a page cannot judge a show on two different calendars. The
 * status stripe's copy is rendered on the client from the show payload while
 * its past/tonight/upcoming state is computed on the server; if the two picked
 * zones differently, a band could say TONIGHT above tomorrow's date.
 *
 * The VENUE's state wins over the show's, because the venue is where the show
 * happens and a show row's `state` is denormalized and can lag an edit. Note
 * this only ever matters for a venue with no resolved `timezone`, since
 * `resolveShowTimezone` consults `state` only as a fallback.
 *
 * Not the repo-wide rule yet. The show PAGE uses it throughout, but `ShowCard`
 * and the artist / venue list rows still pass `show.state` alongside the
 * venue's timezone, which differs from this for a zone-less venue whose state
 * disagrees with its show row. That is the same class of bug this exists to
 * prevent, one surface out; converging them is a follow-up, not this ticket.
 */
export function showTimingInput(show: ShowResponse): ShowTimingInput {
  const venue = show.venues?.[0]
  return {
    eventDate: show.event_date,
    state: venue?.state ?? show.state,
    timezone: venue?.timezone,
  }
}

/**
 * Render-time dedup helpers for show lists shown on artist + venue
 * detail pages (PSY-559).
 *
 * The dedup key MUST include time (full ISO event_date), not just
 * the date — matinee + evening sets at the same venue on the same
 * day are NOT duplicates and must remain visible.
 *
 * Lowest-ID wins. Used as a stopgap so the visual fix lands even if
 * the backend dedup cmd (`cmd/dedup-shows`) hasn't been run on the
 * target environment yet.
 */

interface ShowLike {
  id: number
  event_date: string
}

interface ArtistInList {
  id: number
  is_headliner?: boolean | null
  set_type?: string
  position?: number
}

interface ShowWithVenueAndArtists extends ShowLike {
  venue?: { id: number } | null
  artists: ArtistInList[]
}

interface ShowWithArtists extends ShowLike {
  artists: ArtistInList[]
}

// Pick a stable headliner artist id for a show. Prefer set_type, then
// is_headliner, then position 0, then the first artist. Returns null
// when the show has no artists.
function headlinerArtistId(show: ShowWithArtists): number | null {
  if (!show.artists || show.artists.length === 0) {
    return null
  }
  return (
    show.artists.find(a => a.set_type === 'headliner')?.id ??
    show.artists.find(a => a.is_headliner === true)?.id ??
    show.artists.find(a => a.position === 0)?.id ??
    show.artists[0].id
  )
}

// Filter input to one row per dedup key, preserving input order.
// Among collisions on the same key, the show with the lowest id wins.
//
// Map iteration order alone is not enough: a later row can replace an
// earlier winner inside the Map and that mutation would shuffle the
// rendered order relative to the API response. We collect winner ids
// into a Set and re-filter the original array instead.
function pickWinners<T extends ShowLike>(
  shows: T[],
  keyFor: (show: T) => string,
): T[] {
  const winnersByKey = new Map<string, T>()
  for (const show of shows) {
    const key = keyFor(show)
    const existing = winnersByKey.get(key)
    if (!existing || show.id < existing.id) {
      winnersByKey.set(key, show)
    }
  }
  const winnerIds = new Set<number>()
  for (const v of winnersByKey.values()) winnerIds.add(v.id)
  return shows.filter(s => winnerIds.has(s.id))
}

/**
 * Dedup shows on an ARTIST detail page. Each show carries its own
 * `venue.id`, so the key is `(venue.id, event_date)`.
 */
export function dedupArtistShows<T extends ShowWithVenueAndArtists>(shows: T[]): T[] {
  return pickWinners(shows, show => `${show.venue?.id ?? 0}|${show.event_date}`)
}

/**
 * Dedup shows on a VENUE detail page. The list is already scoped to
 * ONE venue, so the key is `(headliner_artist_id, event_date)`.
 * Shows without artists keep their own bucket (key uses 0).
 */
export function dedupVenueShows<T extends ShowWithArtists>(shows: T[]): T[] {
  return pickWinners(shows, show => `${headlinerArtistId(show) ?? 0}|${show.event_date}`)
}

/** The bill fields that decide who leads. */
interface BillArtist {
  set_type?: string | null
  is_headliner?: boolean | null
}

/**
 * Split a bill into the acts at the top and everyone under them.
 *
 * The curated `set_type` is authoritative when it says "headliner";
 * `is_headliner` is the older flag and still carries shows written before the
 * roles existed, so both count. A bill that claims neither is read in listed
 * order, with the first act leading — which is how a flyer reads.
 *
 * Shared because "who headlines this show" must not have a different answer on
 * the show card, the show header and the venue table for the same show.
 */
export function splitBill<T extends BillArtist>(
  artists: T[]
): { headliners: T[]; support: T[] } {
  const leads = (artist: T) =>
    artist.set_type === 'headliner' || artist.is_headliner === true

  const headliners = artists.filter(leads)
  if (headliners.length === 0) {
    return artists.length > 0
      ? { headliners: [artists[0]], support: artists.slice(1) }
      : { headliners: [], support: [] }
  }
  return { headliners, support: artists.filter(artist => !leads(artist)) }
}

/**
 * Upcoming Shows list count label.
 * When the loaded page(s) are a subset of the filter-aware total, show
 * "N of T shows" so users know the catalog is larger than what's on screen.
 */
export function formatShowCountLabel(loaded: number, total?: number | null): string {
  const noun = loaded === 1 ? 'show' : 'shows'
  if (total == null || total <= loaded) {
    return `${loaded} ${noun}`
  }
  // Pinned to en-US, like every date in this list (`formatInTimezone`): the
  // label is server-rendered since PSY-1624, and a bare `toLocaleString()`
  // yields "1,234" from Node and "1.234" in a de-DE browser — a hydration
  // mismatch that only appears once a filter's total passes 999.
  return `${loaded.toLocaleString('en-US')} of ${total.toLocaleString('en-US')} ${noun}`
}
