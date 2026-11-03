/**
 * Text measurement for OG cards.
 *
 * Satori lays text out itself but offers no way to ASK how wide a string will
 * be, and an OG card has no reflow to fall back on: a headline that overruns
 * 1200px is simply clipped in every feed it lands in. "Colorado Springs" at the
 * display size is wider than the 1056px content box, and a long room list
 * overruns its share of the footer outright. So the card measures before it
 * renders.
 *
 * Measurement is EXACT for ASCII, the accented Latin that folds onto it, and
 * the handful of non-ASCII glyphs listed in `EXTRA_ADVANCES`. Everything else
 * is deliberately OVER-estimated; see `UNKNOWN_ADVANCE`. Over-estimating is the
 * safe direction: it can cost a headline a size step it did not need, whereas
 * under-estimating clips text off a canvas that has no reflow.
 *
 * Note that "measurable here" and "renders without an outbound request" are two
 * different questions: the fallback face in `brand.ts` draws Greek and Cyrillic
 * correctly and offline, but this module still charges them `UNKNOWN_ADVANCE`.
 * Read the `brand.ts` header for which scripts are covered; read
 * `UNKNOWN_ADVANCE` for why covered still does not mean measured.
 *
 * The tables are the real advance widths of the shipping `.ttf` files, per 1000
 * units of em, read straight out of each font's `hmtx` table. Every shipped
 * face is on a 1000-unit em square, so the numbers are the font's own integers
 * rather than rounded ratios. Regenerate them together with the fonts (see
 * `brand.ts`); `textFit.fonts.test.ts` fails if the files and these numbers
 * ever drift apart:
 *
 *     # ASCII advances (one line per face)
 *     python -c "from fontTools.ttLib import TTFont; \
 *       f=TTFont('lib/og/fonts/SchibstedGrotesk-Bold.ttf'); c=f.getBestCmap(); \
 *       h=f['hmtx']; u=f['head'].unitsPerEm; \
 *       print(','.join(str(round(h[c[i]][0]*1000/u)) for i in range(0x20,0x7F)))"
 *     # the non-ASCII glyphs in EXTRA_ADVANCES, and the vertical metrics
 *     python -c "from fontTools.ttLib import TTFont; \
 *       f=TTFont('lib/og/fonts/SchibstedGrotesk-Bold.ttf'); c=f.getBestCmap(); \
 *       h=f['hmtx']; u=f['head'].unitsPerEm; \
 *       print({hex(g): round(h[c[g]][0]*1000/u) for g in (0xB7, 0x2013, 0x2019, 0x10F, 0x13E, 0x165)}, \
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

/** The body face (Schibsted Grotesk), at the two weights `brand.ts` ships. */
export type SansFace = 'schibstedBold' | 'schibstedRegular'

/** Every face with a table: the body weights plus the data face (Golos Text). */
type MeasuredFace = SansFace | 'data'

/** ASCII 0x20–0x7E advance widths, per 1000 em units. */
const TABLES: Record<MeasuredFace, number[]> = {
  schibstedBold:
    '210,281,400,719,600,911,695,221,348,348,406,635,292,463,304,479,632,404,597,603,627,616,616,568,612,617,304,296,635,635,635,564,922,738,679,717,736,605,584,730,753,426,589,726,579,936,768,768,672,768,689,648,689,718,714,1008,735,659,634,362,479,362,635,500,500,596,628,583,627,597,370,579,604,284,303,607,298,913,617,611,628,628,458,544,380,617,604,880,606,609,542,360,273,359,635'
      .split(',')
      .map(Number),
  schibstedRegular:
    '254,240,353,654,564,850,686,199,309,309,443,635,265,488,280,444,611,343,572,576,584,580,595,531,579,594,280,268,635,635,635,529,885,700,645,703,697,585,561,718,720,354,536,660,545,868,729,770,615,770,633,613,644,673,678,935,652,623,595,310,444,310,635,500,500,551,589,551,589,573,341,555,567,242,272,552,262,873,581,597,590,590,401,513,340,581,554,812,559,548,504,327,239,327,635'
      .split(',')
      .map(Number),
  data:
    '250,300,390,680,640,840,670,220,380,380,450,550,300,380,300,480,620,620,580,585,620,580,605,620,610,605,300,300,550,550,550,510,1020,670,670,680,700,610,595,705,760,314,555,690,570,860,760,740,655,740,680,640,620,730,670,960,660,640,600,380,480,380,550,500,260,565,615,545,615,580,365,615,615,268,268,565,320,885,615,600,615,615,390,530,400,605,560,810,550,560,510,380,340,380,550'
      .split(',')
      .map(Number),
}

/**
 * Exact advances for non-ASCII codepoints, consulted BEFORE accent folding.
 *
 * `·` separates rooms in the footer and `–` separates the two dates in the week
 * range, so both appear on essentially every card; left to the unknown-glyph
 * fallback they would be charged several times their real width.
 *
 * `ď ľ ť` are here because folding would UNDER-measure them: Schibsted Grotesk
 * draws their caron as a raised comma beside the letter, so each is wider than
 * the `d`, `l` or `t` it folds onto. Every other accented glyph in the shipped
 * subsets advances exactly like its base letter (the drift test checks the
 * whole subset), which is what lets `toMeasurable` stand in for a full table.
 */
const EXTRA_ADVANCES: Record<MeasuredFace, Record<number, number>> = {
  schibstedBold: { 0xb7: 304, 0x2013: 500, 0x2019: 292, 0x10f: 705, 0x13e: 329, 0x165: 421 },
  schibstedRegular: { 0xb7: 280, 0x2013: 500, 0x2019: 265, 0x10f: 628, 0x13e: 276, 0x165: 364 },
  data: { 0xb7: 300, 0x2013: 530, 0x2019: 300 },
}

/**
 * Descender depth as a fraction of font size, from each face's `hhea` table
 * (Schibsted Grotesk -258/1000, Golos Text -220/1000). Exported so the drift
 * test can check them against the font files.
 */
export const DESCENT_RATIO = { sans: 0.258, data: 0.22 } as const

const FIRST_CODE = 0x20
const LAST_CODE = 0x7e

/**
 * Width charged to a glyph this module has no table entry for.
 *
 * Set to the widest advance in the brand subsets (Schibsted Grotesk Regular
 * `Œ`, 1136) so unknown text is OVER-measured. Under-measuring runs text off
 * a canvas that has no clipping; over-measuring merely costs it a size step.
 *
 * The tables above cover the BRAND faces, which ship Latin only. Greek,
 * Cyrillic and the rest of the widened coverage are drawn from the separate
 * fallback face registered in `brand.ts`, and they stay on this conservative
 * path ON PURPOSE: carrying real advance tables for ~1,000 more glyphs would be
 * a large, hand-maintained table to buy back one size step on a card that
 * renders correctly either way.
 *
 * So there are two distinct reasons a glyph lands here, and they have
 * different consequences:
 *
 *   - Covered by the fallback (Greek, Cyrillic, Latin Ext-B): renders
 *     correctly, no outbound request, merely measured generously.
 *   - Covered by nothing (CJK, emoji, and the scripts listed in `brand.ts`):
 *     also triggers a Google Fonts / jsDelivr fetch mid-render, and the face
 *     that comes back is one this module cannot measure at all.
 *
 * Widening the fallback shrinks the second class but not the first, so this
 * constant stays whatever the widest brand advance is.
 */
const UNKNOWN_ADVANCE = 1136

/** The separator between named items in a fitted list. */
const LIST_SEPARATOR = ' · '

/**
 * Fold a string down to the ASCII the tables cover.
 *
 * Accents are dropped rather than looked up because every accented glyph in
 * these faces outside `EXTRA_ADVANCES` has exactly its base letter's advance,
 * so `é` measures as `e` with zero error, which keeps the tables at 95 entries
 * instead of several hundred.
 */
function toMeasurable(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '')
}

/** Width of `text` in px for any measured face, with per-character tracking. */
function measureFace(
  text: string,
  face: MeasuredFace,
  fontSize: number,
  letterSpacing: number
): number {
  const table = TABLES[face]
  const extra = EXTRA_ADVANCES[face]
  let units = 0
  let chars = 0
  for (const char of text.normalize('NFC')) {
    const code = char.codePointAt(0)!
    const exact =
      code >= FIRST_CODE && code <= LAST_CODE ? table[code - FIRST_CODE] : extra[code]
    if (exact !== undefined) {
      units += exact
      chars += 1
      continue
    }
    for (const folded of toMeasurable(char)) {
      const code = folded.codePointAt(0)!
      units += code >= FIRST_CODE && code <= LAST_CODE ? table[code - FIRST_CODE] : UNKNOWN_ADVANCE
      chars += 1
    }
  }
  return (units * fontSize) / 1000 + chars * letterSpacing
}

/**
 * Width of `text` in px, for a body face at `fontSize`.
 *
 * `letterSpacing` must be passed whenever the rendered element sets it: the
 * card tracks its headline at roughly -2.3% of the font size, which is far too
 * large to absorb as measurement slack.
 */
export function measureSans(
  text: string,
  face: SansFace,
  fontSize: number,
  letterSpacing = 0
): number {
  return measureFace(text, face, fontSize, letterSpacing)
}

/**
 * Width of `text` in px for the data face, including per-character tracking.
 *
 * A glyph outside the subset is charged the same conservative over-estimate as
 * the sans path, because Satori will resolve it from some other face at a
 * width this module cannot know, and here the result feeds the headline's
 * budget, so under-charging would hand the city more room than it actually has.
 */
export function measureData(text: string, fontSize: number, letterSpacing = 0): number {
  return measureFace(text, 'data', fontSize, letterSpacing)
}

/**
 * How far to raise a data run so its baseline meets a sans run's.
 *
 * Satori has an `alignItems: 'baseline'`, but it aligns the BOTTOM of the line
 * boxes rather than the baselines, so a 132px headline beside a 40px data run
 * drifts by the difference in their descender depths. The card therefore
 * aligns the boxes explicitly with `flex-end` and lifts the smaller run by
 * exactly that difference.
 *
 * `DESCENT_RATIO.sans` is the body face's. A headline drawn from the fallback
 * face instead (a Cyrillic or Greek city) sits on that face's descender
 * (0.293), so the state beside it lands roughly `citySize * 0.035` too high,
 * about 5px at the largest headline size. Left uncorrected on purpose: fixing
 * it means threading the resolved face through the fit path, and Satori
 * resolves it per grapheme, so a mixed-script name has no single answer to
 * thread.
 */
export function dataBaselineLift(sansSize: number, dataSize: number): number {
  return sansSize * DESCENT_RATIO.sans - dataSize * DESCENT_RATIO.data
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
 * `fitFontSize`'s data-face sibling: largest size in `[minSize, maxSize]` at
 * which a data run fits `maxWidth`.
 *
 * Separate from `fitFontSize` rather than a `face` parameter on it, because the
 * two differ in the unit of `tracking` and that difference changes the maths.
 * `fitFontSize` takes EM tracking, which scales with the size, so width is
 * linear in size and one proportional step lands on the fit. Data runs set an
 * ABSOLUTE px `letterSpacing` that does NOT scale, so width is affine
 * (`advance * size + tracking * chars`) and the proportional step overshoots by
 * the constant term. This solves the affine form directly, which is exact
 * rather than merely closer.
 */
export function fitDataSize(
  text: string,
  maxWidth: number,
  maxSize: number,
  minSize: number,
  tracking = 0
): number {
  if (measureData(text, maxSize, tracking) <= maxWidth) return maxSize
  // Two measurements recover the affine coefficients without this function
  // needing to know the advance table or how graphemes are counted.
  const constant = measureData(text, 0, tracking)
  const perSize = measureData(text, 1, 0)
  if (perSize <= 0) return maxSize
  return Math.max(minSize, Math.min(maxSize, Math.floor((maxWidth - constant) / perSize)))
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
