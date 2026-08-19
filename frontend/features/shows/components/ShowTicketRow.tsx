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
import { ticketLineSegments } from './showTicketLine'
import type { ShowResponse } from '../types'

/**
 * The stored ticket URL repaired into something navigable, or null.
 *
 * Submitters type scheme-less hosts ("tix.example/1") and vendors print
 * uppercase schemes; the scheme test is therefore case-insensitive and
 * anchored (`https?://`), not a bare prefix check — `startsWith('http')`
 * passed "httpfoo.example" through as a RELATIVE href that navigated under
 * /shows/. Protocol-relative values keep their own scheme resolution.
 */
function repairedTicketHref(ticketUrl: string): string {
  if (/^https?:\/\//i.test(ticketUrl)) return ticketUrl
  if (ticketUrl.startsWith('//')) return `https:${ticketUrl}`
  return `https://${ticketUrl}`
}

interface ShowTicketRowProps {
  show: ShowResponse
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
 */
export function ShowTicketRow({ show }: ShowTicketRowProps) {
  const segments = ticketLineSegments(show)
  const showTitle = showDisplayTitle(
    show.title,
    show.artists.map(artist => artist.name)
  )
  const ticketHref = show.ticket_url ? repairedTicketHref(show.ticket_url) : null

  return (
    <div data-testid="show-ticket-row">
      <MiddotSegments
        segments={segments}
        data-testid="ticket-line"
        className="font-mono text-sm font-medium tabular-nums"
      />

      <div className="mt-2 flex flex-wrap items-baseline gap-x-4 gap-y-1">
        {ticketHref && (
          <BracketLink
            label="Buy Tickets ↗"
            href={ticketHref}
            external
            ariaLabel="Buy tickets (opens in a new tab)"
          />
        )}
        <ShowAddToCalendar show={show} />
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
