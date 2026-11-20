'use client'

import { useRef, useState } from 'react'
import {
  BRACKET_LABEL_LAYOUT_CLASS,
  BracketLabel,
  BracketLink,
} from '@/components/shared/BracketLink'
import { useCalendarFeedStatus } from '@/features/auth/hooks/useCalendarFeed'
import {
  summarizeCalendar,
  type CalendarSummary,
} from '@/features/collections/lib/calendarFeedStatus'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { cn } from '@/lib/utils'
import {
  LazySavedShowsCalendarPanel,
  preloadSavedShowsCalendarPanel,
} from './LazySavedShowsCalendarPanel'

/**
 * The link's label for each calendar summary, verbatim from the owner-approved
 * board spec (docs/research/calendar-subscribe-v4.1-board-spec-2026-11.md,
 * kept outside version control).
 */
export const CALENDAR_LINK_LABELS: Record<CalendarSummary, string> = {
  none: 'Add to calendar',
  connected: 'Calendar: connected',
  stopped: 'Calendar: stopped',
}

const CALENDAR_LINK_LABEL_LIST = Object.values(CALENDAR_LINK_LABELS)

/** The link's type, shared by the invisible copies that reserve its width. */
const LINK_TYPE_CLASS = 'font-data text-xs'

/**
 * The fragment that opened the Library's calendar card before the card was
 * replaced by this link. Arriving on a page carrying it opens the panel once.
 */
const CALENDAR_ARRIVAL_HASH = '#calendar-feed'

/**
 * Where the link sits, as drawn on the boards:
 *
 * - `section-header`: the end of a list's section header (Library Upcoming).
 *   Below `sm` the header wraps it onto a line of its own, label at the
 *   start; from `sm` up it follows the header's controls, label at the end.
 * - `module-footer`: a right-aligned footer row (the signed-in home's saved
 *   shows), label at the end.
 *
 * Colours as drawn: a stopped calendar is in the link colour in both places
 * and a connected one is muted in both; the no-calendar label is in the link
 * colour in a header and muted in the home footer, beside its muted lines.
 */
type CalendarLinkPlacement = 'section-header' | 'module-footer'

const PLACEMENT_SLOT_CLASS: Record<CalendarLinkPlacement, string> = {
  'section-header': 'w-full sm:w-auto',
  'module-footer': '',
}

const PLACEMENT_LINK_CLASS: Record<CalendarLinkPlacement, string> = {
  'section-header': 'justify-self-start sm:justify-self-end',
  'module-footer': 'justify-self-end',
}

function linkVariant(
  summary: CalendarSummary,
  placement: CalendarLinkPlacement
): 'default' | 'accent' {
  if (summary === 'stopped') return 'accent'
  if (summary === 'none' && placement === 'section-header') return 'accent'
  return 'default'
}

interface SavedShowsCalendarLinkProps {
  placement: CalendarLinkPlacement
  /**
   * Open the panel when the page was reached through the calendar card's old
   * fragment, so links and bookmarks made for the card still reach the
   * calendar. Closing the panel drops the fragment, so it opens once.
   *
   * Opening the panel prepares the account's calendar link, creating one when
   * the account has none (useSavedShowsCalendarLink), so arriving through the
   * fragment creates a link with no click of its own.
   */
  opensFromHash?: boolean
}

/**
 * The bracket link that reports the saved-shows calendar's state and opens
 * the calendar panel in place.
 *
 * Its label follows `summarizeCalendar`, read from the status that carries no
 * feed address. The server, and every render before the status arrives,
 * shows the no-calendar label: the link is in server HTML, so nothing appears
 * at hydration.
 *
 * The slot holds an invisible copy of every label in one grid cell, so it is
 * always as wide as the longest and the controls beside it never move when the
 * label changes (a late change of width in a right-aligned row drags its
 * neighbours under a pointer about to click them). Measured by the browser
 * rather than fixed in pixels, so it holds in any font.
 */
export function SavedShowsCalendarLink({
  placement,
  opensFromHash = false,
}: SavedShowsCalendarLinkProps) {
  const { authStatus, user } = useAuthContext()
  const viewerId = user?.id
  const { data: status } = useCalendarFeedStatus(authStatus === 'authenticated')
  // The window is read at render, as the panel reads it: the label is as
  // fresh as the surface's last render and does not tick on its own.
  const summary = summarizeCalendar(status, new Date())
  const label = CALENDAR_LINK_LABELS[summary]

  const slotRef = useRef<HTMLDivElement>(null)
  const linkRef = useRef<HTMLButtonElement>(null)
  const hash = useUrlHash()
  // The panel acts for one account and closes when the viewer changes. The
  // person's own choice is kept with the viewer who made it; until there is
  // one, the page's fragment decides for the viewer the link mounted for.
  const [choice, setChoice] = useState<{
    open: boolean
    viewerId: string | undefined
  } | null>(null)
  const [arrivalViewerId] = useState(viewerId)
  const open =
    choice === null
      ? opensFromHash &&
        hash === CALENDAR_ARRIVAL_HASH &&
        arrivalViewerId === viewerId
      : choice.open && choice.viewerId === viewerId
  // The panel mounts on the first open and stays mounted, so it closes with
  // its own animation and focus return. Adjusted during render.
  const [panelMounted, setPanelMounted] = useState(false)
  if (open && !panelMounted) setPanelMounted(true)

  const setOpen = (next: boolean) => {
    setChoice({ open: next, viewerId })
    if (!next && window.location.hash === CALENDAR_ARRIVAL_HASH) {
      window.history.replaceState(
        null,
        '',
        window.location.pathname + window.location.search
      )
    }
  }

  return (
    <div
      ref={slotRef}
      className={cn('grid', PLACEMENT_SLOT_CLASS[placement])}
      data-testid="saved-shows-calendar-link-slot"
    >
      {CALENDAR_LINK_LABEL_LIST.map(sizerLabel => (
        <span
          key={sizerLabel}
          aria-hidden="true"
          className={cn(
            BRACKET_LABEL_LAYOUT_CLASS,
            LINK_TYPE_CLASS,
            'invisible col-start-1 row-start-1'
          )}
          data-calendar-link-sizer=""
        >
          <BracketLabel label={sizerLabel} />
        </span>
      ))}
      <BracketLink
        ref={linkRef}
        label={label}
        variant={linkVariant(summary, placement)}
        onClick={() => setOpen(!open)}
        onPointerEnter={preloadSavedShowsCalendarPanel}
        onFocus={preloadSavedShowsCalendarPanel}
        aria-haspopup="dialog"
        aria-expanded={open}
        className={cn(
          LINK_TYPE_CLASS,
          'col-start-1 row-start-1',
          PLACEMENT_LINK_CLASS[placement]
        )}
      />
      {panelMounted ? (
        <LazySavedShowsCalendarPanel
          open={open}
          onOpenChange={setOpen}
          anchorRef={slotRef}
          // The link itself, not the slot: a press on the slot's empty
          // reserved width is a press outside.
          triggerRef={linkRef}
          align="end"
        />
      ) : null}
    </div>
  )
}
