import Link from 'next/link'
import { cn } from '@/lib/utils'
import {
  timelineCurrentPlaceLabel,
  timelineDateLabel,
  timelinePlaceLabel,
  timelineYear,
  type TimelineStop,
} from './showTimelineCopy'
import type { ShowTimelineEntry } from '../types'

/**
 * One dated stop on the spine: its date, where it was, and a link to it.
 *
 * Unlinked without a `show_slug`. The column is nullable and the backend sends
 * "" for a missing one, and `/shows/` is the shows INDEX rather than a 404, so
 * an unguarded href would take the reader off the page instead of failing
 * visibly.
 *
 * The place is inside the link, not beside it: the whole "AUG 9 METRO, CHICAGO"
 * group names one show, and splitting it would put two adjacent tab stops on
 * one fact.
 */
function SpineStop({
  entry,
  subjectYear,
  direction,
  className,
}: {
  entry: ShowTimelineEntry
  subjectYear: string
  /** Announced before the date, so the arrow glyph carries no meaning. */
  direction: 'Previous show' | 'Next show'
  className?: string
}) {
  const place = timelinePlaceLabel(entry)
  const label = [timelineDateLabel(entry, subjectYear), place]
    .filter(Boolean)
    .join(' ')

  const content = (
    <>
      {/* The space is its own text node OUTSIDE the span, matching the sr-only
          connectives on the bill above: accessible-name computation trims each
          node, so a trailing space inside the span is dropped and the direction
          runs into the date. */}
      <span className="sr-only">{direction}:</span>{' '}
      {label}
    </>
  )

  if (!entry.show_slug) {
    return <span className={cn('text-muted-foreground', className)}>{content}</span>
  }
  return (
    <Link
      href={`/shows/${entry.show_slug}`}
      className={cn(
        'text-muted-foreground transition-colors hover:text-foreground hover:underline focus-visible:text-foreground focus-visible:underline',
        className
      )}
    >
      {content}
    </Link>
  )
}

export interface ShowGigTimelineProps {
  /**
   * The show being read, as a stop on its own spine. It supplies the middle
   * marker AND the year every neighbour's date is compared against, which is
   * why it arrives whole rather than pre-formatted: two of those three strings
   * would otherwise be derived at the call site from the same object, and the
   * third could only ever be this stop's own year.
   */
  current: TimelineStop
  previous: ShowTimelineEntry | null
  next: ShowTimelineEntry | null
  /**
   * The act whose route this is, for the landmark's accessible name.
   *
   * Load-bearing on a co-headline bill: the heading above prints every curated
   * headliner, while these dates belong to exactly ONE of them, and without a
   * name the reader is invited to read one act's route as the whole show's.
   * Resolve it from `headliner_artist_id` against the bill, so it is the name
   * the page already printed rather than a second copy of it.
   *
   * Empty when the bill is empty, which falls back to the bare landmark label.
   */
  headlinerName?: string
}

/**
 * The headliner's adjacent dates as one banded line under the bill:
 * `< AUG 9 METRO, CHICAGO  |  AUG 12 SALT SHED  |  AUG 14 ROYAL OAK, MI >`.
 *
 * The module turns the page from a leaf into a corridor, and after the show it
 * is the tour's archive spine, so it renders in every lifecycle state.
 *
 * SELF-HIDING when there is no neighbour in either direction. The marker for
 * the current date is not content: the reader is already on it, and the date
 * is in the heading block a line above.
 *
 * The arrows and the marker are `aria-hidden` decoration. Direction is
 * announced by each stop's own screen-reader prefix instead, because "left
 * arrow" is not a direction in a document and the glyphs would otherwise be
 * the only thing distinguishing two dates.
 *
 * ONE ROW only at `lg`, where three place names fit across the reading column.
 * Narrower than that it stacks to previous-then-next and drops the marker for
 * the date the reader is already on, rather than clipping the row or reflowing
 * three ragged fragments that no longer read as a route.
 */
export function ShowGigTimeline({
  current,
  previous,
  next,
  headlinerName,
}: ShowGigTimelineProps) {
  if (!previous && !next) return null

  const currentYear = timelineYear(current)
  // Compared against its own year, so this stop never carries one. The
  // neighbours are what the year rule exists for.
  const currentLabel = [
    timelineDateLabel(current, currentYear),
    timelineCurrentPlaceLabel(current),
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <nav
      aria-label={
        headlinerName ? `Gig timeline for ${headlinerName}` : 'Gig timeline'
      }
      data-testid="show-gig-timeline"
      className="mt-4 border-y border-border/60 py-2"
    >
      <div className="flex flex-col gap-y-1 font-mono text-xs tracking-wider lg:flex-row lg:items-baseline lg:justify-between lg:gap-x-6">
        {/* The arrow is INSIDE the guard. Rendered unconditionally it becomes a
            glyph pointing at nothing whenever a direction has no date, which is
            the majority state for `next` on a past show, and below `lg` it gets
            a line of its own. */}
        <span className="flex items-baseline gap-x-2">
          {previous && (
            <>
              <span aria-hidden="true" className="text-muted-foreground/60">
                &larr;
              </span>
              <SpineStop
                entry={previous}
                subjectYear={currentYear}
                direction="Previous show"
              />
            </>
          )}
        </span>

        {/* The date the reader is on. Not a link, and marked with a caret
            rather than styled as one, so the row states its own position in
            the route.

            Dropped below the width that fits three stops on one line. Stacked,
            it is a third row restating a date the heading two lines above
            already carries, and the two stops the reader CANNOT see from here
            are what the module is for. */}
        <span className="hidden items-baseline gap-x-2 font-semibold text-foreground lg:flex">
          <span aria-hidden="true" className="text-primary">
            &#9656;
          </span>
          <span>{currentLabel}</span>
        </span>

        {/* `justify-end` only once the row is horizontal: stacked, a
            right-aligned line under a left-aligned one reads as two unrelated
            fragments rather than one route. */}
        <span className="flex items-baseline gap-x-2 lg:justify-end">
          {next && (
            <>
              <SpineStop
                entry={next}
                subjectYear={currentYear}
                direction="Next show"
              />
              <span aria-hidden="true" className="text-muted-foreground/60">
                &rarr;
              </span>
            </>
          )}
        </span>
      </div>
    </nav>
  )
}
