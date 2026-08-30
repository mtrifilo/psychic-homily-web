import type { ShowLifecycleState } from '@/lib/utils/showTiming'

/**
 * Whether a surface may say `SOLD OUT` about this show.
 *
 * Its own module, and a STRUCTURAL input type rather than `ShowResponse`, so
 * the rule can be imported by a surface that does not carry the full show
 * payload — the share-card route builds a narrow `ShowData` of its own, and
 * an edge route has a hard bundle ceiling that pulling a feature module's
 * import graph would eat into. It lives here rather than in `showTicketLine`
 * because the ticket line is only one of its callers; a header badge whose
 * rule is filed under the ticket line is a rule nobody greps for.
 *
 * `SOLD OUT` asserts two things at once — that the event is happening, and
 * that tickets are gone — so both a cancellation and an elapsed show
 * withdraw it. Neither claim survives the show: a past sold-out show
 * printing the badge over `NO LONGER AVAILABLE` is one page arguing with
 * itself, and printing it under a `CANCELLED` stripe contradicts the one
 * fact a reader must not miss.
 *
 * KNOWN SCOPE, so the next reader does not over-trust this. It governs the
 * show DETAIL page: the ticket line's segment and the header badge beside
 * the date. It does NOT yet govern two other surfaces that badge sold-out
 * shows, and both are deliberate for now rather than overlooked:
 *
 * - `ShowBill`'s badge, rendered by the venue and artist PAST-SHOWS tables,
 *   which have a pinned test asserting the badge appears there. A listing
 *   row is a different claim from a detail page's status band, and changing
 *   it is a product call with its own test to rewrite.
 * - The show route's own `opengraph-image`, which still reads the flag raw.
 *   That one IS a straightforward inconsistency, not a considered split, and
 *   it is the more costly of the two because the card is cached long once
 *   the show has settled. Left out of the change that introduced this
 *   predicate to keep an edge-runtime route out of a page-behaviour diff;
 *   it is the obvious next caller.
 */
export function saysSoldOut(
  show: { is_sold_out: boolean; is_cancelled: boolean },
  lifecycle: ShowLifecycleState
): boolean {
  return show.is_sold_out && !show.is_cancelled && lifecycle !== 'past'
}
