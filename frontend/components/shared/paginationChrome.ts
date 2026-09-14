import type { MouseEvent } from 'react'

/**
 * Shared chrome for the archive-navigation family (`Pagination`, `YearStrip`).
 * Both render the same dense `font-mono text-xs` link row and are meant to look
 * like one control stacked in two halves, so their link styling lives in one
 * place rather than being copied into each file and drifting apart the first
 * time one of them is restyled.
 */

export const navLinkClass =
  'rounded-sm text-foreground transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

/**
 * The filter-strip row: horizontally scrollable below `sm`, wrapping above it.
 * Shared by `YearStrip` and `MonthStrip`, which navigate different axes of the
 * same list and must not drift into two shapes.
 */
export const navStripClass =
  'flex items-baseline gap-x-2 overflow-x-auto font-mono text-xs sm:flex-wrap sm:overflow-x-visible'

/** The list inside a filter strip. Pairs with {@link navStripClass}. */
export const navStripListClass =
  'flex items-baseline gap-x-2 whitespace-nowrap sm:flex-wrap sm:gap-y-1'

/** The separator between strip entries. Decorative, so it is hidden from AT. */
export const navStripSeparatorClass = 'mr-2 text-muted-foreground'

/**
 * Current item. Deliberately carries weight and an underline on top of the
 * color: color alone is not a sufficient distinction (WCAG 1.4.1).
 */
export const navCurrentClass =
  'font-bold text-primary underline underline-offset-4'

/**
 * Locale is pinned rather than left to the runtime. These counts render on the
 * server and again on the client, and a viewer whose browser groups digits
 * differently ("1.234" vs "1,234") would otherwise hydrate into a mismatch.
 * Same reason the date helpers in `lib/utils` pin `en-US`.
 */
export const formatCount = (value: number) => value.toLocaleString('en-US')

/**
 * Normalizes a page-ish number to a positive integer, falling back when the
 * input is not a usable number at all.
 *
 * Consumers routinely derive page counts as `Math.ceil(total / PAGE_SIZE)`, and
 * `total` is `undefined` on the first render of a list that is still loading —
 * which yields `NaN`. `NaN` must not reach the components: it defeats every
 * clamp (`Math.max(1, NaN)` is `NaN`), renders "Page NaN of NaN", and because
 * `NaN !== NaN` it makes the render-phase state adjustments in these components
 * re-render forever until React throws "Too many re-renders".
 */
export function toPageNumber(value: number, fallback: number): number {
  return Number.isFinite(value) ? Math.max(1, Math.floor(value)) : fallback
}

/**
 * The page a pager will actually SHOW for a requested page number.
 *
 * A `?page=` can name a page past the end — a stale bookmark, a hand-typed
 * number, a result set that shrank — and every pager clamps it into range
 * rather than rendering "Page 99 of 3". Anything deriving per-page state
 * alongside a pager has to clamp identically, or it keys that state on a page
 * number nothing on screen refers to.
 */
export function clampToPageCount(page: number, totalPages: number): number {
  return Math.min(toPageNumber(page, 1), toPageNumber(totalPages, 1))
}

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
 *
 * Lives here rather than beside the component because it is pure arithmetic and
 * `showArchive.ts` — imported by server routes — derives page labels from it.
 * `Pagination.tsx` re-exports it for importers that reach for both.
 */
export function paginationWindow(
  currentPage: number,
  totalPages: number
): PaginationWindowItem[] {
  const total = toPageNumber(totalPages, 1)
  const current = clampToPageCount(currentPage, total)

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

/**
 * A cmd/ctrl/shift/alt-click (or a click a handler already cancelled) opens the
 * target in a new tab or window, or does nothing at all: THIS tab never
 * navigates. Consumers use the navigate callbacks to move focus and announce a
 * page change, so firing on those clicks would yank focus for a navigation that
 * did not happen here.
 */
export function isPlainNavigationClick(
  event: MouseEvent<HTMLAnchorElement>
): boolean {
  return (
    !event.defaultPrevented &&
    event.button === 0 &&
    !event.metaKey &&
    !event.ctrlKey &&
    !event.shiftKey &&
    !event.altKey
  )
}
