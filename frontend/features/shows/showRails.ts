import type { components } from '@/types/api'
import { looksLikeCalendarDate, formatDayChip } from '@/features/scenes/sceneDay'
import type { VenueShow } from '@/features/venues/types'

/**
 * The show page's two discovery rails: what else is on in this metro that
 * night, and what else this room has coming.
 *
 * This module is the rails' POLICY — which rows survive, what the headings
 * say, where the "see all" bracket points — kept apart from the markup in
 * `ShowDiscoveryRails.tsx` so each decision has one testable home. Nothing
 * here renders.
 */

/** The also-tonight payload, derived from the generated schema (never hand-written). */
export type ShowAlsoTonightResponse =
  components['schemas']['ShowAlsoTonightResponse']

/** One row of the also-tonight rail. Same wire type the scene day/week views read. */
export type AlsoTonightShow = components['schemas']['SceneShowSummary']

/**
 * Rows drawn per rail.
 *
 * Three, the number the locked mock draws. A rail is a glance, not a listing:
 * the full answer lives one click away behind each rail's "see all", and the
 * year-anchored pagination precedent the venue and artist show lists set
 * (PSY-1750..1756) is deliberately NOT imported here — it governs ORDERED
 * ARCHIVES a reader navigates, not a fixed-length teaser.
 */
export const SHOW_RAIL_ROW_CAP = 3

/**
 * Rows to REQUEST for the venue rail: one more than the cap.
 *
 * The venue's upcoming list contains the show being viewed whenever that show
 * is itself upcoming, and dropping it is what stops a page recommending itself.
 * Asking for exactly the cap would then leave the rail one row short on every
 * upcoming show — the common case. One spare absorbs that removal.
 *
 * The also-tonight endpoint excludes the subject show server-side, so its rail
 * needs no equivalent (`alsoTonightRailRows` still re-checks; see there).
 */
export const VENUE_RAIL_FETCH_LIMIT = SHOW_RAIL_ROW_CAP + 1

/**
 * The also-tonight rows this rail should draw.
 *
 * The exclusion is belt-and-braces: `GET /shows/{id}/also-tonight` already
 * documents that it excludes the subject show, and this is the boundary where
 * that promise arrives from another process. A show listed in its own "also
 * tonight" rail is the single most visible way this feature can be wrong, and
 * the check costs one comparison per row.
 */
export function alsoTonightRailRows(
  rail: ShowAlsoTonightResponse | undefined,
  currentShowId: number,
  cap: number = SHOW_RAIL_ROW_CAP
): AlsoTonightShow[] {
  // `shows` is typed nullable by the generator even though the API always
  // emits an array — same accommodation `dayShows` makes.
  const rows = rail?.shows ?? []
  return rows.filter(show => show.id !== currentShowId).slice(0, cap)
}

/**
 * The venue rows this rail should draw: the room's next shows, minus this one.
 */
export function moreAtVenueRailRows(
  shows: VenueShow[] | undefined,
  currentShowId: number,
  cap: number = SHOW_RAIL_ROW_CAP
): VenueShow[] {
  return (shows ?? []).filter(show => show.id !== currentShowId).slice(0, cap)
}

/**
 * The also-tonight heading's qualifier: `Tonight`, or the night's own date.
 *
 * "Tonight" is the mock's register and the rail's name, but it is only TRUE on
 * the night itself, and a show page is read months early and years late. The
 * payload settles it: `is_tonight` is computed on the SCENE's clock with the
 * 6am night boundary applied (until 06:00 local, "tonight" still names
 * yesterday's date), because a client computing it from the viewer's device
 * would give a reader in Berlin a different answer than a reader in Chicago
 * for the same Chicago night. Read the flag; never re-derive it.
 */
export function alsoTonightQualifier(rail: ShowAlsoTonightResponse): string {
  return rail.is_tonight ? 'Tonight' : formatDayChip(rail.date)
}

/**
 * The full also-tonight heading, in the page's `SECTION / QUALIFIER` register.
 *
 * The city is the metro's PRINCIPAL city as the backend resolved it, so an
 * Evanston room reads "Chicago" — the scope the rows were actually selected
 * by. It is omitted rather than guessed when the payload has no scene, which
 * is also the case where there are no rows to head.
 */
export function alsoTonightRailTitle(rail: ShowAlsoTonightResponse): string {
  const qualifier = alsoTonightQualifier(rail)
  return rail.city ? `Also / ${qualifier} · ${rail.city}` : `Also / ${qualifier}`
}

/**
 * Where the also-tonight rail's "see all" goes: the scene's own page for that
 * night, `/scenes/{slug}/{YYYY-MM-DD}`.
 *
 * Null unless BOTH halves are honest. `scene_slug` is withheld by the backend
 * whenever following it would land somewhere that does not list the show it
 * came from (an archive date outside the scene-day window, or a room the metro
 * backfill never reached), so its presence is the server's own permission to
 * link. The date is re-checked against the same shape the `/scenes/{slug}/
 * {period}` route uses to route a segment, so a malformed date can never be
 * turned into a link to a page that will 404.
 */
export function alsoTonightSeeAllHref(
  rail: ShowAlsoTonightResponse
): string | null {
  if (!rail.scene_slug || !looksLikeCalendarDate(rail.date)) return null
  return `/scenes/${rail.scene_slug}/${rail.date}`
}

/**
 * Whether the night held more than this rail drew, so the "see all" bracket
 * says something rather than merely repeating the rows.
 *
 * Two independent sources of truncation: the backend's own cap (`has_more`,
 * which compares against the whole night, not against this rail), and this
 * rail's cap of three. Either one hiding a show is a reason to offer the full
 * night.
 */
export function alsoTonightHasMore(
  rail: ShowAlsoTonightResponse,
  drawnCount: number,
  currentShowId: number
): boolean {
  if (rail.has_more) return true
  const available = (rail.shows ?? []).filter(
    show => show.id !== currentShowId
  ).length
  return available > drawnCount
}
