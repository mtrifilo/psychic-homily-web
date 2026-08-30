import { formatLocation } from '@/lib/formatLocation'
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
 * `??`, NOT `||`, and the difference is load-bearing (PSY-1696). `venues.state`
 * is NOT NULL, so a venue with no state on file stores `''` rather than null,
 * and `??` therefore keeps that empty string instead of consulting the show
 * row. That looks like it discards information, and it is deliberate.
 *
 * The row this decides is a US show repointed onto an international venue by a
 * merge, which does not rewrite the denormalized `shows.state`: venue
 * `state: ''`, `timezone: null`, show `state: 'NY'`. Falling through to `'NY'`
 * would hand a Berlin show a zone the state map KNOWS, so
 * `isShowTimezoneResolved` would answer true and the page would print DOORS and
 * a start time in America/New_York beside a Berlin address. Keeping `''` keeps
 * the answer honest — no zone is known — and the show page's clock refusals
 * hold. A guessed DATE is at most a day out; a laundered CLOCK is hours out and
 * reads as fact. `CompactShowRow` documents the identical trap in its `'AZ'`
 * form.
 *
 * The cost of that choice is paid in `showToFormValues`, which MUST seed the
 * edit form's `venue.state` field from this same value: `ShowForm`'s submit
 * resolves its zone from that field, so if the two spellings diverge the form
 * opens on one wall clock and saves through another, moving `event_date` on a
 * no-op Save and again on every save after.
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
 * Render-time dedup helper for show lists (PSY-559).
 *
 * One caller left as of PSY-1754: the Atlas venue panel, which fetches a
 * single unpaged page. Neither DETAIL page dedups any more — see
 * `VenueShowsList` (PSY-1753) and `ArtistShowsList` (PSY-1754) for why: the
 * structural unique index makes the class impossible, and a filtered page
 * would render fewer rows than its own pager claims.
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

/** The bill fields that decide who comes first. */
interface OrderedBillArtist {
  position: number
  id: number
}

/**
 * Bill order lives in `show_artists.position`. Every backend read path already
 * sorts by it (`buildShowResponse`, `loadShowArtistResponses`), so this is a
 * defensive re-assertion against a caller, cache layer, or future query handing
 * us a different order.
 *
 * Ties are possible: `idx_show_artists_position` is a plain index, so nothing
 * enforces one position per show, and rows written outside the create path
 * (backfills, seeds) can share position 0. The backend's `ORDER BY position
 * ASC` has no tiebreaker, so Postgres may order tied rows differently between
 * requests. Break the tie on `id` so the rendered bill is at least
 * deterministic client-side.
 *
 * Shared for the same reason `splitBill` is: the header's bill and the listen
 * module's cards are two renderings of ONE running order, and a second copy of
 * the tiebreak rule is exactly the kind of thing that drifts silently.
 *
 * This is HALF of that running order, not all of it. Position is the sequence;
 * `splitBill` then hoists whoever is curated as a headliner, because `set_type`
 * is authoritative at any position and a bill entered in stage order puts the
 * headliner last. A surface that sorts and stops will print a different bill
 * from one that does both, on exactly the shows where it matters most.
 */
export function byBillPosition(
  a: OrderedBillArtist,
  b: OrderedBillArtist
): number {
  return a.position - b.position || a.id - b.id
}

/** The location fields an act is placed by. */
interface PlaceableArtist {
  city?: string | null
  state?: string | null
  country?: string | null
}

/**
 * Where an act is from, or null when nothing about it is placeable.
 *
 * Judged on the PARTS, never on the formatted string. Comparing the result to
 * `LOCATION_UNKNOWN` would also silence an artist whose city is literally
 * "Location Unknown", which is exactly the placeholder an extraction run writes
 * when it does not know.
 *
 * `formatLocation` carries the locked display rule: country is included UNLESS
 * the state is set and the country is USA/US.
 *
 * Shared for the same reason `byBillPosition` is. The header's bill and the
 * listen module's cards state the same fact about the same act a few hundred
 * pixels apart, so a second copy of the placeability test is a rule with two
 * answers on one page.
 */
export function billHometown(artist: PlaceableArtist): string | null {
  const hasPlaceableLocation = [
    artist.city,
    artist.state,
    artist.country,
  ].some(part => part?.trim())
  if (!hasPlaceableLocation) return null
  return formatLocation({
    city: artist.city,
    state: artist.state,
    country: artist.country,
  })
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
