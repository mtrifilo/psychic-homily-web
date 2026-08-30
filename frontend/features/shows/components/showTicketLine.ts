import { showTimingInput } from '../utils'
import { startTimeFactSegment } from './showStatusStripeCopy'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'

/**
 * The stored ticket URL repaired into something navigable, or null when
 * there is nothing to buy or no honest way to offer it.
 *
 * Null for a blank/whitespace value (storable: the backend persists the
 * field untrimmed and ingest paths skip the handler validator), for a
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
  const raw = show.ticket_url?.trim()
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
 * stripe's own precedence. Both claims are PRESENT TENSE, so a cancelled or
 * past show makes neither: `SOLD OUT` asserts the event is happening and
 * tickets are gone, `ON SALE` that they can be bought — the fuller past
 * register (`NO LONGER AVAILABLE`) belongs to the past-register ticket.
 * `SOLD OUT` swaps `ON SALE` per the mock; `ON SALE` requires somewhere to
 * actually buy — it branches on {@link ticketHref}, the same derivation the
 * Buy Tickets bracket renders from, so the words and the affordance cannot
 * drift. The price half is {@link ticketPriceSegments}: one price renders
 * bare, an advance/door pair renders as the mock's `$35 ADV · DOOR $40`. The
 * mock's trailing `CASH` is a separate fact with no column and no source that
 * states it reliably, so the line does not claim it (PSY-1864).
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
  if (!show.is_cancelled && lifecycle !== 'past') {
    if (show.is_sold_out) {
      segments.push('SOLD OUT')
    } else if (ticketHref(show, lifecycle)) {
      segments.push('ON SALE')
    }
  }
  segments.push(...ticketPriceSegments(show))
  const age = show.age_requirement?.trim()
  if (show.venues.length === 0 && age) {
    segments.push(age)
  }
  return segments
}
