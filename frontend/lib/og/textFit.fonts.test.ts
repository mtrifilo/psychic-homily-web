import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { brotliCompressSync, constants as zlib } from 'node:zlib'
import { DESCENT_RATIO, measureData, measureSans, type SansFace } from './textFit'

/**
 * Drift guard between the shipped font files and the numbers copied out of them.
 *
 * Every other test in this directory checks the advance tables against
 * themselves, so all of them stay green if someone replaces or re-subsets a
 * face without re-running the regeneration recipe — and the only symptom would
 * be share cards quietly clipping or under-filling in every feed they land in.
 * This reads the actual `.ttf` bytes instead.
 *
 * The reader below is deliberately minimal (table directory, `head`, `hhea`,
 * `cmap` format 4, `hmtx`) rather than a dependency: it only has to understand
 * the four files in this repo.
 */

interface Font {
  unitsPerEm: number
  descentRatio: number
  /** Advance in units per 1000 em, or null when the glyph is absent. */
  advance(codePoint: number): number | null
}

function parseFont(path: string): Font {
  const buf = readFileSync(path)

  const tables: Record<string, number> = {}
  const numTables = buf.readUInt16BE(4)
  for (let i = 0; i < numTables; i++) {
    const p = 12 + i * 16
    tables[buf.toString('ascii', p, p + 4)] = buf.readUInt32BE(p + 8)
  }

  const unitsPerEm = buf.readUInt16BE(tables.head + 18)
  const descent = buf.readInt16BE(tables.hhea + 6)
  const numberOfHMetrics = buf.readUInt16BE(tables.hhea + 34)

  // Pick the Windows Unicode BMP cmap subtable.
  const cmap = tables.cmap
  let sub = -1
  const subtables = buf.readUInt16BE(cmap + 2)
  for (let i = 0; i < subtables; i++) {
    const p = cmap + 4 + i * 8
    const platform = buf.readUInt16BE(p)
    const encoding = buf.readUInt16BE(p + 2)
    if (platform === 3 && (encoding === 1 || encoding === 10)) {
      sub = cmap + buf.readUInt32BE(p + 4)
      break
    }
  }
  if (sub < 0 || buf.readUInt16BE(sub) !== 4) {
    throw new Error(`${path}: expected a format 4 Unicode cmap`)
  }

  const segX2 = buf.readUInt16BE(sub + 6)
  const endO = sub + 14
  const startO = endO + segX2 + 2
  const deltaO = startO + segX2
  const rangeO = deltaO + segX2

  function glyphFor(codePoint: number): number {
    for (let i = 0; i < segX2 / 2; i++) {
      if (codePoint > buf.readUInt16BE(endO + i * 2)) continue
      const start = buf.readUInt16BE(startO + i * 2)
      if (codePoint < start) return 0
      const delta = buf.readInt16BE(deltaO + i * 2)
      const rangeOffset = buf.readUInt16BE(rangeO + i * 2)
      if (rangeOffset === 0) return (codePoint + delta) & 0xffff
      const g = buf.readUInt16BE(rangeO + i * 2 + rangeOffset + (codePoint - start) * 2)
      return g === 0 ? 0 : (g + delta) & 0xffff
    }
    return 0
  }

  return {
    unitsPerEm,
    descentRatio: Math.abs(descent) / unitsPerEm,
    advance(codePoint: number) {
      const glyph = glyphFor(codePoint)
      if (glyph === 0) return null
      const metric = Math.min(glyph, numberOfHMetrics - 1)
      return (buf.readUInt16BE(tables.hmtx + metric * 4) * 1000) / unitsPerEm
    },
  }
}

const FONTS = 'lib/og/fonts'

/**
 * Every brand face with an advance table, with the function that measures it.
 * Measuring one character at 1000px yields its advance per 1000 em directly.
 */
interface MeasuredFace {
  label: string
  path: string
  advanceOf: (char: string) => number
  descentRatio: number
}

const sansFace = (face: SansFace, file: string): MeasuredFace => ({
  label: face,
  path: `${FONTS}/${file}`,
  advanceOf: char => measureSans(char, face, 1000),
  descentRatio: DESCENT_RATIO.sans,
})

const MEASURED: MeasuredFace[] = [
  sansFace('schibstedBold', 'SchibstedGrotesk-Bold.ttf'),
  sansFace('schibstedRegular', 'SchibstedGrotesk-Regular.ttf'),
  {
    label: 'data',
    path: `${FONTS}/GolosText-Regular.ttf`,
    advanceOf: char => measureData(char, 1000),
    descentRatio: DESCENT_RATIO.data,
  },
]
const DATA = MEASURED[2].path
const BRAND_FACES = MEASURED.map(face => face.path)

describe.each(MEASURED)('$label advance table', ({ path, advanceOf, descentRatio }) => {
  const font = parseFont(path)

  it('matches the font file across the whole ASCII range', () => {
    const mismatches: string[] = []
    for (let cp = 0x20; cp <= 0x7e; cp++) {
      const real = font.advance(cp)
      const table = advanceOf(String.fromCodePoint(cp))
      if (real === null || Math.abs(real - table) > 0.5) {
        mismatches.push(`U+${cp.toString(16)} '${String.fromCodePoint(cp)}': font ${real} vs table ${table}`)
      }
    }
    expect(mismatches).toEqual([])
  })

  // These two are on essentially every card — `·` between room names and `–`
  // between the two dates — so a wrong value here skews every footer.
  it('matches the font file for the punctuation the cards emit', () => {
    for (const char of ['·', '–', '’']) {
      expect(advanceOf(char)).toBeCloseTo(font.advance(char.codePointAt(0)!)!, 1)
    }
  })

  // The folding in `textFit` measures an accented letter as its base letter, so
  // any glyph in the subset that is WIDER than its base is under-measured
  // unless it has its own entry. Checked across every Latin-1 and Latin
  // Extended-A codepoint, not a sample: the body face's `ď ľ ť` are wider than
  // `d l t`, and a sample would not have found them. A glyph the face does not
  // carry at all is drawn from another family, so it must be charged the
  // over-estimate rather than its base letter.
  it('measures every accented Latin glyph at its real advance, or over-measures it when absent', () => {
    const unknown = advanceOf('東')
    const mismatches: string[] = []
    for (let cp = 0xa0; cp <= 0x17f; cp++) {
      const real = font.advance(cp)
      const char = String.fromCodePoint(cp)
      const base = char.normalize('NFD').replace(/\p{M}/gu, '')
      // Only glyphs that FOLD onto one ASCII letter are measured by the table;
      // anything else is charged the deliberate over-estimate.
      if (base.length !== 1 || base.codePointAt(0)! > 0x7e) continue
      const measured = advanceOf(char)
      if (real === null ? measured !== unknown : Math.abs(measured - real) > 0.5) {
        mismatches.push(`U+${cp.toString(16)} '${char}': font ${real} vs measured ${measured}`)
      }
    }
    expect(mismatches).toEqual([])
  })

  // Anything the tables do not cover is charged one fixed advance, and that
  // only over-measures if no glyph this face ships is wider. A re-subset that
  // brings in a wider glyph has to raise the constant with it.
  it('charges unknown glyphs at least the widest advance it ships', () => {
    let widest = 0
    for (let cp = 0x20; cp <= 0x2122; cp++) {
      widest = Math.max(widest, font.advance(cp) ?? 0)
    }
    expect(advanceOf('東')).toBeGreaterThanOrEqual(widest)
  })

  it('keeps the descent ratio the baseline correction is built on', () => {
    expect(font.descentRatio).toBeCloseTo(descentRatio, 4)
  })
})

describe('Golos Text figures', () => {
  const font = parseFont(DATA)

  // Satori never applies the `tnum` feature, so the data face's tabular figures
  // exist only because the recipe in `brand.ts` points the cmap at them. The
  // proportional `1` is 485/1000 against a tabular 620; this is what fails if
  // a regeneration drops that step.
  it('draws the tabular figures by default, as the frozen tnum makes them', () => {
    const figures = '0123456789'.split('').map(d => font.advance(d.codePointAt(0)!)!)
    expect(font.advance('1'.codePointAt(0)!)).toBe(font.advance('0'.codePointAt(0)!))
    // Golos's own tabular set spans 580 to 620, not one width.
    expect(Math.max(...figures) - Math.min(...figures)).toBeLessThanOrEqual(40)
  })
})

describe('subset coverage', () => {
  it('carries every glyph the cards actually draw, in every shipped face', () => {
    const required = 'psychichomily.com0123456789 ·–ABCDEFGHIJKLMNOPQRSTUVWXYZ'
    for (const path of BRAND_FACES) {
      const font = parseFont(path)
      for (const char of required) {
        expect(
          font.advance(char.codePointAt(0)!),
          `${path} is missing '${char}'`
        ).not.toBeNull()
      }
    }
  })

  it('keeps every face on the 1000-unit em square the tables assume', () => {
    for (const path of BRAND_FACES) {
      expect(parseFont(path).unitsPerEm, path).toBe(1000)
    }
  })

  it('keeps the brand faces inside the edge budget they ship in', () => {
    // Every OG route is an Edge Function under a 1 MiB cap that `next build`
    // does not enforce, and the largest deploys with about 14 KiB of brotli
    // headroom. The brand faces are the biggest assets in it; brotli is what
    // the deployed size counts. A third Schibsted weight (~23.8 KB) fails here.
    const brotli = BRAND_FACES.reduce(
      (sum, path) =>
        sum +
        brotliCompressSync(readFileSync(path), {
          params: { [zlib.BROTLI_PARAM_QUALITY]: 11 },
        }).byteLength,
      0
    )
    expect(brotli).toBeLessThan(64_000)
  })
})

/**
 * The guard on the outbound-request behaviour documented in `brand.ts`.
 *
 * Satori resolves a grapheme against every supplied face and only fetches a
 * typeface from Google Fonts when NONE of them has it — so "is this codepoint
 * in the shipped bytes?" is exactly "does rendering it stay offline?". That
 * makes this the one assertion standing between a re-subset and the silent
 * return of a third-party request carrying DB-derived text.
 *
 * Failure here is not cosmetic. Read the `brand.ts` header before widening or
 * narrowing anything below.
 */
describe('fallback face script coverage', () => {
  const FALLBACKS = [`${FONTS}/NotoSans-Regular.ttf`]

  /**
   * One representative per LIVING language the cards promise to render offline.
   *
   * Named by language rather than by Unicode block on purpose: the subset is
   * enumerated per language to fit the 1 MiB edge limit, so "the Cyrillic block"
   * is not the unit that ships and testing it as one would misdescribe what is
   * guaranteed. Ukrainian `ґ` and Romanian `ș` sit outside the base blocks and
   * are exactly the characters a block-shaped subset would have dropped.
   */
  const COVERED: Array<[string, string]> = [
    ['Greek', 'Αθήνα'],
    ['Greek (Thessaloniki)', 'Θεσσαλονίκη'],
    ['Russian', 'Москва'],
    ['Ukrainian', 'Київ'],
    ['Ukrainian (ge with upturn)', 'Ґ'],
    ['Serbian', 'Београд'],
    ['Bulgarian', 'София'],
    ['Macedonian', 'Скопје'],
    ['Belarusian', 'Мінск'],
    ['Kazakh / Azeri schwa', 'Әә'],
    ['Romanian (comma below)', 'Șș Țț'],
    ['Azerbaijani schwa', 'Əə'],
    ['Croatian digraph', 'Ǆǆ'],
    ['General Punctuation', '–—‘’“”•…‧'],
    ['Numero', '№'],
  ]

  it.each(COVERED)('renders %s from the shipped bytes, with no network fetch', (_label, sample) => {
    const faces = [...BRAND_FACES, ...FALLBACKS].map(parseFont)
    for (const char of sample) {
      const cp = char.codePointAt(0)!
      const covered = faces.some(f => f.advance(cp) !== null)
      expect(
        covered,
        `U+${cp.toString(16).toUpperCase()} '${char}' is in no shipped face — ` +
          `rendering it would fetch a typeface from Google Fonts at render time`
      ).toBe(true)
    }
  })

  it('stays within the byte budget the 1 MiB edge limit leaves it', () => {
    // Not style policing. These routes are Edge Functions and the largest
    // deploys at ~0.986 of a 1 MiB cap, with about 14 KiB of brotli headroom,
    // a limit `next build` does not enforce, so it fails at DEPLOY.
    // A re-subset that quietly grows this file is the most likely way to break
    // the deploy, and this is the only place that would catch it first.
    const bytes = readFileSync(`${FONTS}/NotoSans-Regular.ttf`).byteLength
    expect(bytes).toBeLessThan(40_000)
  })

  it('charges unknown glyphs at least the widest advance the fallback ships', () => {
    // Text the tables cannot measure may be drawn from this face, so the
    // over-estimate has to cover it too, not only the brand faces.
    for (const path of FALLBACKS) {
      const font = parseFont(path)
      let widest = 0
      for (let cp = 0x20; cp <= 0x2122; cp++) {
        widest = Math.max(widest, font.advance(cp) ?? 0)
      }
      expect(measureSans('東', 'schibstedBold', 1000), path).toBeGreaterThanOrEqual(widest)
    }
  })

  it('ships exactly one fallback weight, as the budget requires', () => {
    // A second weight cost 121 KB raw and pushed the function over the limit.
    // If someone adds one back, the size guard above should be the thing that
    // fails -- but assert the count too, so the reason is legible.
    expect(FALLBACKS).toHaveLength(1)
  })

  it('carries no Latin, so the brand face always wins there', () => {
    // Satori searches the requested family first, but a fallback carrying Latin
    // would still be dead weight in an edge bundle — and would mask a genuine
    // regression in a brand subset behind a Noto Sans glyph.
    for (const path of FALLBACKS) {
      const font = parseFont(path)
      for (const char of 'AZaz09') {
        expect(font.advance(char.codePointAt(0)!), `${path} should not carry '${char}'`).toBeNull()
      }
    }
  })

  it('does NOT claim the scripts brand.ts documents as still fetching', () => {
    // Pins the residual gap so it stays a written decision rather than drifting
    // into an assumption. If one of these ever starts passing, the gap section
    // of the `brand.ts` header is out of date.
    const faces = [...BRAND_FACES, ...FALLBACKS].map(parseFont)
    const uncovered: Array<[string, number]> = [
      ['CJK', 0x4e2d],
      ['Hangul', 0xac00],
      ['Hiragana', 0x3042],
      ['Arabic', 0x0627],
      ['Hebrew', 0x05d0],
      ['Thai', 0x0e01],
      ['Devanagari', 0x0905],
      ['Misc Symbols (★)', 0x2605],
      // Dropped to fit the edge limit, not because it is unwanted -- `Đà Nẵng`
      // draws Đ and à from the brand face but fetches ẵ. Cheapest thing to add
      // back if the limit ever rises; see the gap section of brand.ts.
      ['Vietnamese (Latin Ext Additional)', 0x1eb5],
    ]
    for (const [label, cp] of uncovered) {
      expect(
        faces.some(f => f.advance(cp) !== null),
        `${label} U+${cp.toString(16).toUpperCase()} is now covered — update the residual-gap section of brand.ts`
      ).toBe(false)
    }
  })
})
