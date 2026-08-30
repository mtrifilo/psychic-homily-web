'use client'

import { useState } from 'react'
import { ExternalLink, Download } from 'lucide-react'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Checkbox } from '@/components/ui/checkbox'
import { BracketLink } from '@/components/shared/BracketLink'
import { useAuthContext } from '@/lib/context/AuthContext'
import { googleCalendarEventUrl } from '@/lib/utils/calendarFeedUrls'
import { showDisplayTitle } from '@/lib/utils/showDisplayTitle'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { API_BASE_URL } from '@/lib/api-base'
import { useSaveShow, useShowSaveCount } from '../hooks/useSavedShows'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'

/**
 * Assumed show length for the Google Calendar link — mirrors the backend's
 * defaultShowDuration so the two export paths never disagree about when a
 * show ends.
 *
 * A THIRD number exists and is deliberately not this one: the status stripe's
 * `ESTIMATED_SHOW_LENGTH_HOURS` is four hours from DOORS rather than three
 * from the start, because it was specified as reader-facing copy rather than
 * as a calendar block. Both can render on one show page. Do not unify them
 * without deciding which number is right; see that constant's comment.
 */
const SHOW_DURATION_MS = 3 * 60 * 60 * 1000

/**
 * Builds the absolute URL of the show's one-shot .ics download.
 *
 * It has to be absolute for the same reason the feed URLs are: the link is a
 * download target that may be copied out of the browser. `API_BASE_URL` is the
 * relative `/api` in a development browser (proxied same-origin for cookie
 * reasons), which still resolves — the Next catch-all proxy forwards `/api/*`
 * and passes the calendar content-type through — it just has to be made
 * absolute first. SSR-safe: on the server the relative branch yields a
 * root-relative URL, and nothing renders it before the popover opens anyway.
 */
export function showCalendarIcsUrl(slugOrId: string): string {
  const base = API_BASE_URL.replace(/\/$/, '')
  const path = `/shows/${encodeURIComponent(slugOrId)}/calendar.ics`

  if (/^https?:\/\//.test(API_BASE_URL)) {
    return `${base}${path}`
  }

  const origin = typeof window === 'undefined' ? '' : window.location.origin
  return `${origin}${base}${path}`
}

/**
 * Builds the Google Calendar template link from the show's response fields.
 *
 * Title decoration (CANCELLED: prefix, [SOLD OUT] suffix) mirrors the
 * backend's applyEventSummaryAndStatus so this path and the .ics download
 * describe the same event the same way — Google's template URL has no
 * STATUS field, so the prefix is the only cancellation signal it can carry.
 *
 * `details` uses the canonical SITE_URL, never window.location: a calendar
 * entry outlives deployments, and one created from a preview deploy must not
 * carry a preview-host link.
 */
export function showGoogleCalendarUrl(show: ShowResponse): string {
  const venue = show.venues[0]
  const start = new Date(show.event_date)
  const locationParts = venue
    ? [venue.name, venue.address, venue.city, venue.state].filter(
        (part): part is string => !!part && part.trim() !== ''
      )
    : []

  let title = showDisplayTitle(
    show.title,
    show.artists.map(a => a.name)
  )
  if (show.is_cancelled) {
    title = `CANCELLED: ${title}`
  } else if (show.is_sold_out) {
    title = `${title} [SOLD OUT]`
  }

  const showUrl = `${SITE_URL}/shows/${show.slug || show.id}`

  return googleCalendarEventUrl({
    title,
    start,
    end: new Date(start.getTime() + SHOW_DURATION_MS),
    details: show.is_cancelled
      ? `This show has been cancelled.\n${showUrl}`
      : showUrl,
    location: locationParts.join(', '),
  })
}

interface ShowAddToCalendarProps {
  show: ShowResponse
  /**
   * Server-computed, threaded from the route through ShowTicketRow. A PROP
   * and never derived here: this is a client component, so a local
   * `getShowLifecycleState` would read the READER's clock at the venue's day
   * boundary and could disagree with the stripe rendered above it in the
   * same HTML.
   */
  lifecycle: ShowLifecycleState
}

/**
 * "[Add to calendar]" popover for one show: a Google Calendar template link
 * and a one-shot .ics download. Lives in the ticket module's verb row (the
 * locked show mock's placement — see ShowTicketRow's docstring for the
 * supersession of the earlier meta-row convention).
 *
 * Renders NOTHING for a past show. The refusal lives here rather than in a
 * caller's JSX so that every mount inherits it: an appointment booked for a
 * show that is over lands in the reader's calendar app, where this page
 * cannot correct it.
 *
 * The boundary is the lifecycle's venue-local midnight, NOT the start
 * instant, so the verb survives the evening of the show: a reader can still
 * decide to go, and the page still reads as tonight's. The cost is a calendar
 * entry created after the set began, which is useless rather than misleading.
 *
 * Withdrawing this affordance is NOT access control — the export endpoint
 * governs its own access, and a URL somebody already holds still works.
 *
 * Signed-in viewers get an "Also save this show" checkbox, CHECKED by
 * default: the calendar action then also saves the show. The save is
 * fire-and-forget — a failed save must never block the calendar action,
 * which is the actual point of the click. Anonymous viewers get the calendar
 * actions plus a quiet sign-in hint; the calendar path is deliberately never
 * auth-gated.
 */
export function ShowAddToCalendar({
  show,
  lifecycle,
}: ShowAddToCalendarProps) {
  const [open, setOpen] = useState(false)
  const [alsoSave, setAlsoSave] = useState(true)
  const { isAuthenticated, user } = useAuthContext()

  // Deduped against the SaveButton on the same page via the shared query key;
  // only fetched once the popover opens.
  const { data: saveData } = useShowSaveCount(
    show.id,
    isAuthenticated,
    open && isAuthenticated,
    user?.id
  )
  const saveShow = useSaveShow()
  const isSaved = saveData?.is_saved ?? false

  // Below every hook, because React identifies hooks by call order and an
  // earlier return would make their count depend on `lifecycle` — but ABOVE
  // the two URL constants, which is not a nicety. `showGoogleCalendarUrl`
  // calls `.toISOString()` on the parsed `event_date`, which throws a
  // RangeError for a date it cannot read, and the lifecycle labels exactly
  // that show `past`. Built the URLs first and the component would crash on
  // the branch whose whole job is to render nothing.
  if (lifecycle === 'past') return null

  const icsUrl = showCalendarIcsUrl(show.slug || String(show.id))
  const googleUrl = showGoogleCalendarUrl(show)

  const handleCalendarAction = () => {
    if (isAuthenticated && alsoSave && !isSaved && !saveShow.isPending) {
      // Fire-and-forget: the anchor's own navigation/download proceeds
      // regardless. Errors are swallowed — the calendar action succeeded,
      // and the heart on the page still shows the true saved state.
      saveShow.mutate(show.id)
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <BracketLink label="Add to calendar" active={open} />
      </PopoverTrigger>
      <PopoverContent className="w-72 p-4" align="start">
        {isAuthenticated && !isSaved && (
          <label className="flex items-center gap-2 text-sm cursor-pointer">
            <Checkbox
              checked={alsoSave}
              onCheckedChange={checked => setAlsoSave(checked === true)}
              aria-label="Also save this show"
            />
            Also save this show
          </label>
        )}
        {!isAuthenticated && (
          <p className="text-xs text-muted-foreground">
            Sign in to save shows you&apos;re going to.
          </p>
        )}

        {(!isAuthenticated || !isSaved) && (
          <div className="border-t border-border mt-3 mb-3" />
        )}

        <div className="flex flex-col gap-2.5">
          <a
            href={googleUrl}
            target="_blank"
            rel="noopener noreferrer"
            onClick={handleCalendarAction}
            className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:text-primary/80"
          >
            Google Calendar
            <ExternalLink className="h-3.5 w-3.5" />
          </a>
          <a
            href={icsUrl}
            onClick={handleCalendarAction}
            className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:text-primary/80"
          >
            Apple / Outlook (.ics)
            <Download className="h-3.5 w-3.5" />
          </a>
        </div>
      </PopoverContent>
    </Popover>
  )
}
