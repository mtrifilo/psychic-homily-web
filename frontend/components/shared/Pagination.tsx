'use client'

import { useCallback, useRef, useState } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import {
  formatCount,
  isPlainNavigationClick,
  navCurrentClass,
  navLinkClass,
  toPageNumber,
} from './paginationChrome'

/** A slot in the rendered page strip: a page number, or a collapsed gap. */
export type PaginationWindowItem = number | 'ellipsis'

/**
 * Page counts at or below this render every page; above it the strip collapses
 * to first / last / current±1 with ellipses (GOV.UK pagination pattern).
 */
const FULL_STRIP_MAX_PAGES = 7

/**
 * GOV.UK-style page windowing. Returns every page up to
 * {@link FULL_STRIP_MAX_PAGES}, and beyond that the first page, the last page,
 * and the current page with its immediate neighbors, with `'ellipsis'` marking
 * each collapsed gap.
 *
 * Inputs are clamped rather than rejected: a caller whose `currentPage` has
 * drifted past the end (stale URL, shrinking result set) still gets a sane
 * strip instead of a crash or an empty nav.
 */
export function paginationWindow(
  currentPage: number,
  totalPages: number
): PaginationWindowItem[] {
  const total = toPageNumber(totalPages, 1)
  const current = Math.min(toPageNumber(currentPage, 1), total)

  if (total <= FULL_STRIP_MAX_PAGES) {
    return Array.from({ length: total }, (_, index) => index + 1)
  }

  const pages = new Set([1, total, current])
  if (current > 1) pages.add(current - 1)
  if (current < total) pages.add(current + 1)
  const sorted = [...pages].sort((a, b) => a - b)

  const items: PaginationWindowItem[] = []
  sorted.forEach((page, index) => {
    if (index > 0 && page - sorted[index - 1] > 1) items.push('ellipsis')
    items.push(page)
  })
  return items
}

/** Row-count summary rendered in the pager caption. */
export interface PaginationCaptionRange {
  /** 1-based index of the first row on the current page. */
  start: number
  /** 1-based index of the last row on the current page. */
  end: number
  /** Total rows across every page. */
  total: number
}

export interface PaginationProps {
  /** 1-based current page. Clamped into `[1, totalPages]`. */
  currentPage: number
  /** Total page count. The whole control renders `null` at 1 or fewer. */
  totalPages: number
  /**
   * Builds the href for a 1-based page number. The consumer owns the entire URL
   * shape — query params, page-1-writes-null, and anchor suffixes such as
   * `#past-shows` all belong here, not in this component.
   */
  pageHref: (page: number) => string
  /**
   * Accessible name for the `<nav>` landmark. Must be unique per page when a
   * surface renders more than one pager (e.g. "Past shows pagination").
   */
  ariaLabel: string
  /**
   * Contextual label per page, keyed by 1-based page number
   * (`{ 1: 'Sep–Dec', 2: 'Jun–Sep' }`). Derived by the consumer from the rows
   * each page covers — this component never computes them. Omit entirely for
   * numeral-only pagers (charts, releases, any non-chronological list).
   */
  rangeLabels?: Record<number, string>
  /** Row counts for the caption. Omit to caption with just "Page X of Y". */
  captionRange?: PaginationCaptionRange
  /**
   * Fired with the target page when a page link is activated, alongside (not
   * instead of) navigation. Use it to move focus to the list heading — see
   * {@link usePaginationFocusTarget}.
   */
  onNavigate?: (page: number) => void
  /** Label for the toward-page-1 control. Defaults to "Newer". */
  previousLabel?: string
  /** Label for the toward-last-page control. Defaults to "Older". */
  nextLabel?: string
  /**
   * Whether this instance owns the page-change announcement. Defaults to `true`,
   * so a surface with one pager gets it without asking.
   *
   * Set `false` on every pager but one when a surface renders the SAME list
   * twice (above and below it, the venue archive's shape). Both instances would
   * otherwise ship their own live region and update it in the same commit, and a
   * screen reader speaks each one — "Page 2 of 4" twice per click. Which
   * instance keeps it is the consumer's call; the one nearer the focus target
   * is the natural owner.
   *
   * Only the region is dropped, never the caption or the mobile position text:
   * the position stays readable in the pager itself, this just stops a second
   * copy from being SPOKEN.
   */
  announce?: boolean
  /** Extra classes on the `<nav>`. */
  className?: string
}

/**
 * Disabled boundary controls keep their space rather than unmounting, so the
 * row never reflows as the reader pages through (EpisodeNav precedent). They
 * are `aria-hidden` because a dimmed, unclickable span is dead weight in the
 * accessibility tree — the same treatment ArchiveMasthead gives unavailable
 * adjacent periods.
 */
const disabledBoundaryClass =
  'pointer-events-none select-none text-muted-foreground/50'

/** Minimum 44x44px hit area for the thumb-driven mobile row. */
const mobileTouchTargetClass =
  'inline-flex min-h-11 min-w-11 items-center justify-center'

function BoundaryControl({
  label,
  chevron,
  side,
  targetPage,
  disabled,
  pageHref,
  onNavigate,
  className,
}: {
  label: string
  chevron: string
  side: 'previous' | 'next'
  targetPage: number
  disabled: boolean
  pageHref: (page: number) => string
  onNavigate?: (page: number) => void
  className?: string
}) {
  // Margin rather than a whitespace text node: the mobile row makes this an
  // `inline-flex` touch target, and flex layout collapses the whitespace, which
  // renders "‹Newer" instead of "‹ Newer".
  const content =
    side === 'previous' ? (
      <>
        <span aria-hidden="true" className="mr-1">
          {chevron}
        </span>
        {label}
      </>
    ) : (
      <>
        {label}
        <span aria-hidden="true" className="ml-1">
          {chevron}
        </span>
      </>
    )

  if (disabled) {
    return (
      <span
        aria-hidden="true"
        data-disabled="true"
        data-testid={`pagination-${side}-disabled`}
        className={cn(disabledBoundaryClass, className)}
      >
        {content}
      </span>
    )
  }

  return (
    <Link
      href={pageHref(targetPage)}
      onClick={(event) => {
        if (isPlainNavigationClick(event)) onNavigate?.(targetPage)
      }}
      className={cn(navLinkClass, className)}
    >
      {content}
    </Link>
  )
}

/**
 * Shared pager for chronological and numeral-only lists. Every page is a real
 * `<a href>` so crawlers reach the whole archive; client-side transitions are
 * layered on top by the consumer via `onNavigate`, never by this component.
 *
 * Presentation-only by design: no nuqs, no router, no data fetching. It is
 * handed a page count, an href builder, and (optionally) the date-range label
 * each page covers.
 *
 * Renders `null` at one page or fewer, so consumers can drop it in
 * unconditionally above and below a list.
 *
 * The page-change announcement is a LIVE REGION, so it only speaks for a
 * consumer that keeps this component mounted across the change. A surface that
 * swaps its results region for a loading state on every page click unmounts the
 * region along with it, and a live region that appears already populated
 * announces nothing — the reader gets no page-change feedback at all (PSY-1768).
 * Hold the previous page on screen (`placeholderData: keepPreviousData`, dimmed)
 * rather than tearing the list down. Consumers rendering this twice must also
 * pass `announce={false}` on all but one instance.
 *
 * Usage:
 *   <Pagination
 *     currentPage={page}
 *     totalPages={totalPages}
 *     ariaLabel="Past shows pagination"
 *     pageHref={(p) => (p === 1 ? '?#past-shows' : `?page=${p}#past-shows`)}
 *     rangeLabels={{ 1: 'Sep–Dec', 2: 'Jun–Sep' }}
 *     captionRange={{ start: 51, end: 100, total: 161 }}
 *     onNavigate={() => focusTarget()}
 *   />
 */
export function Pagination({
  currentPage,
  totalPages,
  pageHref,
  ariaLabel,
  rangeLabels,
  captionRange,
  onNavigate,
  previousLabel = 'Newer',
  nextLabel = 'Older',
  announce = true,
  className,
}: PaginationProps) {
  // A non-finite page count falls back to 1, which renders nothing at all. A
  // list whose count is still resolving should show no pager, never a crash or
  // "Page NaN of NaN". See `toPageNumber`.
  const total = toPageNumber(totalPages, 1)
  const current = Math.min(toPageNumber(currentPage, 1), total)

  const positionText = `Page ${current} of ${total}`
  const currentRangeLabel = rangeLabels?.[current]
  const captionText = captionRange
    ? `Showing ${formatCount(captionRange.start)}–${formatCount(captionRange.end)} of ${formatCount(captionRange.total)} · ${positionText}`
    : positionText

  // The live region ships here so every consumer gets the announcement for
  // free, but it stays silent on mount: a region populated in the initial
  // render would have some screen readers announce "Page 1 of 4" on every
  // cold page load. Only a page change the reader caused is worth speaking.
  //
  // Adjusted during render rather than in an effect (React's documented
  // "adjusting state when a prop changes" pattern): an effect would announce a
  // beat late, and the compiler rejects synchronous setState inside one.
  const [announcement, setAnnouncement] = useState('')
  const [announcedPage, setAnnouncedPage] = useState(current)
  if (announcedPage !== current) {
    setAnnouncedPage(current)
    setAnnouncement(
      currentRangeLabel ? `${positionText}, ${currentRangeLabel}` : positionText
    )
  }

  if (total <= 1) return null

  const items = paginationWindow(current, total)

  return (
    <nav
      aria-label={ariaLabel}
      className={cn('font-mono text-xs', className)}
      data-testid="pagination"
    >
      {/* Desktop: boundary controls flanking the numbered strip, caption right-aligned. */}
      <div
        data-testid="pagination-desktop"
        className="hidden flex-wrap items-baseline gap-x-4 gap-y-2 sm:flex"
      >
        <BoundaryControl
          label={previousLabel}
          chevron="‹"
          side="previous"
          targetPage={current - 1}
          disabled={current <= 1}
          pageHref={pageHref}
          onNavigate={onNavigate}
        />
        <ul className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
          {items.map((item, index) =>
            item === 'ellipsis' ? (
              // Decorative: the gap is conveyed by the page numbers on either
              // side, and a focusable "…" is a dead stop for keyboard users.
              <li
                key={`ellipsis-${index}`}
                aria-hidden="true"
                className="text-muted-foreground"
              >
                …
              </li>
            ) : (
              <li key={item}>
                <Link
                  href={pageHref(item)}
                  // The current page stays a real link: it is the canonical URL
                  // for this slice of the list, and crawlers plus "copy link"
                  // both depend on it being there.
                  aria-current={item === current ? 'page' : undefined}
                  onClick={(event) => {
                    if (isPlainNavigationClick(event)) onNavigate?.(item)
                  }}
                  className={cn(
                    navLinkClass,
                    item === current && navCurrentClass
                  )}
                >
                  {/*
                    The whole accessible name lives in one node. Split across
                    sibling elements, name computation concatenates each
                    element's trimmed text with no separator, and engines
                    disagree about where to reinsert spaces — so "Page 2" plus
                    "Jun–Sep" can come out as "Page 2Jun–Sep". The visible
                    parts are aria-hidden; visible text stays a subset of the
                    name, satisfying label-in-name for voice control.
                  */}
                  <span className="sr-only">
                    {rangeLabels?.[item]
                      ? `Page ${item}, ${rangeLabels[item]}`
                      : `Page ${item}`}
                  </span>
                  <span aria-hidden="true">{item}</span>
                  {rangeLabels?.[item] ? (
                    <span aria-hidden="true"> · {rangeLabels[item]}</span>
                  ) : null}
                </Link>
              </li>
            )
          )}
        </ul>
        <p className="ml-auto text-[11px] text-muted-foreground">
          {captionText}
        </p>
        <BoundaryControl
          label={nextLabel}
          chevron="›"
          side="next"
          targetPage={current + 1}
          disabled={current >= total}
          pageHref={pageHref}
          onNavigate={onNavigate}
        />
      </div>

      {/* Mobile: the numbered strip collapses to position + current range. */}
      <div
        data-testid="pagination-mobile"
        className="flex items-center justify-between gap-2 sm:hidden"
      >
        <BoundaryControl
          label={previousLabel}
          chevron="‹"
          side="previous"
          targetPage={current - 1}
          disabled={current <= 1}
          pageHref={pageHref}
          onNavigate={onNavigate}
          className={mobileTouchTargetClass}
        />
        <p className="text-center text-muted-foreground">
          {positionText}
          {currentRangeLabel ? (
            <>
              <span aria-hidden="true"> · </span>
              <span className="text-foreground">{currentRangeLabel}</span>
            </>
          ) : null}
        </p>
        <BoundaryControl
          label={nextLabel}
          chevron="›"
          side="next"
          targetPage={current + 1}
          disabled={current >= total}
          pageHref={pageHref}
          onNavigate={onNavigate}
          className={mobileTouchTargetClass}
        />
      </div>

      {announce ? (
        <div role="status" aria-live="polite" className="sr-only">
          {announcement}
        </div>
      ) : null}
    </nav>
  )
}

/**
 * Focus target for client-side page changes. A page swap that only mutates the
 * list leaves focus stranded on a control that may have moved or unmounted, and
 * leaves screen-reader users with no signal that the content changed. Spread
 * `targetProps` onto the list heading and call `focusTarget` from the pager's
 * `onNavigate`; the pager's own live region handles the announcement.
 *
 * Usage:
 *   const { targetProps, focusTarget } = usePaginationFocusTarget<HTMLHeadingElement>()
 *   <h2 {...targetProps}>Past shows</h2>
 *   <Pagination onNavigate={() => focusTarget()} ... />
 */
export function usePaginationFocusTarget<
  T extends HTMLElement = HTMLHeadingElement,
>() {
  const ref = useRef<T>(null)
  const focusTarget = useCallback(() => {
    ref.current?.focus()
  }, [])
  // `tabIndex: -1` makes the heading programmatically focusable without adding
  // it to the tab order.
  return { targetProps: { ref, tabIndex: -1 } as const, focusTarget }
}
