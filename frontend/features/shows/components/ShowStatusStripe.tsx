import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'
import {
  buildShowStatusStripeSegments,
  showStatusStripeZone,
} from './showStatusStripeCopy'

interface ShowStatusStripeProps {
  show: ShowResponse
  /**
   * Where the show sits on the venue's calendar, computed ON THE SERVER by
   * `getShowLifecycleState` and threaded down. Never recomputed here: a client
   * clock would put a Berlin reader's midnight on a Phoenix show, and a value
   * that changed between render and hydration would move the whole page.
   */
  lifecycle: ShowLifecycleState
}

/**
 * The one band at the top of a show page that says where the show sits in
 * time: TONIGHT, a plain upcoming date, PAST SHOW, or CANCELLED.
 *
 * Same position, same height, every state, so the eye lands in one place and
 * the page below never moves. Typographic and unadorned by design: no icon, no
 * badge, no color-coded severity. The newsprint register does the work, and a
 * stamp that changes shape per state stops reading as one thing.
 *
 * `min-h-11` rather than a hard height so the longest state (TONIGHT with
 * doors, music and the estimated end) can wrap on a narrow screen instead of
 * clipping. The band is server-rendered from server-computed state, so its
 * height is settled before hydration either way.
 */
export function ShowStatusStripe({ show, lifecycle }: ShowStatusStripeProps) {
  const segments = buildShowStatusStripeSegments({
    eventDate: show.event_date,
    doorsAt: show.doors_at,
    musicAt: show.music_at,
    isCancelled: show.is_cancelled,
    lifecycle,
    ...showStatusStripeZone(show),
  })

  if (segments.length === 0) return null

  return (
    <div
      data-testid="show-status-stripe"
      className="w-full bg-foreground text-background"
    >
      <div className="container mx-auto flex min-h-11 max-w-6xl flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 font-mono text-[11px] uppercase tracking-[1.4px] sm:text-xs">
        {segments.map((segment, index) => (
          <span key={segment} className="flex items-center gap-x-3">
            {index > 0 && (
              <span aria-hidden="true" className="text-background/50">
                &middot;
              </span>
            )}
            {segment}
          </span>
        ))}
      </div>
    </div>
  )
}
