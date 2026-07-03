/**
 * THE show-display-title convention: `title → bill names → "Untitled Show"`
 * (PSY-1328 — consolidates three divergent copies + ~8 inline `||` chains).
 *
 * Hardening carried over from PSY-1325's review:
 * - Whitespace-only titles are NOT titles — a truthy `"  "` renders an
 *   invisible (and, in link rows, unclickable) label.
 * - Blank bill entries get the same trim-truthy gate: a `[""]` payload must
 *   fall through to the empty state, not join into nothing.
 * - Null-tolerant inputs: sibling API fields are omitempty-optional on the
 *   wire; a future omitempty on title/names must not become a TypeError.
 */

export const UNTITLED_SHOW = 'Untitled Show'

export interface ShowDisplayTitleOptions {
  /**
   * Cap on bill names in the fallback; overflow renders as "+N more"
   * (e.g. the scene preview's narrow rows use 3 — a festival-sized bill
   * would wrap a one-line row into a paragraph).
   */
  cap?: number
  /**
   * Compose "@ venue" into the FALLBACK (never into a real title) for
   * surfaces that don't render the venue anywhere else — e.g. the charts
   * list's compact rows. Surfaces that show the venue separately must NOT
   * pass this, or the venue doubles.
   */
  venueName?: string | null
}

export function showDisplayTitle(
  title: string | null | undefined,
  artistNames: readonly (string | null | undefined)[] | null | undefined,
  opts: ShowDisplayTitleOptions = {},
): string {
  const trimmedTitle = (title ?? '').trim()
  if (trimmedTitle) return trimmedTitle

  const names = (artistNames ?? [])
    .map((n) => (n ?? '').trim())
    .filter(Boolean)

  let bill = ''
  if (names.length > 0) {
    const cap = opts.cap !== undefined && opts.cap > 0 ? opts.cap : names.length
    const shown = names.slice(0, cap).join(', ')
    const more = names.length - cap
    bill = more > 0 ? `${shown} +${more} more` : shown
  }

  const venue = (opts.venueName ?? '').trim()
  if (bill && venue) return `${bill} @ ${venue}`
  if (bill) return bill
  if (venue) return `Show @ ${venue}`
  return UNTITLED_SHOW
}
