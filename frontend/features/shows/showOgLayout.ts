import { OG_SIZE } from '@/lib/og/brand'
import { fitFontSize, measureMono, measureSans } from '@/lib/og/textFit'

/**
 * Geometry of the show share card, from the blessed Figma mock
 * (`XakQQ0nYGqnt77PrHKO9IE`, node `1192:15`, with its true 300px render at
 * `1192:40`).
 *
 * Separated from the route that renders it so a unit test can import these
 * without pulling in `next/og` — the budgets the tests assert against are then
 * the ones the card actually uses. The sibling weekly card does the same, for
 * the same reason.
 *
 * The card is consumed at ~300px, a 4× downscale, so divide by 4 to check
 * anything here: date 8.5 · title 19.5 · support 8.5 · venue 9.5 · domain 8 ·
 * SOLD OUT 8. Nothing that carries meaning may sit under ~8px.
 */
export const PAD_X = 70
export const PAD_Y = 60
export const CONTENT_WIDTH = OG_SIZE.width - PAD_X * 2

export const DATE_SIZE = 34
export const SUPPORT_SIZE = 34
export const DOMAIN_SIZE = 32
export const SOLD_OUT_SIZE = 32
export const HEADLINE_GAP = 12

export const TITLE_SIZE_MAX = 78
export const TITLE_SIZE_MIN = 48
export const TITLE_LINE_HEIGHT = 84
/** What the layout budgets vertically before the footer would be pushed off. */
export const TITLE_MAX_LINES = 2

export const VENUE_SIZE = 38
/** 6.5px effective — below this the line stops being readable, so it clips. */
export const VENUE_SIZE_MIN = 26
/** Keeps the venue line clear of the wordmark on the opposite edge. */
export const FOOTER_GAP = 40

export const WORDMARK = 'psychichomily.com'

/**
 * The flyer plate, when a show has one.
 *
 * A gig poster is a designed object, so it is placed as a PLATE — letterboxed
 * whole at its own aspect ratio — rather than cropped to fill a slot. Cropping
 * a poster cuts the headline off it, which is the one thing on it that a share
 * card needs to survive.
 *
 * The box is portrait because posters are: at 380×510 a 2:3 flyer fills the
 * full height, which is the common case getting the best of the space. Wider
 * artwork letterboxes inside the same box, so the text column's width does not
 * depend on the image — one geometry, whatever a submitter uploads.
 *
 * At the ~300px the card is consumed at, the plate lands at 95×128: a
 * recognisable thumbnail rather than a decorative smudge.
 */
export const PLATE_BOX_WIDTH = 380
export const PLATE_BOX_HEIGHT = OG_SIZE.height - PAD_Y * 2
/** Keeps the plate off the text column at the share size, where 40px reads as 10. */
export const PLATE_GAP = 40

/**
 * What the text column gets once the plate has taken its band — and, because
 * the footer STACKS beside a plate, what the venue line gets too.
 *
 * The wordmark costs ~326px in mono at its display size. Keeping the footer on
 * one row here would leave the venue 274px of this 640px column — not enough
 * for "Sleeping Village · Chicago, IL" even at the minimum size, i.e. the
 * ORDINARY case would clip. The row becomes two rows instead: vertical space is
 * the one thing the column has spare.
 */
export const TEXT_WIDTH_WITH_PLATE = CONTENT_WIDTH - PLATE_BOX_WIDTH - PLATE_GAP

/**
 * Size the flyer to fit inside the plate box without cropping or distorting it.
 *
 * Returned as explicit pixel dimensions rather than left to `object-fit`, so
 * the geometry is a value the tests can assert on instead of a behaviour of
 * Satori's image resolver — and so a portrait flyer with an extreme ratio is
 * KNOWN to be bounded on both axes rather than hoped to be.
 */
export function fitPlate(
  naturalWidth: number,
  naturalHeight: number
): { width: number; height: number } {
  const scale = Math.min(
    PLATE_BOX_WIDTH / naturalWidth,
    PLATE_BOX_HEIGHT / naturalHeight
  )
  return {
    // At least 1px on each axis: a 3000×1 input would otherwise round to a
    // zero-height element, which Satori draws as nothing at all.
    width: Math.max(1, Math.round(naturalWidth * scale)),
    height: Math.max(1, Math.round(naturalHeight * scale)),
  }
}

/**
 * Width the venue line gets once the wordmark has taken its share of the row.
 *
 * The wordmark is measured in MONO because that is the face it renders in;
 * measuring it as sans under-reserves ~60px and lets the venue line wrap.
 */
export const VENUE_MAX_WIDTH =
  CONTENT_WIDTH - measureMono(WORDMARK, DOMAIN_SIZE) - FOOTER_GAP

/**
 * Wrapped text never fills its lines completely — a line breaks at the last
 * word that fits, so some width is always left behind.
 */
const WRAP_EFFICIENCY = 0.9

/** The longest run with no break opportunity, which cannot be wrapped away. */
function longestToken(text: string): string {
  return text.split(/[\s/–—-]+/).reduce((a, b) => (b.length > a.length ? b : a), '')
}

/**
 * Size for the title, which WRAPS rather than shrinking to one line.
 *
 * Constrained by two budgets, not one. The area budget alone cannot represent a
 * single unbreakable token wider than the content box: a long one-word title
 * satisfies "total width fits in two lines" while still being cut mid-glyph at
 * the edge, because no amount of wrapping makes a token narrower than itself.
 */
export function fitTitleSize(title: string, contentWidth: number = CONTENT_WIDTH): number {
  const byArea = fitFontSize(
    title,
    'satoshiBold',
    contentWidth * TITLE_MAX_LINES * WRAP_EFFICIENCY,
    TITLE_SIZE_MAX,
    TITLE_SIZE_MIN
  )
  const byToken = fitFontSize(
    longestToken(title),
    'satoshiBold',
    contentWidth,
    TITLE_SIZE_MAX,
    TITLE_SIZE_MIN
  )
  return Math.min(byArea, byToken)
}

/**
 * Size for the venue line, which must stay on ONE line.
 *
 * Takes the budget it actually has rather than the column width, because the
 * two card shapes reserve differently: the full-width card puts the wordmark
 * BESIDE the line, the plate card puts it below.
 */
export function fitVenueSize(venueLine: string, maxWidth: number = VENUE_MAX_WIDTH): number {
  return fitFontSize(venueLine, 'satoshiMedium', maxWidth, VENUE_SIZE, VENUE_SIZE_MIN)
}

/**
 * Whether the venue line still overruns its budget at the minimum size.
 *
 * `fitFontSize` clamps at its floor and returns a size that does NOT fit,
 * telling the caller nothing — so the caller has to ask separately in order to
 * clip rather than let the line run under the wordmark.
 */
export function venueOverflows(venueLine: string, maxWidth: number = VENUE_MAX_WIDTH): boolean {
  return measureSans(venueLine, 'satoshiMedium', fitVenueSize(venueLine, maxWidth)) > maxWidth
}

/**
 * The venue line: name, city and state, with empty parts dropped.
 *
 * Built by joining what exists rather than interpolating, so a venue with no
 * state does not render a trailing `Phoenix, `.
 */
export function buildVenueLine(
  name: string,
  city?: string | null,
  state?: string | null
): string {
  const place = [city, state].filter(Boolean).join(', ')
  return place ? `${name} · ${place}` : name
}
