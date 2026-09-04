/**
 * The rooms leaderboard's ordering and labelling rules (PSY-1784).
 *
 * Split out of the component for the same reason the calendar's bucketing is:
 * these are decisions about MEANING, not markup, and the sparse-vs-dense rule
 * below is the one this module exists to make testable at both ends.
 */

import { socialLinkHref } from '@/lib/socialLinks'
import type { SceneVenue } from './types'

/** How the list is ordered, and therefore whether the counts carry ordering. */
export type RoomOrder = 'ranked' | 'alphabetical'

/**
 * A ranking needs at least two ranked things.
 *
 * DERIVED, not quoted: decision 11's matrix locks THAT the rooms module has a
 * dense state (leaderboard by count) and a sparse one (named list,
 * alphabetical, counts dropped), and the mock draws both — but neither names
 * the number that separates them. This is the smallest rule that reproduces
 * both locked frames from the data alone: Phoenix has 12 rooms with something
 * booked and ranks; Portland has ONE upcoming show, so exactly one room is
 * non-zero and a "leaderboard" of 1, 0, 0 orders nothing while implying the two
 * zeroes lost a contest they were never in.
 *
 * Deliberately NOT a threshold on the scene's show count or room count: the
 * question the count answers is "which room should I check first", and only the
 * spread across rooms can answer it. A scene can be busy at one room and dead
 * everywhere else, and that scene should read alphabetically.
 */
export const ROOMS_RANKABLE_MIN_BOOKED = 2

/** Rooms with at least one approved show still to come. */
export function bookedRoomCount(rooms: SceneVenue[]): number {
  return rooms.filter(room => room.upcoming_show_count > 0).length
}

/** Ranked when the counts can order the list, alphabetical when they cannot. */
export function defaultRoomOrder(rooms: SceneVenue[]): RoomOrder {
  return bookedRoomCount(rooms) >= ROOMS_RANKABLE_MIN_BOOKED
    ? 'ranked'
    : 'alphabetical'
}

/**
 * Order the rooms, both orders TOTAL.
 *
 * The ranked branch re-states the server's own `upcoming_show_count DESC, name
 * ASC, id ASC` rather than trusting arrival order. That is not distrust of the
 * API: the alphabetical escape hatch means this list is re-ordered client-side
 * anyway, and having exactly one place that decides row order is what keeps the
 * two orders from drifting into different tiebreaks. Ids break name ties
 * because venue names are unique only WITHIN a city and a metro spans several,
 * so one scene really can hold two rooms of the same name.
 */
export function orderRooms(rooms: SceneVenue[], order: RoomOrder): SceneVenue[] {
  // The locale is PINNED, and that is not pedantry. `localeCompare()` with no
  // locale uses the runtime's, which is the server's on the prerender and the
  // reader's in the browser — so a room list containing an accented or
  // non-Latin name could sort one way in the HTML and another after hydration,
  // which React reports as a mismatch and repairs by re-rendering the list.
  const byNameThenId = (a: SceneVenue, b: SceneVenue) =>
    a.name.localeCompare(b.name, 'en') || a.id - b.id

  return [...rooms].sort((a, b) =>
    order === 'ranked'
      ? b.upcoming_show_count - a.upcoming_show_count || byNameThenId(a, b)
      : byNameThenId(a, b)
  )
}

/**
 * Where the reader would actually be going, or nothing.
 *
 * The room's own city, because a metro scene is not one city — a Tempe room
 * belongs to the Phoenix scene and the mock prints `(Tempe)` beside it. The
 * state joins it only when it DIFFERS from the scene's, which is the case the
 * wire contract calls out: a Philadelphia scene reaches into Camden NJ, and a
 * bare `(Camden)` there reads as a Pennsylvania neighbourhood.
 *
 * `venues.city` is NOT NULL but may still be the empty string, so a blank is a
 * state to render around rather than an impossible one.
 */
export function roomLocationLabel(
  room: SceneVenue,
  sceneState: string
): string | null {
  const city = room.city?.trim()
  if (!city) return null
  const state = room.state?.trim()
  return state && state !== sceneState.trim() ? `${city}, ${state}` : city
}

/**
 * The room's own website, or nothing.
 *
 * `SceneVenueSummary.Website` is `venues.website`, the same column the venue
 * page reads as `venue.social?.website`, so it goes through the same gate: one
 * stored value cannot link on one surface and not the other. The gate refuses a
 * `javascript:` value, which would otherwise execute in the reader's session,
 * and resolves a legacy scheme-less value instead of dropping it.
 */
export function roomWebsite(room: SceneVenue): string | null {
  return socialLinkHref('website', room.website)
}
