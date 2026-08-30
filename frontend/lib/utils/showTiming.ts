import { resolveShowTimezone } from './formatters'

/**
 * Where a show sits relative to now, answered once.
 *
 * Two questions, because the surfaces asking are not asking the same thing and
 * collapsing them is how this went wrong before:
 *
 * - "Can a reader still BUY this?" is a question about the start instant.
 *   Doors open once, at one moment, everywhere in the world.
 * - "Is this still a live LISTING?" is a question about the venue's calendar
 *   day. A show is over when it is over WHERE IT HAPPENED, not when the
 *   reader's clock rolls over. `getShowLifecycleState` answers it in three
 *   parts and `isShowPast` collapses that to a boolean; they are one boundary
 *   with two shapes, not two boundaries.
 *
 * Pick by the claim being made, not by which reads better. An `Offer` is the
 * first kind; a cache lifetime is the second.
 *
 * Where they are used TODAY, so nobody has to grep for it: `hasShowStarted`
 * backs both JSON-LD offer gates and the field-notes FORM gate.
 * `getShowLifecycleState` is computed once per show-page request (and once per
 * share-card request) and threaded down as a prop. `showIsArchived` refines it
 * for every past-tense CLAIM: the ticket line's closing statement and the
 * field-notes empty state. `hasReadableStartDate` backs that predicate and the
 * field-notes copy directly. `isShowPast` has
 * exactly one consumer, the share card's cache window. No listing surface uses
 * it yet: the past/upcoming splits on artist, venue and library pages are
 * partitioned by the API, and the scene pages read backend-computed
 * `is_past_day` / `is_past_week`. It is here because the show-page lifecycle
 * design needs one venue-local answer, and that work will consume it.
 *
 * There is deliberately NO post-midnight grace window ("a Friday show is still
 * Friday at 1 AM Saturday"). The lifecycle design calls for one, its duration
 * is an open product decision, and inventing a constant here would quietly bind
 * every consumer to a number nobody chose. It belongs in `isShowPast` when it
 * is decided, and nowhere else.
 */

/**
 * The venue-facing slice of a show payload. All three fields are optional or
 * nullable because every payload that carries them is delivered over the wire:
 * a TYPE is not a runtime guarantee when the frontend and backend deploy
 * separately and Next's data cache can serve a body fetched before a schema
 * change.
 */
export interface ShowTimingInput {
  /** The UTC start instant, as the API delivers it. */
  eventDate: string | null | undefined
  /** `venues.timezone` (GeoNames-backed), when the venue has one resolved. */
  timezone?: string | null
  /**
   * US state code, the fallback zone for venues predating the backfill. US
   * ONLY: an unrecognized value (including any non-US region) resolves to
   * America/Phoenix, so a non-US venue needs `timezone` populated to be judged
   * on its own calendar. See `isShowPast`.
   */
  state?: string | null
}

/**
 * The venue-local calendar date of an instant, as a comparable integer
 * (`20260314` for March 14 2026).
 *
 * Packed into one number rather than compared as `Date`s so the comparison is
 * over calendar days and nothing else: a `Date` reconstructed from local parts
 * would reintroduce a zone, and subtracting instants would reintroduce DST.
 * Ordering holds because each field occupies a fixed decimal width.
 *
 * Constructs its formatter per call, deliberately. An earlier revision memoized
 * them, which is the right shape for a hot loop, but the callers are per-page,
 * not per-row, so the memo bought nothing and cost a module-level mutable map
 * plus a cap to bound it. The busiest path is the share card, which now asks
 * TWICE — once for the cache window and once for the sold-out badge — so four
 * formatter constructions per request, and up to eight if the card retries.
 * Still not a loop. Add the memo back when a caller arrives that asks this per
 * row; `resolveShowTimezone` builds a throwaway formatter of its own on the
 * same path, so measure both together.
 */
function venueLocalDayOrdinal(instant: number, timeZone: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date(instant))
  const part = (type: string) => Number(parts.find(p => p.type === type)?.value)
  return part('year') * 10000 + part('month') * 100 + part('day')
}

/** The start instant in epoch milliseconds, or `null` when it cannot be read. */
function startInstantMs(eventDate: string | null | undefined): number | null {
  const at = Date.parse(eventDate ?? '')
  return Number.isFinite(at) ? at : null
}

/**
 * Whether the show's start instant can be read at all.
 *
 * The check `getShowLifecycleState` demands of every surface that renders
 * WORDS, made callable so a surface can ASK it rather than infer it. (It is
 * not yet the only implementation: `showStatusStripeCopy` still parses the
 * instant itself, because it needs the number rather than the yes/no. Fold
 * that one in here if this ever grows a rule beyond `Date.parse`.) An
 * undateable
 * show comes back `past` from that function, a default inherited from a
 * cache-window caller where "past" only meant "cache it longer" — so a
 * surface that prints the past register without asking this first will put an
 * archive claim over a show whose date nobody could read, on a page that
 * cannot show a date anywhere to justify it.
 *
 * Takes the instant field alone: readability is a property of the string, and
 * no timezone can rescue an unparseable one. This is deliberately NOT the
 * same question as "is the venue's timezone known" — a show on a guessed zone
 * still has a real date, and the stripe still says PAST SHOW for it.
 */
export function hasReadableStartDate(
  eventDate: string | null | undefined
): boolean {
  return startInstantMs(eventDate) !== null
}

export type ShowLifecycleState = 'past' | 'today' | 'upcoming'

/**
 * Whether a surface may speak about this show IN THE PAST TENSE.
 *
 * THE archive predicate. Every past-tense CLAIM — words a reader sees — must
 * branch on this rather than on `lifecycle === 'past'` directly, because the
 * raw lifecycle is wrong for that question in two directions. Both were live
 * in earlier revisions of the change that introduced this function; neither
 * ever reached production, and there is no incident to go looking for.
 *
 * REFUSALS are the deliberate exception and must NOT be converted: `ticketHref`
 * and `ShowAddToCalendar` branch on the raw lifecycle on purpose. Withholding
 * is the safe direction, and `ShowAddToCalendar`'s raw branch is load-bearing
 * against a `RangeError` — it must fire for an undateable show, which this
 * predicate deliberately reports as NOT archived.
 *
 * The two directions:
 *
 * - A CANCELLED show that has since gone by is `past` to the lifecycle, which
 *   knows nothing about cancellation. The stripe says CANCELLED and never
 *   PAST SHOW; a surface answering in the other state's words contradicts the
 *   one fact a reader must not miss.
 *
 *   Scope note, because the obvious stronger claim is NOT true today: this
 *   governs the surfaces that branch on it, not every string on the page. The
 *   field-notes COMPOSER still invites a cancelled show's reader to "share
 *   your experience at this show" — its copy does not consult this predicate,
 *   and rewording it is a copy decision rather than a refactor. Do not read
 *   this function as a guarantee that nothing asks.
 * - An UNDATEABLE show is `past` by a default inherited from a cache-window
 *   caller, where "past" only meant "cache it longer". A page that cannot
 *   print a date cannot assert the show happened either.
 *
 * Structural input type, not `ShowResponse`, so a surface outside the shows
 * feature can ask without importing it — the field-notes section lives in
 * `features/comments` and takes exactly these two facts as props.
 *
 * Note this is NOT the boundary for whether the show has BEGUN: that is
 * `hasShowStarted`, the start instant, and it is what the field-notes FORM
 * and the JSON-LD offer gate use. This one turns over at venue-local
 * midnight, so between doors and midnight a show is started but not yet
 * archived — which is the whole evening a page must still read as tonight's.
 */
export function showIsArchived(
  show: { eventDate: string | null | undefined; isCancelled: boolean },
  lifecycle: ShowLifecycleState
): boolean {
  if (show.isCancelled || lifecycle !== 'past') return false
  return hasReadableStartDate(show.eventDate)
}

/**
 * Where a show sits on the venue's own calendar: yesterday or earlier, today,
 * or a later day.
 *
 * The three-way answer `isShowPast` cannot give. A status line that only knows
 * past-or-not has to ask a second question to tell "tonight" from "in three
 * weeks", and every caller that asks it separately picks its own boundary. This
 * is the one boundary: the venue's calendar day.
 *
 * `today` deliberately spans the WHOLE venue-local day, so a show whose doors
 * opened two hours ago is still `today` until venue-local midnight. That is the
 * same edge `isShowPast` draws, and it is NOT the edge `hasShowStarted` draws:
 * between the start instant and local midnight a listing still reads as
 * tonight's while its ticket offer is already withdrawn. Both are correct for
 * what they claim, and a surface that mixes them is what this comment exists to
 * prevent. If a reader must not be told "tonight" once the band is on stage,
 * that is a copy decision to be made against `hasShowStarted`, not a change to
 * this boundary.
 *
 * Carries the same guards, and the same known timezone limit, as `isShowPast`;
 * see its comment before using this for anything a reader sees. One of those
 * guards needs restating here because this answer now drives COPY: an
 * undateable show comes back `past`, a default inherited from a cache-window
 * caller where "past" only meant "cache it longer". A surface that renders
 * words must check the date itself rather than let that default put PAST SHOW
 * over a show whose date nobody could read.
 *
 * `today` means the venue's calendar day, not the evening: a 2 PM matinee is
 * `today` from midnight, and a surface whose word for this state is TONIGHT
 * will say TONIGHT about it. That is a copy question for the surface, which
 * has the show's own doors time to answer it with.
 */
export function getShowLifecycleState(
  show: ShowTimingInput,
  now: Date = new Date()
): ShowLifecycleState {
  const startedAt = startInstantMs(show.eventDate)
  if (startedAt === null) return 'past'
  const readAt = now.getTime()
  if (!Number.isFinite(readAt)) return 'upcoming'
  const timeZone = resolveShowTimezone(show.state, show.timezone)
  const showDay = venueLocalDayOrdinal(startedAt, timeZone)
  const today = venueLocalDayOrdinal(readAt, timeZone)
  if (today > showDay) return 'past'
  if (today === showDay) return 'today'
  return 'upcoming'
}

/**
 * Whether the show's calendar day has ended in the venue's own timezone.
 *
 * The boundary is venue-local midnight, not the start instant: a show that
 * started two hours ago is still tonight's listing, and an 8 PM Phoenix show is
 * past at 00:00 Phoenix time whatever the reader's clock says.
 *
 * Use this for LISTING LIVENESS: how long a share card may be cached, whether
 * a row is still "tonight". Do NOT use it to decide whether tickets are still
 * on sale: doors close at an instant, and stretching that to the end of the
 * local day would publish a purchasable ticket for a show already in progress,
 * or for nearly a full day after an after-midnight one. `hasShowStarted` is the
 * predicate for that question.
 *
 * KNOWN LIMIT, read this before using it for anything a reader sees: the zone
 * comes from `resolveShowTimezone`, which ends at a US state map defaulting to
 * America/Phoenix. A venue outside the US whose `venues.timezone` has not been
 * backfilled is therefore judged on Arizona's calendar, which can be most of a
 * day out. Today that only picks a cache lifetime. Populate `venues.timezone`
 * for non-US venues before this decides anything a reader or crawler reads.
 *
 * Both inputs are guarded: an undateable show counts as past, and an unreadable
 * `now` counts as not-past, because the alternative is `Intl` throwing
 * `RangeError` out of a server component. "Counts as past" is not automatically
 * the safe direction here the way withholding an offer would be: for the share
 * card it selects the LONG cache window, which is why `isShowCardSettled`
 * depends on its route rejecting unparseable dates upstream.
 *
 * `now` is injectable so callers on the server can stay pure functions of their
 * input and so tests do not depend on the wall clock.
 */
export function isShowPast(show: ShowTimingInput, now: Date = new Date()): boolean {
  // Expressed through the three-way answer rather than alongside it: two
  // functions deciding the same midnight independently is exactly how they
  // drift apart, and the guards above are the subtle half.
  return getShowLifecycleState(show, now) === 'past'
}

/**
 * Whether the show's advertised start instant has passed.
 *
 * Use this whenever the answer is a claim about the SHOW ITSELF rather than
 * about the listing: whether an `Offer` may still say `InStock`, whether the
 * field-notes form may open. Doors open at one moment, and no reader's calendar
 * moves it.
 *
 * Takes the instant alone, not a `ShowTimingInput`: an instant is the same
 * instant in every zone, and accepting zone fields it cannot use would invite
 * callers to pass one and expect it to matter. That zone-independence is the
 * point for the field-notes gate, which exists to mirror the backend's own
 * `show.EventDate.After(time.Now())` rejection; a frontend that picked a
 * different boundary would either hide a form the API would have accepted or
 * offer one it is about to reject with a 400.
 *
 * An undateable show counts as started, which opens a form the API may then
 * reject with a 400. Deliberate: it is what this gate already did before the
 * derivation moved here, and the API is the authority on that rejection either
 * way. `isShowPast` applies the same rule to an unreadable date, but do not
 * assume the two are equally harmless there: see its own note.
 */
export function hasShowStarted(
  eventDate: string | null | undefined,
  now: Date = new Date()
): boolean {
  const startedAt = startInstantMs(eventDate)
  return startedAt === null || startedAt <= now.getTime()
}
