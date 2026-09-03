import { EntityDetailContainer } from '@/components/shared'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'
import { showTimingInput } from '../utils'
import { buildShowStatusStripeSegments } from './showStatusStripeCopy'

interface ShowStatusStripeProps {
  show: ShowResponse
  /**
   * Where the show sits on the venue's calendar, computed on the SERVER by
   * `getShowLifecycleState` and threaded down. Never recomputed here: the
   * reader's clock is not the venue's, and a value that changed between render
   * and hydration would move the whole page.
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
 * Full-bleed background with the shared detail-page gutter inside it, so the
 * text starts on the same line as the breadcrumb and everything below.
 *
 * TONAL, not inverted: the muted surface and foreground text of the page's own
 * palette, bounded by hairline rules in the border token. The band separates
 * itself from the page by tint rather than by reversing the page's contrast.
 *
 * `min-h-11` rather than a hard height so the longest state (TONIGHT with
 * doors, music and the estimated end) can wrap on a narrow screen instead of
 * clipping. The band is server-rendered from server-computed state, so its
 * height is settled before hydration. That rests on the route seeding the show
 * into the query cache (`prefetchEntity`); without that seed `ShowDetail`
 * would paint its spinner first and the band would arrive at hydration and
 * shove the page.
 */
export function ShowStatusStripe({ show, lifecycle }: ShowStatusStripeProps) {
  const segments = buildShowStatusStripeSegments({
    ...showTimingInput(show),
    doorsAt: show.doors_at,
    musicAt: show.music_at,
    isCancelled: show.is_cancelled,
    lifecycle,
  })

  if (segments.length === 0) return null

  return (
    <div
      // A live region because this text changes under a reader: an admin
      // marking a show cancelled swaps the band's whole statement without
      // navigating, and the destructive alert this replaced announced itself
      // (`role="alert"`) when it appeared. Polite, not assertive: it is a
      // status line, not an interruption.
      role="status"
      aria-live="polite"
      data-testid="show-status-stripe"
      className="w-full border-y border-border bg-muted text-foreground"
    >
      <EntityDetailContainer className="flex min-h-11 flex-wrap items-center gap-x-3 gap-y-1 py-2 font-mono text-[11px] uppercase tracking-[1.4px] sm:text-xs">
        {segments.map((segment, index) => (
          // The separator is bonded to the segment that FOLLOWS it rather than
          // being a sibling, so a wrap can never strand a middot at the end of
          // a line with nothing after it.
          //
          // The middot is decorative and hidden, which is why the leading space
          // is a real character in the text: without it the accessible name
          // runs the segments together as "TONIGHTDOORS 7PM". The space is a
          // whitespace-only child of a flex container, so it costs no layout.
          <span key={segment} className="flex items-center gap-x-3">
            {index > 0 && (
              <span aria-hidden="true" className="text-muted-foreground">
                &middot;
              </span>
            )}
            {index > 0 ? ` ${segment}` : segment}
          </span>
        ))}
      </EntityDetailContainer>
    </div>
  )
}
