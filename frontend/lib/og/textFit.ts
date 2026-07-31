/**
 * Text measurement for OG cards.
 *
 * Satori lays text out itself but offers no way to ASK how wide a string will
 * be, and an OG card has no reflow to fall back on: a headline that overruns
 * 1200px is simply clipped in every feed it lands in. The blessed mock was only
 * ever drawn with "Chicago" (515px at 132px); "Colorado Springs" is 1075px
 * against a 1056px content box, and a long room list overruns its share of the
 * footer outright. So the card measures before it renders.
 *
 * Measurement is EXACT for ASCII, the accented Latin that folds onto it, and
 * the handful of punctuation glyphs the cards actually emit. Everything else is
 * deliberately OVER-estimated — see `UNKNOWN_ADVANCE`. Over-estimating is the
 * safe direction: it can cost a headline a size step it did not need, whereas
 * under-estimating clips text off a canvas that has no reflow.
 *
 * Note that "measurable here" and "renders without an outbound request" are two
 * different questions, and widening the font coverage moved them apart: the
 * fallback face in `brand.ts` draws Greek and Cyrillic correctly and offline,
 * but this module still charges them `UNKNOWN_ADVANCE`. Read the `brand.ts`
 * header for which scripts are covered; read `UNKNOWN_ADVANCE` for why covered
 * still does not mean measured.
 *
 * The tables are the real advance widths of the shipping `.ttf` files, per 1000
 * units of em, read straight out of each font's `hmtx` table. Regenerate them
 * together with the fonts (see `brand.ts`); `textFit.fonts.test.ts` fails if the
 * files and these numbers ever drift apart:
 *
 *     # ASCII advances (one line per face)
 *     python -c "from fontTools.ttLib import TTFont; \
 *       f=TTFont('lib/og/fonts/Satoshi-Bold.ttf'); c=f.getBestCmap(); \
 *       h=f['hmtx']; u=f['head'].unitsPerEm; \
 *       print(','.join(str(round(h[c[i]][0]*1000/u)) for i in range(0x20,0x7F)))"
 *     # the non-ASCII glyphs the cards emit, and the vertical metrics
 *     python -c "from fontTools.ttLib import TTFont; \
 *       f=TTFont('lib/og/fonts/Satoshi-Bold.ttf'); c=f.getBestCmap(); \
 *       h=f['hmtx']; u=f['head'].unitsPerEm; \
 *       print({hex(g): round(h[c[g]][0]*1000/u) for g in (0xB7, 0x2013, 0x2019)}, \
 *             'descent', abs(f['hhea'].descent)/u)"
 *
 * This module owns every number derived from those font files — advances and
 * vertical metrics both — so there is ONE place to re-emit.
 *
 * Kerning is deliberately ignored. Satori applies it, and it almost always
 * TIGHTENS text, so an unkerned measurement over-estimates slightly — again the
 * safe direction. Letter-spacing is NOT ignored; pass it explicitly, because at
 * the card's tracking it is a ~4% effect rather than a rounding error.
 */

export type SansFace = 'satoshiBold' | 'satoshiMedium' | 'satoshiRegular'

/** ASCII 0x20–0x7E advance widths, per 1000 em units. */
const TABLES: Record<SansFace, number[]> = {
  satoshiBold:
    '270,320,434,719,592,959,736,244,308,308,420,660,291,445,291,407,705,398,589,569,646,600,620,538,641,620,311,311,660,660,660,554,944,681,651,759,740,593,567,782,742,290,552,674,538,884,758,786,639,786,670,592,590,731,700,1035,681,641,588,306,407,306,517,545,276,548,619,550,619,557,339,614,589,248,248,538,251,885,589,596,619,619,393,463,340,583,536,797,513,529,473,349,336,349,660'
      .split(',')
      .map(Number),
  satoshiMedium:
    '273,294,372,696,577,932,707,208,279,279,411,660,276,431,276,376,693,364,575,568,636,591,611,520,625,611,296,296,660,660,660,533,924,662,639,743,722,583,555,765,722,267,529,643,522,860,734,777,625,777,654,577,562,711,669,1005,645,606,583,279,376,279,500,530,245,534,600,534,600,542,317,598,575,230,230,508,234,862,575,581,600,600,368,448,318,567,509,768,482,498,457,318,323,318,660'
      .split(',')
      .map(Number),
  satoshiRegular:
    '277,271,317,675,563,909,682,176,254,254,403,660,262,418,262,349,683,334,562,568,627,583,603,504,612,603,282,282,660,660,660,515,907,645,629,729,706,575,543,750,705,246,510,616,509,839,713,769,614,769,640,563,537,693,641,979,612,575,580,255,349,255,485,518,218,522,584,520,584,528,297,584,562,213,213,482,219,842,562,568,584,584,345,434,298,553,486,742,455,471,443,292,311,292,660'
      .split(',')
      .map(Number),
}

/**
 * Non-ASCII advances for the glyphs the cards genuinely emit.
 *
 * `·` separates rooms in the footer and `–` separates the two dates in the week
 * range, so both appear on essentially every card. Left to the unknown-glyph
 * fallback, `·` measured 584 against a real 275 — a systematic over-estimate of
 * ~10px per separator that could drop a room name that would have fitted.
 */
const EXTRA_ADVANCES: Record<SansFace, Record<number, number>> = {
  satoshiBold: { 0xb7: 333, 0x2013: 977, 0x2019: 281 },
  satoshiMedium: { 0xb7: 302, 0x2013: 951, 0x2019: 262 },
  satoshiRegular: { 0xb7: 275, 0x2013: 927, 0x2019: 245 },
}

/** Space Mono is monospaced — every glyph in the subset advances 612/1000. */
const MONO_ADVANCE = 612

/**
 * Descender depth as a fraction of font size, from each face's `hhea` table
 * (Satoshi -240/1000, Space Mono -361/1000). Exported so the drift test can
 * check them against the font files.
 */
export const DESCENT_RATIO = { sans: 0.24, mono: 0.361 } as const

const FIRST_CODE = 0x20
const LAST_CODE = 0x7e

/**
 * Width charged to a glyph this module has no table entry for.
 *
 * Set to the widest advance in the shipped subsets (1211) so unknown text is
 * always OVER-measured. Under-measuring runs text off a canvas that has no
 * clipping; over-measuring merely costs it a size step.
 *
 * The tables above cover the BRAND faces (Satoshi, Space Mono), which are Latin
 * only. Greek, Cyrillic and the rest of the widened coverage are drawn from the
 * separate fallback face registered in `brand.ts`, and they stay on this
 * conservative path ON PURPOSE — carrying real advance tables for ~1,000 more
 * glyphs across three weights would be a large, hand-maintained table to buy
 * back one size step on a card that renders correctly either way.
 *
 * So there are now two distinct reasons a glyph lands here, and they have
 * different consequences:
 *
 *   - Covered by the fallback (Greek, Cyrillic, Vietnamese, symbols): renders
 *     correctly, no outbound request, merely measured generously.
 *   - Covered by nothing (CJK, emoji, and the scripts listed in `brand.ts`):
 *     also triggers a Google Fonts / jsDelivr fetch mid-render, and the face
 *     that comes back is one this module cannot measure at all.
 *
 * Widening the fallback shrinks the second class but not the first, so this
 * constant stays whatever the widest shipped advance is.
 */
const UNKNOWN_ADVANCE = 1211

/** The separator between named items in a fitted list. */
const LIST_SEPARATOR = ' · '

/**
 * Fold a string down to the ASCII the tables cover.
 *
 * Accents are dropped rather than looked up because every accented glyph in
 * these faces has exactly its base letter's advance (verified across all three
 * Satoshi weights), so `é` measures as `e` with zero error — which keeps the
 * tables at 95 entries instead of several hundred.
 */
function toMeasurable(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '')
}

/**
 * Width of `text` in px, for a Satoshi face at `fontSize`.
 *
 * `letterSpacing` must be passed whenever the rendered element sets it — the
 * card tracks its headline at roughly -2.3% of the font size, which is far too
 * large to absorb as measurement slack.
 */
export function measureSans(
  text: string,
  face: SansFace,
  fontSize: number,
  letterSpacing = 0
): number {
  const table = TABLES[face]
  const extra = EXTRA_ADVANCES[face]
  let units = 0
  let chars = 0
  for (const char of toMeasurable(text)) {
    const code = char.codePointAt(0)!
    units +=
      code >= FIRST_CODE && code <= LAST_CODE
        ? table[code - FIRST_CODE]
        : (extra[code] ?? UNKNOWN_ADVANCE)
    chars += 1
  }
  return (units * fontSize) / 1000 + chars * letterSpacing
}

/**
 * Width of `text` in px for Space Mono, including per-character tracking.
 *
 * Monospaced only WITHIN the subset. A glyph outside it is charged the same
 * conservative over-estimate as the sans path, because Satori will resolve it
 * from some other face at a width this module cannot know — and here the result
 * feeds the headline's budget, so under-charging would hand the city more room
 * than it actually has.
 */
export function measureMono(text: string, fontSize: number, letterSpacing = 0): number {
  let units = 0
  let chars = 0
  for (const char of toMeasurable(text)) {
    const code = char.codePointAt(0)!
    units += code >= FIRST_CODE && code <= LAST_CODE ? MONO_ADVANCE : UNKNOWN_ADVANCE
    chars += 1
  }
  return (units * fontSize) / 1000 + chars * letterSpacing
}

/**
 * How far to raise a mono run so its baseline meets a sans run's.
 *
 * Satori has an `alignItems: 'baseline'`, but it aligns the BOTTOM of the line
 * boxes rather than the baselines — measured at 16px of drift for a 132px sans
 * beside a 40px mono, which these descent ratios predict as 17px. So the card
 * aligns the boxes explicitly with `flex-end` and lifts the smaller run by the
 * difference in descender depth, which is exactly what that drift is.
 *
 * `DESCENT_RATIO.sans` is Satoshi's. A headline drawn from the fallback face
 * instead — a Cyrillic or Greek city — sits on a deeper descender (0.293 vs
 * 0.24), so the state pill beside it lands roughly `citySize * 0.05` too high,
 * about 7px at the largest headline size. Left uncorrected on purpose: fixing it
 * means threading the resolved face through the fit path, and Satori resolves it
 * per grapheme, so a mixed-script name has no single answer to thread.
 */
export function monoBaselineLift(sansSize: number, monoSize: number): number {
  return sansSize * DESCENT_RATIO.sans - monoSize * DESCENT_RATIO.mono
}

/**
 * Largest size in `[minSize, maxSize]` at which `text` fits `maxWidth`.
 *
 * Returns `maxSize` whenever the text already fits, so the common case renders
 * at the blessed display size and only genuinely long names step down.
 */
export function fitFontSize(
  text: string,
  face: SansFace,
  maxWidth: number,
  maxSize: number,
  minSize: number,
  trackingEm = 0
): number {
  const widthAt = (size: number) => measureSans(text, face, size, trackingEm * size)
  const widthAtMax = widthAt(maxSize)
  if (widthAtMax <= maxWidth) return maxSize
  // Width is linear in size (advances and tracking both scale with it), so one
  // proportional step lands on the fit rather than needing a search.
  return Math.max(minSize, Math.floor((maxWidth / widthAtMax) * maxSize))
}

/**
 * Fit as many list items as `maxWidth` allows, with an honest `+N` remainder.
 *
 * Truncation must never be silent — on the weekly card this names the venues
 * the coverage actually draws from, and a card implying full city coverage
 * would be false (PSY-1576 decision 8) — so anything dropped is always counted.
 *
 * `overflowLabel` supplies the degenerate case where not even one item fits.
 * It is a parameter rather than baked in because the copy is the caller's
 * product language, not a property of text measurement.
 */
export function fitItemList(
  items: string[],
  face: SansFace,
  fontSize: number,
  maxWidth: number,
  overflowLabel: (count: number) => string
): string | null {
  if (items.length === 0) return null

  let shown = ''
  let count = 0

  for (const item of items) {
    const candidate = count === 0 ? item : `${shown}${LIST_SEPARATOR}${item}`
    const remainingIfAdded = items.length - (count + 1)
    const withRemainder =
      remainingIfAdded > 0 ? `${candidate} +${remainingIfAdded}` : candidate
    if (measureSans(withRemainder, face, fontSize) > maxWidth) break
    shown = candidate
    count += 1
  }

  // Even a single name overruns the space — fall back to the caller's summary,
  // which is always short enough and still tells the reader this is a slice.
  if (count === 0) return overflowLabel(items.length)

  const remaining = items.length - count
  return remaining > 0 ? `${shown} +${remaining}` : shown
}
