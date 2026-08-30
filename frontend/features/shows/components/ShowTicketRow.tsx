'use client'

import {
  AddToCollectionButton,
  BracketLink,
  SaveButton,
  ShareButton,
} from '@/components/shared'
import { showDisplayTitle } from '@/lib/utils/showDisplayTitle'
import { MiddotSegments } from './MiddotSegments'
import { ShowAddToCalendar } from './ShowAddToCalendar'
import { ticketHref, ticketLineSegments } from './showTicketLine'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'

interface ShowTicketRowProps {
  show: ShowResponse
  /**
   * Server-computed, threaded from the route (see ShowDetail's prop of the
   * same name). The sale-state segment is a present-tense claim, and only
   * the lifecycle can say whether the present tense applies.
   */
  lifecycle: ShowLifecycleState
}

/**
 * The ticket module of the show page: the how-much line and every verb a
 * reader can act on, in the locked mock's order — `[Buy Tickets ↗]
 * [Add to calendar] [Save] [Add to collection] [Share]`. (`[Share]` is a
 * kept affordance the mock omits — user decision. `[Add to calendar]`
 * sitting in the verb row rather than on a section header is also the
 * mock's call, superseding the syndication-affordance placement rule for
 * this surface; it exports one event, not a feed.)
 *
 * The calendar/save coupling is unchanged in kind but now adjacent:
 * `ShowAddToCalendar` saves the show as a side effect and dedupes against
 * `SaveButton` through the shared query key. They used to sit rows apart
 * with a warning to check both when moving either; they now share this row,
 * which is the easier invariant to keep.
 *
 * PAST REGISTER (PSY-1690): the two forward-looking verbs drop out and the
 * rest of the row stays, which is the locked PAST mock's row minus the
 * attendance affordance it draws in their place. Neither refusal is written
 * here — `[Buy Tickets ↗]` goes via {@link ticketHref} and
 * `[Add to calendar]` via `ShowAddToCalendar`'s own past guard, each beside
 * the thing it governs, so this row states the mock's ORDER and nothing
 * about when a verb applies. Both carry their reason at the definition.
 *
 * `[Save]` deliberately STAYS. A past show is an archive document, demoted
 * rather than deleted, and saving one is how a reader keeps it: /shows/saved
 * lists past saves, and withdrawing the verb here would orphan every save
 * already made the moment the show ended. The mock's past row shows
 * `[I was there]` in this neighbourhood instead, but attendance is an
 * explicit non-goal of this ticket (privacy default undecided), and its
 * absence is not a reason to withdraw a working affordance beside it.
 */
export function ShowTicketRow({ show, lifecycle }: ShowTicketRowProps) {
  const segments = ticketLineSegments(show, lifecycle)
  const showTitle = showDisplayTitle(
    show.title,
    show.artists.map(artist => artist.name)
  )
  // The one derivation of "is there somewhere to buy" (showTicketLine):
  // null for cancelled, sold-out, and past shows, so neither the sale-state
  // words nor this bracket can argue with the stripe.
  const buyHref = ticketHref(show, lifecycle)

  return (
    <div data-testid="show-ticket-row">
      <MiddotSegments
        segments={segments}
        data-testid="ticket-line"
        className="font-mono text-sm font-medium tabular-nums"
      />

      <div className="mt-2 flex flex-wrap items-baseline gap-x-4 gap-y-1">
        {buyHref && (
          // Keeps the pre-existing announced name: the ↗ is a VISUAL outbound
          // marker, and letting it into the accessible name has a screen
          // reader read "north east arrow" right before the suffix says the
          // same thing in words. Only the new-tab claim moved to BracketLink.
          <BracketLink
            label="Buy Tickets ↗"
            href={buyHref}
            external
            ariaLabel="Buy tickets"
          />
        )}
        <ShowAddToCalendar show={show} lifecycle={lifecycle} />
        {/* SaveButton's bracket branch defaults to the header-linkbox
            treatment (11px mono); this row is 14px sans, and one odd bracket
            mid-row reads as a mistake. The count display the Button variant
            carried is deliberately absent here — the mock's row has no
            counts; social proof belongs to the attendance module. */}
        <SaveButton
          showId={show.id}
          variant="bracket"
          className="font-sans text-sm"
        />
        <AddToCollectionButton
          entityType="show"
          entityId={show.id}
          entityName={showTitle}
          variant="bracket"
        />
        <ShareButton
          path={show.slug ? `/shows/${show.slug}` : null}
          variant="bracket"
          ariaLabel="Share this show"
        />
      </div>
    </div>
  )
}
