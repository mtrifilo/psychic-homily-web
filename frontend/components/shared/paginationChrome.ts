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
