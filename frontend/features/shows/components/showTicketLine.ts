import { showTimingInput } from '../utils'
import { startTimeFactSegment } from './showStatusStripeCopy'
import { saysSoldOut } from './showSaleState'
import { showIsArchived } from '@/lib/utils/showTiming'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'

/**
 * The stored ticket URL with surrounding whitespace removed, or null when
 * the field holds nothing meaningful.
 *
 * Whitespace-only is a REAL stored value, not a paranoid guard: the backend
 * persists `ticket_url` untrimmed and the ingest paths skip the handler's
 * validator. Both readers in this module go through here so that fact, and
 * any future addition to it, has one home.
 *
 * "Stored", not "raw": the trim is the point of the function, so a caller
 * wanting the byte-for-byte column value must read `show.ticket_url` itself.
 *
 * Deliberately says nothing about whether the URL may be OFFERED — that is
 * {@link ticketHref}'s job, and it is a different question with different
 * inputs.
 */
function storedTicketUrl(show: ShowResponse): string | null {
  return show.ticket_url?.trim() || null
}

/**
 * The stored ticket URL repaired into something navigable, or null when
 * there is nothing to buy or no honest way to offer it.
 *
 * Null for a blank/whitespace value (see {@link storedTicketUrl}), for a
 * cancelled or sold-out show, and for a PAST show — offering a purchase
 * under a stripe that says CANCELLED or PAST SHOW is the page arguing with
 * itself, and the click is the half that costs a reader money. This is THE
 * derivation of "is there somewhere to buy": {@link ticketLineSegments}
 * branches on it and the Buy Tickets bracket consumes it, so a refusal
 * added here reaches both.
 *
 * Submitters type scheme-less hosts ("tix.example/1") and vendors print
 * uppercase schemes; the scheme test is therefore case-insensitive and
 * anchored (`https?://`), not a bare prefix check — `startsWith('http')`
 * passed "httpfoo.example" through as a RELATIVE href that navigated under
 * /shows/. Protocol-relative values keep their own scheme resolution.
 */
export function ticketHref(
  show: ShowResponse,
  lifecycle: ShowLifecycleState
): string | null {
  const raw = storedTicketUrl(show)
  if (!raw || show.is_cancelled || show.is_sold_out || lifecycle === 'past') {
    return null
  }
  if (/^https?:\/\//i.test(raw)) return raw
  if (raw.startsWith('//')) return `https:${raw}`
  return `https://${raw}`
}

/**
 * The ticket line's price register: `$35`, `$12.50`, `Free`. Whole dollars
 * drop the cents — the locked mock's line reads `$35 ADV`, and `.00` on a
 * tabular mono line is noise. Local to this line on purpose; the app-wide
 * `formatPrice` keeps its two-decimal form for the surfaces built on it,
 * which means a /shows card ($25.00) and the page it opens ($25) currently
 * spell one price two ways — a known register split, named here so the
 * card-side conversion is a deliberate follow-up rather than a drift.
 */
function ticketPrice(price: number): string {
  if (price === 0) return 'Free'
  return Number.isInteger(price) ? `$${price}` : `$${price.toFixed(2)}`
}

/**
 * `[]`, `['$35']`, or the mock's split pair `['$35 ADV', 'DOOR $40']`
 * (PSY-1864).
 *
 * `ADV` / `DOOR` are disambiguators, so they are spelled only when there ARE
 * two DIFFERENT numbers to tell apart. A lone price renders bare — including a
 * lone DOOR price, where the word would distinguish the number from nothing.
 * Segments rather than one string because the pair is two middot-separated
 * facts in the mock, not a compound one.
 *
 * Equal prices collapse to one bare segment. Nothing stops a curator entering
 * the same number twice (the door field's own placeholder says "only if it
 * differs", but that is a hint, not a constraint, and an importer has no hint
 * at all), and `$35 ADV · DOOR $35` spends two segments and two qualifiers to
 * say one thing — it reads as a rendering bug, and it would make this function
 * contradict the rule stated one paragraph above.
 *
 * Zero is a price ("Free"), not silence, which is why the guards test
 * `!= null` rather than truthiness.
 */
function ticketPriceSegments(show: ShowResponse): string[] {
  const advance = show.price
  const door = show.door_price
  if (advance != null && door != null && advance !== door) {
    return [`${ticketPrice(advance)} ADV`, `DOOR ${ticketPrice(door)}`]
  }
  const only = advance ?? door
  return only != null ? [ticketPrice(only)] : []
}

/**
 * Whether this show carries any ticket commerce at all: somewhere a reader
 * was sent to get in, or a price for getting in.
 *
 * Says nothing about WHEN — an upcoming show returns true too. It is the
 * "was there anything to sell" half of the past register's test; the "is it
 * over" half is {@link saysPastRegister}, and they are kept apart so neither
 * name has to promise the other's guard.
 *
 * The link is read through {@link storedTicketUrl} rather than
 * {@link ticketHref}, which refuses past shows by design — the question here
 * is what the show sold while it was upcoming, not what a reader can buy now.
 *
 * A LINK OUTRANKS A ZERO PRICE, deliberately: a free show with an RSVP or
 * guestlist link did have a reservation to close out, and `Free · NO LONGER
 * AVAILABLE` is the true thing to say about it once the list is shut.
 *
 * THE SOLD-OUT FLAG COUNTS TOO, and it is the input this function most
 * easily forgets because it is not a price and not a link. An ingested show
 * an admin marked sold out may carry neither field, and its line read
 * `8PM · SOLD OUT` right up until the show ended. Without this test it would
 * flip to a bare `8PM`: the register would go from a present-tense claim to
 * SILENCE rather than to the past tense of that claim, losing the most
 * emphatic evidence the show ever had of a sale to close.
 *
 * BOTH PRICES COUNT: a show sold only at the door still charged for entry.
 *
 * What returns false is the free show with no link and no sold-out flag, and
 * the show carrying none of the three — `NO LONGER AVAILABLE` is the past
 * tense of `ON SALE`, and a line that never said anything has nothing to
 * un-say. Those cases would only restate the stripe's `PAST SHOW` in a
 * register the page has no reason to use.
 */
function hasTicketCommerce(show: ShowResponse): boolean {
  if (storedTicketUrl(show) || show.is_sold_out) return true
  // The same zero that {@link ticketPrice} renders as "Free"; keep the two
  // in step if a price ever becomes something other than a plain number.
  return (show.price ?? 0) > 0 || (show.door_price ?? 0) > 0
}

/**
 * Whether this line speaks in the PAST register at all: {@link showIsArchived}
 * applied to a `ShowResponse`.
 *
 * A thin adapter and nothing more, deliberately. The rule itself (cancelled
 * shows and undateable shows are not archives) is shared with the field-notes
 * section in another feature, and an earlier revision of this change spelled
 * it out here AND there — the two copies then disagreed about cancellation
 * within one commit, which is how a cancelled past show came to be asked what
 * it was like. Add conditions to the shared predicate, never to this wrapper.
 *
 * There is deliberately NO condition on the venue timezone, and the omission
 * looks wrong at first glance, so here is why. `startTimeFactSegment` two
 * calls up REFUSES to print this show's start time on a guessed zone, because
 * a confidently wrong hour is worse than no hour. That rule does not transfer:
 * a guessed zone can be many hours out, which ruins an hour, but the stripe
 * still prints `PAST SHOW` for the same show on the same guess. Adding the
 * test would make this line MORE conservative than the band it is required to
 * agree with — the page would say PAST SHOW at the top and decline to close
 * the ticket line beneath it. One boundary, one answer, even when the boundary
 * is imperfect; fixing the guess is the timezone backfill's job, not this
 * line's.
 */
function saysPastRegister(
  show: ShowResponse,
  lifecycle: ShowLifecycleState
): boolean {
  return showIsArchived(
    { eventDate: show.event_date, isCancelled: show.is_cancelled },
    lifecycle
  )
}

/**
 * The ticket line's segments: `8PM · ON SALE · $35`, middot-joined by the
 * component.
 *
 * The start time leads (user decision): for the common show with no
 * announced doors/music the stripe prints a date alone, and this line is
 * then the only clock on the page — the mock's bare `ON SALE · $35 ADV`
 * assumed times the data mostly does not have yet. It renders through the
 * stripe's register and refusal rules ({@link startTimeFactSegment}): the
 * page has ONE clock spelling, and a venue whose timezone is a guess gets no
 * confidently-wrong hour here either.
 *
 * The sale state must never argue with the stripe, so its guards mirror the
 * stripe's own precedence. `SOLD OUT` and `ON SALE` are PRESENT TENSE, so a
 * cancelled or past show makes neither: `SOLD OUT` asserts the event is
 * happening and tickets are gone, `ON SALE` that they can be bought.
 * `SOLD OUT` swaps `ON SALE` per the mock; `ON SALE` requires somewhere to
 * actually buy — it branches on {@link ticketHref}, the same derivation the
 * Buy Tickets bracket renders from, so the words and the affordance cannot
 * drift. The price half is {@link ticketPriceSegments}: one price renders
 * bare, an advance/door pair renders as the mock's `$35 ADV · DOOR $40`. The
 * mock's trailing `CASH` is a separate fact with no column and no source that
 * states it reliably, so the line does not claim it. Both numbers are read
 * again by {@link hasTicketCommerce}, which decides whether the past register
 * has anything to close out.
 *
 * A past show closes the line instead: `$35 · NO LONGER AVAILABLE`, the
 * register of the locked PAST show-page mock. It sits AFTER the price
 * rather than in the sale state's slot because that is the mock's order and
 * because it reads as what it is — a record of what entry cost, then the
 * fact that the door has shut. {@link saysPastRegister} decides whether this
 * line is in that register at all and {@link hasTicketCommerce} whether it
 * has anything to close out; cancellation and an unreadable date are handled
 * in the former, with the reasons.
 *
 * The age segment is a venue-less fallback: the venue module's facts line
 * owns the age fact, but a show with no venue row never mounts that module,
 * and losing "21+" from the page is a fact a reader plans around (the old
 * meta row printed it unconditionally).
 */
export function ticketLineSegments(
  show: ShowResponse,
  lifecycle: ShowLifecycleState
): string[] {
  const timing = showTimingInput(show)
  const segments: string[] = []
  const startTime = startTimeFactSegment({
    eventDate: show.event_date,
    state: timing.state,
    timezone: timing.timezone,
  })
  if (startTime) {
    segments.push(startTime)
  }
  if (saysSoldOut(show, lifecycle)) {
    segments.push('SOLD OUT')
  } else if (ticketHref(show, lifecycle)) {
    segments.push('ON SALE')
  }
  segments.push(...ticketPriceSegments(show))
  if (saysPastRegister(show, lifecycle) && hasTicketCommerce(show)) {
    segments.push('NO LONGER AVAILABLE')
  }
  const age = show.age_requirement?.trim()
  if (show.venues.length === 0 && age) {
    segments.push(age)
  }
  return segments
}
