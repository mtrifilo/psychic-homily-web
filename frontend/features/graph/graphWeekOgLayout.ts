/**
 * Geometry of the "this week in the graph" share card, from the approved mock
 * (PSY-1726 board, share-artifact section, Figma node `1311:2`).
 *
 * Separated from the card that renders it for the reason the rest of the family
 * is: a unit test can import these without pulling in `next/og`, so the budgets
 * the tests assert against are the ones the card actually uses. Both HIGH
 * findings on the first weekly card were invisible to CI precisely because its
 * fit budgets lived inside the route.
 *
 * Sizes are px on the 1200×630 canvas. The family's rule is to design at full
 * size and VERIFY AT 300px — a link renders about that wide in a group chat —
 * and to treat anything under ~8px effective as decoration that must not carry
 * meaning. Divide by 4 to check: headline 23 · counts 8.5-11 · eyebrow 8.5 ·
 * range 8.5.
 *
 * The motif's own geometry — the projection, the caps, the radii — is NOT here.
 * It is the same picture the share page draws, at a different size and with a
 * different paint model, so it lives in `graphMotif` where a surface that has
 * nothing to do with `next/og` can reach it. What stays here is how this CARD
 * composes text over it: the box it occupies, the fades that keep the text
 * legible, and the opacities tuned against this background.
 */

import { OG_SIZE } from '@/lib/og/brand'
import { fitMonoSize, measureMono, measureSans } from '@/lib/og/textFit'

export const PAD_X = 72
export const PAD_Y = 64

/** The box every element on the card has to fit inside. */
export const CONTENT_WIDTH = OG_SIZE.width - PAD_X * 2

/**
 * `PSYCHIC HOMILY · THE MAP OF THE SCENE`, top left, Space Mono.
 *
 * 34px, matching the weekly city card's mono floor: it is the only line on the
 * card carrying the brand, and 30px lands at 7.5px effective — under the
 * family's own floor. It gets the FULL content width rather than the headline
 * column's, which is what makes 34px fit; the motif behind its tail is drawn at
 * low opacity for the same reason.
 */
export const EYEBROW_SIZE = 34
export const EYEBROW_TRACKING = 2
export const EYEBROW_TEXT = 'PSYCHIC HOMILY · THE MAP OF THE SCENE'

/**
 * `This week in the graph`, Satoshi Bold.
 *
 * A FIXED size with no fit function, and that is a property of the copy rather
 * than an omission: this string is a constant, so it is measured once — by the
 * test next to this file — instead of being re-measured on every render. The
 * two variable-length strings on the card (the counts and the range) are both
 * mono and both bounded, and only the counts need a step-down.
 *
 * It wraps to two lines inside `TEXT_WIDTH` by design. 92px is 23px at the
 * 300px share size, which keeps it the dominant element there too.
 */
export const HEADLINE_SIZE = 92
export const HEADLINE_LINE_HEIGHT = 88
export const HEADLINE_TEXT = 'This week in the graph'

/**
 * `+12 ARTISTS · +34 CONNECTIONS`, Space Mono, primary orange.
 *
 * Steps down rather than clipping, because the count is what the card exists to
 * say. The floor is the family's 8.5px-effective mono floor; the widest
 * realistic line (`+9,999 ARTISTS · +99,999 CONNECTIONS`) fits inside
 * `COUNTS_MAX_WIDTH` at it.
 */
export const COUNTS_SIZE_MAX = 40
export const COUNTS_SIZE_MIN = 34
export const COUNTS_TRACKING = 1

/** `JUL 27 - AUG 2 2026`, Space Mono, muted. */
export const RANGE_SIZE = 34
export const RANGE_TRACKING = 2

/** Optical gaps down the left column. */
export const HEADLINE_GAP = 22
export const RANGE_GAP = 14

/**
 * Width of the left text column.
 *
 * Narrower than the content box on purpose: the headline wraps inside it, and
 * the wrap is what keeps the map motif on the right visible as a map rather
 * than as a strip behind a single long line.
 */
export const TEXT_WIDTH = 660

/**
 * The counts line gets more room than the headline column.
 *
 * It is a single unwrappable mono run — a wrap would split `+12 ARTISTS` across
 * two lines — so it is allowed to reach past `TEXT_WIDTH` into the motif's
 * fade, where the gradient is still near-opaque.
 *
 * The VALUE is not chosen, it is derived, and the test beside this file is what
 * derives it: 810 is the smallest budget that seats the widest count line this
 * card can produce (`+9,999 ARTISTS · +99,999 CONNECTIONS`) at the 34px mono
 * floor, while still letting an ordinary week run at the full 40px. It also
 * keeps the line's right edge inside `MOTIF_FADE_CLEAR_STOP`, so no part of it
 * is ever set over undimmed dots.
 */
export const COUNTS_MAX_WIDTH = 810

/**
 * Horizontal fade that puts the text on solid brand background.
 *
 * Expressed as gradient stops rather than a box, because Satori paints it as
 * one `linear-gradient` over the motif. Opaque until the headline column ends,
 * clear well past the longest line the counts can produce — the clear stop is
 * 80% rather than the 74% this shipped with in draft, because at 74% the tail
 * of `+9,999 ARTISTS · +99,999 CONNECTIONS` landed exactly where the gradient
 * ran out and sat on undimmed connector lines. The test beside this file pins
 * the relationship so the two cannot drift apart again.
 */
export const MOTIF_FADE_OPAQUE_STOP = 46
export const MOTIF_FADE_CLEAR_STOP = 80

/**
 * A second fade, top-down, purely for the eyebrow.
 *
 * The eyebrow is the one line given the FULL content width — that is what buys
 * it 34px and the 8.5px-effective floor — so it necessarily reaches past the
 * horizontal fade and over the motif. Verified at the 300px share size: without
 * this, `THE MAP OF THE SCENE` sat on dots and stopped being readable, while
 * widening the horizontal fade enough to cover it would have hidden most of the
 * map. A local band is the smaller change and keeps the composition.
 */
export const MOTIF_TOP_FADE_HEIGHT = 190

/**
 * How strongly the motif reads THROUGH the card's dark background.
 *
 * Opacity lives here rather than with the projection because it is a property
 * of this composition — the motif sits behind text on this surface and beside
 * nothing on the share page — while the radii and the caps, which are the same
 * picture at two sizes, live in `graphMotif`.
 */
export const MOTIF_DOT_OPACITY = 0.34
export const MOTIF_NEW_DOT_OPACITY = 0.95
/**
 * Deliberately faint. At 0.5 — the draft value — a busy week rendered as a
 * spiderweb rather than a map: the lines out-shouted the dots they connect,
 * which are the thing the card is about. Verified against a seeded 1,218-node
 * snapshot at both full size and 300px.
 */
export const MOTIF_CONNECTOR_OPACITY = 0.26

/**
 * Next requires a STATIC alt, so it cannot name the counts — they change every
 * night. The page supplies the real numbers through `openGraph.images[].alt`.
 */
export const GRAPH_WEEK_OG_ALT =
  'A map of the Psychic Homily music graph with this week’s new artists and connections highlighted'

/** What the counts line gets, once its own tracking is accounted for. */
export function fitCountsSize(counts: string): number {
  return fitMonoSize(counts, COUNTS_MAX_WIDTH, COUNTS_SIZE_MAX, COUNTS_SIZE_MIN, COUNTS_TRACKING)
}

/** Measured widths the test asserts the fixed-size copy against. */
export function eyebrowWidth(): number {
  return measureMono(EYEBROW_TEXT, EYEBROW_SIZE, EYEBROW_TRACKING)
}

/** The headline's longest unbreakable run — what has to fit on one line. */
export function headlineLongestWordWidth(): number {
  return Math.max(
    ...HEADLINE_TEXT.split(' ').map(word => measureSans(word, 'satoshiBold', HEADLINE_SIZE))
  )
}
