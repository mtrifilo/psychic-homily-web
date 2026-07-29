import { OG_SIZE } from '@/lib/og/brand'
import { measureMono } from '@/lib/og/textFit'

/**
 * Geometry of the weekly-city share card, from the blessed Figma mock.
 *
 * Separated from the card that renders it for two reasons: the padding feeds
 * BOTH the CSS and the fit budgets, so writing it twice would let a layout
 * tweak silently invalidate every measurement; and a unit test can import these
 * without pulling in `next/og`, so the budgets the tests assert against are the
 * ones the card actually uses.
 *
 * Sizes are px on the 1200×630 canvas. The family's rule is to design at full
 * size but verify at 300px — a link renders about that wide in a group chat —
 * and to treat anything under ~8px effective as decoration that must not carry
 * meaning. Divide by 4 to check: city 33 · count 12 · state 10 · footer 8.5 ·
 * range 7.5.
 */
export const PAD_X = 72
export const PAD_Y = 64

/** The box every element on the card has to fit inside. */
export const CONTENT_WIDTH = OG_SIZE.width - PAD_X * 2

/** Week range, top left, Space Mono. */
export const RANGE_SIZE = 30
export const RANGE_TRACKING = 2

/**
 * The city, at display scale.
 *
 * Long names step down from the max rather than clipping. The floor keeps the
 * headline dominant even for the longest metro name — 72px is 18px at the
 * 300px share size, still far above the legibility floor.
 */
export const CITY_SIZE_MAX = 132
export const CITY_SIZE_MIN = 72
/** Design tracking is -3px at 132px; hold the ratio as the city steps down. */
export const CITY_TRACKING_EM = -3 / CITY_SIZE_MAX

/** The state, in mono, set on the city's baseline. */
export const STATE_SIZE = 40
export const STATE_TRACKING = 3
export const CITY_STATE_GAP = 20

/** Optical gap between the city and the show count beneath it. */
export const HEADLINE_GAP = 2
export const COUNT_SIZE = 48

export const FOOTER_SIZE = 34
/** Keeps the room list clear of the wordmark on the opposite edge. */
export const FOOTER_GAP = 48

export const WORDMARK = 'psychichomily.com'

/**
 * Next requires a STATIC alt, so it cannot say "this week" — the same card
 * serves archived weeks, where that would be false.
 *
 * Lives here rather than beside the renderer so the page can reference it
 * without dragging `next/og` into its bundle.
 */
export const SCENE_WEEK_OG_ALT =
  'Weekly show listing: city, dates, show count, and the rooms we track'

/** What the room list gets once the wordmark has taken its share of the footer. */
export const ROOMS_MAX_WIDTH =
  CONTENT_WIDTH - measureMono(WORDMARK, FOOTER_SIZE) - FOOTER_GAP

/**
 * What the city headline gets once the state is sitting beside it.
 *
 * `state` may be empty — not every scene has one — in which case the headline
 * gets the whole content box.
 */
export function cityMaxWidth(state: string): number {
  if (!state) return CONTENT_WIDTH
  return CONTENT_WIDTH - measureMono(state, STATE_SIZE, STATE_TRACKING) - CITY_STATE_GAP
}
