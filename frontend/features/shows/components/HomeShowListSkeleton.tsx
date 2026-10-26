import { Skeleton } from '@/components/ui/skeleton'
import { HOME_SHOW_LIMIT } from '../api'

/**
 * The home upcoming-shows list's placeholder while its server seed is still
 * streaming: a blank band where the filter chips will sit, then one row per
 * seeded show in a real row's shape.
 *
 * Static by design. It renders no `HomeShowList` and calls no query hook, so it
 * registers nothing in the request's query client; a registered entry would
 * make the seed's `HydrationBoundary` defer that key and the streamed list
 * would render its loader instead of rows.
 *
 * The band mirrors the chips block's box rather than its content: the same
 * `mb-6` and `flex flex-col gap-2`, one line box sized like the filter trigger
 * (`text-sm`, `py-1.5`, a 1px border) and one `text-xs` line for "Popular:".
 * It is one popular line tall; where that line wraps (narrow viewports, long
 * city names) the real block is taller by a line per wrap.
 *
 * `h-[90px]` is the height of a `ShowCard` with one title line and no price or
 * support line: from `sm` with a one-line venue, below `sm` with a venue line
 * that wraps once. A row with a price or support line, or a longer wrap, is
 * taller. The surfaces are the list row's own: `bg-card`, a `border-border/50`
 * border, `rounded-lg`, and the `bg-muted/50` date badge.
 */
export function HomeShowListSkeleton() {
  return (
    <div
      className="w-full"
      aria-hidden="true"
      data-testid="home-show-list-skeleton"
    >
      <div className="mb-6" data-testid="home-show-list-skeleton-chips">
        <div className="flex flex-col gap-2">
          <div className="box-content h-lh border border-transparent py-1.5 text-sm" />
          <div className="h-lh text-xs" />
        </div>
      </div>
      <div className="flex flex-col gap-3">
        {Array.from({ length: HOME_SHOW_LIMIT }, (_, i) => (
          <div
            key={i}
            data-testid="home-show-list-skeleton-row"
            className="h-[90px] rounded-lg border border-border/50 bg-card px-3 py-3 sm:px-4 sm:py-4"
          >
            <div className="flex h-full gap-3 sm:gap-4">
              <div className="w-14 shrink-0 rounded-md bg-muted/50 sm:w-16" />
              <div className="flex min-w-0 flex-1 flex-col gap-2 pt-1">
                <Skeleton className="h-5 w-2/3 max-w-80" />
                <Skeleton className="h-3.5 w-1/2 max-w-60" />
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
