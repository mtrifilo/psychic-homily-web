/**
 * How long after a show's start it can still be "upcoming".
 *
 * The backend splits upcoming from past on the show's VENUE-LOCAL calendar day,
 * so a show stays upcoming until the end of the local day it starts on: at most
 * about 24 hours after its start, 25 across a DST change. 36 hours clears that
 * with margin, so a row older than this is past in every zone.
 */
export const PASSED_SHOW_GRACE_MS = 36 * 60 * 60 * 1000

interface UpcomingShowsPayload {
  shows: ReadonlyArray<{ event_date: string }>
  total: number
}

/**
 * An upcoming-shows payload without the rows that have provably left the
 * upcoming window since it was fetched.
 *
 * A server-side read is served from the Data Cache, which hands back a stale
 * entry however old it is while it refreshes in the background, so the first
 * render after a quiet spell can carry rows the endpoint would no longer
 * return. Rows more than `PASSED_SHOW_GRACE_MS` past their start are dropped
 * and `total` reduced by the same count. A row that passed more recently than
 * that (last night's show) is kept, because a zone-independent bound cannot
 * tell it from tonight's, and so is any other change since the read (a new
 * show, a cancellation): those reach the served HTML late and are corrected by
 * the browser's mount-time refresh. A row whose date does not parse is kept.
 *
 * Returns the payload itself when nothing is dropped. Reads the clock unless
 * handed `now`, so a caller must run where reading it is legitimate: a
 * request-time render, as a `[slug]` page body is once it has awaited `params`.
 */
export function withoutPassedShows<T extends UpcomingShowsPayload>(
  payload: T,
  now: number = Date.now(),
): T {
  const cutoff = now - PASSED_SHOW_GRACE_MS
  const shows = payload.shows.filter(show => !(Date.parse(show.event_date) < cutoff))
  const dropped = payload.shows.length - shows.length
  if (dropped === 0) return payload
  return { ...payload, shows, total: Math.max(0, payload.total - dropped) }
}
