import { showTimingInput } from '../utils'
import { startTimeFactSegment } from './showStatusStripeCopy'
import { saysSoldOut } from './showSaleState'
import { showIsArchived } from '@/lib/utils/showTiming'
import { formatPrice } from '@/lib/utils/formatters'
import { statedShowPrices } from '@/lib/utils/showPrice'
import {
  carriesOurAffiliateTag,
  repairTicketUrl,
  ticketLink,
  ticketVendorLabel,
} from '@/lib/tickets/ticketVendors'
import type { TicketLink } from '@/lib/tickets/ticketVendors'
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
 * branches on it and {@link ticketOffer} consumes it, so a refusal added
 * here reaches both.
 *
 * The repair itself is {@link repairTicketUrl}, shared with the festival
 * page's ticket link so the two surfaces cannot disagree about what a stored
 * value means. This function owns only the refusals above.
 *
 * NOT the href a buy affordance renders, and NOT exported for exactly that
 * reason. This answers "may this be offered, and at what URL", which is the
 * question the `ON SALE` words ask; the URL it returns is UNTAGGED and
 * carries no paid-referral gate, so a Buy Tickets surface built on it would
 * ship an unmonetized, unqualified link and no test would notice.
 * {@link ticketOffer} is the exported one, and is what a new surface should
 * render.
 */
function ticketHref(
  show: ShowResponse,
  lifecycle: ShowLifecycleState
): string | null {
  const raw = storedTicketUrl(show)
  if (!raw || show.is_cancelled || show.is_sold_out || lifecycle === 'past') {
    return null
  }
  return repairTicketUrl(raw)
}

/**
 * What the ticket surface renders for a stored ticket URL: the resolved link,
 * whether an outbound anchor is offered at all, and the vendor's name for when
 * it is not. Null when there is nothing to offer.
 *
 * Two steps that stay separate on purpose. {@link ticketHref} answers "may
 * this show be offered, and at what URL" and is also what the `ON SALE` words
 * branch on; {@link ticketLink} answers "is this vendor's link monetized",
 * which the words have no opinion about. Composing them here rather than in
 * the component keeps every derivation of the Buy Tickets href in this file.
 *
 * `linked` is the site's PAID-REFERRAL RULE, and it is the whole reason this
 * shape exists: an outbound vendor anchor renders only when the click is paid
 * for ({@link carriesOurAffiliateTag}), so a vendor with no affiliate entry,
 * a network with no partner ID configured, and a URL carrying a tag somebody
 * else planted all render as the vendor's NAME instead of a link.
 *
 * FREE ADMISSION IS THE EXEMPTION ({@link isFreeAdmission}): an RSVP or
 * guestlist link on a show that states a price of zero is not a ticket
 * referral, nobody is paid either way, and the click is the reader's only
 * route in. It stays linked whatever the vendor is.
 *
 * `link` is present even when `linked` is false, because the planted-tag
 * report is about the stored value rather than about what this page renders.
 */
export interface TicketOffer {
  link: TicketLink
  linked: boolean
  /** Null only for a value that yields no host; see {@link ticketVendorLabel}. */
  vendorName: string | null
}

export function ticketOffer(
  show: ShowResponse,
  lifecycle: ShowLifecycleState
): TicketOffer | null {
  const href = ticketHref(show, lifecycle)
  if (href === null) return null
  const link = ticketLink(href)
  return {
    link,
    linked: isFreeAdmission(show) || carriesOurAffiliateTag(link),
    vendorName: ticketVendorLabel(href),
  }
}

/**
 * Whether admission is FREE: the show states a price and every price it states
 * is zero.
 *
 * Stating zero and stating nothing are different facts, which is why this
 * reads {@link statedShowPrices} rather than testing `show.price`. A show with
 * no price recorded is unpriced, not free, and an unpriced show's ticket link
 * is a vendor referral like any other.
 *
 * Both columns count. A show with a zero advance price and a real door price
 * charges for entry.
 */
function isFreeAdmission(show: ShowResponse): boolean {
  const prices = statedShowPrices(show)
  return prices.length > 0 && prices.every(price => price === 0)
}

/**
 * `[]`, `['$35']`, or the mock's split pair `['$35 ADV', 'DOOR $40']`
 * (PSY-1864).
 *
 * WHICH prices there are to spell is {@link statedShowPrices}, shared with
 * every dense list on the site, so the page and the card a reader arrived from
 * cannot disagree about whether this show has one price or two. The collapse
 * rules and the reasoning behind them live there. This function owns only the
 * DETAIL register: the mock's qualified pair.
 *
 * `ADV` / `DOOR` are disambiguators, so they are spelled only when there ARE
 * two different numbers to tell apart. A lone price renders bare — including a
 * lone DOOR price, where the word would distinguish the number from nothing.
 * Segments rather than one string because the pair is two middot-separated
 * facts in the mock, not a compound one; a dense list has no room for that and
 * renders `$35/$40` instead ({@link showPriceLabel}).
 *
 * The amounts themselves go through the site-wide {@link formatPrice}. This
 * line used to carry its own whole-dollar copy of it, which is how a /shows
 * card ($25.00) and the page it opened ($25) came to spell one price two ways.
 */
function ticketPriceSegments(show: ShowResponse): string[] {
  const prices = statedShowPrices(show)
  if (prices.length === 2) {
    const [advance, door] = prices
    return [`${formatPrice(advance)} ADV`, `DOOR ${formatPrice(door)}`]
  }
  return prices.map(formatPrice)
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
 * easily forgets, being neither a price nor a link. A show marked sold out
 * can carry neither of the other two, and its line says `SOLD OUT` until the
 * show is over; without this test that line falls to silence rather than to
 * the past tense of the claim it was making.
 *
 * BOTH PRICES COUNT: a show sold only at the door still charged for entry.
 *
 * False for the free show with no link and no sold-out flag, and for the show
 * carrying none of them: `NO LONGER AVAILABLE` is the past tense of
 * `ON SALE`, and a line that never said anything has nothing to un-say.
 */
function hasTicketCommerce(show: ShowResponse): boolean {
  if (storedTicketUrl(show) || show.is_sold_out) return true
  // The same zero that {@link formatPrice} renders as "Free"; keep the two
  // in step if a price ever becomes something other than a plain number.
  return (show.price ?? 0) > 0 || (show.door_price ?? 0) > 0
}

/**
 * Whether this line speaks in the PAST register at all: {@link showIsArchived}
 * applied to a `ShowResponse`.
 *
 * A thin adapter and nothing more. The rule has other callers, so conditions
 * belong in the shared predicate, never here — a condition added to this
 * wrapper applies to one line and silently not to the rest.
 *
 * NO condition on the venue timezone, which looks wrong beside
 * {@link startTimeFactSegment} above: that one refuses to print an hour on a
 * guessed zone, because a confidently wrong hour is worse than none. The rule
 * does not transfer. A guessed zone ruins an hour but still yields a date, and
 * the stripe prints its past state from the same guess — testing it here would
 * leave the page saying PAST SHOW at the top while this line declined to close
 * beneath it.
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
 * actually buy — it branches on {@link ticketHref}, the same nullability
 * {@link ticketOffer} derives from, so the words and the affordance cannot
 * drift. `ON SALE` is a claim about the SHOW, so it is independent of whether
 * this site links out: an unpaid referral is withheld, and tickets are still
 * on sale. The price half is {@link ticketPriceSegments}: one price renders
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
 * The VENDOR segment is what an unpaid referral leaves behind: when
 * {@link ticketOffer} refuses the outbound anchor, the line still names who
 * sells the ticket (`$25 · TicketWeb`), so a reader knows where to go. It is
 * absent whenever the anchor renders, which already names the vendor by being
 * clickable, and absent on every state that has nothing to offer.
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
  const offer = ticketOffer(show, lifecycle)
  if (offer && !offer.linked && offer.vendorName) {
    segments.push(offer.vendorName)
  }
  if (saysPastRegister(show, lifecycle) && hasTicketCommerce(show)) {
    segments.push('NO LONGER AVAILABLE')
  }
  const age = show.age_requirement?.trim()
  if (show.venues.length === 0 && age) {
    segments.push(age)
  }
  return segments
}
