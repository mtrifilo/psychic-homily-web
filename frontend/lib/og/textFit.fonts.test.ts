import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { DESCENT_RATIO, measureMono, measureSans, type SansFace } from './textFit'

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
const SANS: Array<[SansFace, string]> = [
  ['satoshiBold', `${FONTS}/Satoshi-Bold.ttf`],
  ['satoshiMedium', `${FONTS}/Satoshi-Medium.ttf`],
  ['satoshiRegular', `${FONTS}/Satoshi-Regular.ttf`],
]

/** Measuring one character at 1000px yields its advance per 1000 em directly. */
const advanceOf = (char: string, face: SansFace) => measureSans(char, face, 1000)

describe.each(SANS)('%s advance table', (face, path) => {
  const font = parseFont(path)

  it('matches the font file across the whole ASCII range', () => {
    const mismatches: string[] = []
    for (let cp = 0x20; cp <= 0x7e; cp++) {
      const real = font.advance(cp)
      const table = advanceOf(String.fromCodePoint(cp), face)
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
      expect(advanceOf(char, face)).toBeCloseTo(font.advance(char.codePointAt(0)!)!, 1)
    }
  })

  it('measures accented Latin as its base letter, as the folding assumes', () => {
    for (const [accented, base] of [
      ['é', 'e'],
      ['ñ', 'n'],
      ['ü', 'u'],
      ['ł', 'l'],
    ]) {
      expect(font.advance(accented.codePointAt(0)!)).toBeCloseTo(
        font.advance(base.codePointAt(0)!)!,
        1
      )
    }
  })

  it('keeps the descent ratio the baseline correction is built on', () => {
    expect(font.descentRatio).toBeCloseTo(DESCENT_RATIO.sans, 4)
  })
})

describe('Space Mono metrics', () => {
  const font = parseFont(`${FONTS}/SpaceMono-Regular.ttf`)

  it('is genuinely monospaced at the advance the module assumes', () => {
    for (const cp of [0x20, 0x41, 0x69, 0x57, 0x2e, 0x30]) {
      expect(font.advance(cp)).toBeCloseTo(measureMono(String.fromCodePoint(cp), 1000), 1)
    }
  })

  it('keeps the descent ratio the baseline correction is built on', () => {
    expect(font.descentRatio).toBeCloseTo(DESCENT_RATIO.mono, 4)
  })
})

describe('subset coverage', () => {
  // Space Mono has no advance table (it is monospaced, so one constant covers
  // it), but it draws the range, the state and the wordmark — a re-subset that
  // dropped its digits or the en dash would be just as visible on a card.
  const ALL_FACES = [...SANS.map(([, path]) => path), `${FONTS}/SpaceMono-Regular.ttf`]

  it('carries every glyph the cards actually draw, in every shipped face', () => {
    const required = 'psychichomily.com0123456789 ·–ABCDEFGHIJKLMNOPQRSTUVWXYZ'
    for (const path of ALL_FACES) {
      const font = parseFont(path)
      for (const char of required) {
        expect(
          font.advance(char.codePointAt(0)!),
          `${path} is missing '${char}'`
        ).not.toBeNull()
      }
    }
  })

  it('keeps every face on the same em square and descent the card assumes', () => {
    for (const path of ALL_FACES) {
      const font = parseFont(path)
      expect(font.unitsPerEm, path).toBe(1000)
      const expected = path.includes('SpaceMono') ? DESCENT_RATIO.mono : DESCENT_RATIO.sans
      expect(font.descentRatio, path).toBeCloseTo(expected, 4)
    }
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
  const FALLBACKS = [`${FONTS}/NotoSans-Regular.ttf`, `${FONTS}/NotoSans-Bold.ttf`]
  const BRAND = [...SANS.map(([, path]) => path), `${FONTS}/SpaceMono-Regular.ttf`]

  /** One representative per script the cards promise to render offline. */
  const COVERED: Array<[string, string]> = [
    ['Greek', 'Αθήνα'],
    ['Cyrillic', 'Москва'],
    ['Cyrillic (Ukrainian)', 'Київ'],
    ['Cyrillic (Serbian)', 'Београд'],
    ['Latin Ext-B', 'Ǆ'],
    ['Latin Ext Additional (Vietnamese)', 'Đà Nẵng'],
    ['General Punctuation', '‚„…‹›‰'],
    ['Letterlike', '№™'],
  ]

  it.each(COVERED)('renders %s from the shipped bytes, with no network fetch', (_label, sample) => {
    const faces = [...BRAND, ...FALLBACKS].map(parseFont)
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

  it('ships both weights so a bold headline is not drawn at regular weight', () => {
    // The city headline and the show title are the largest text on either card
    // and both are weight 700; Satori picks the nearest weight WITHIN a family,
    // so a single 400 face would quietly render every non-Latin headline light.
    for (const path of FALLBACKS) {
      const font = parseFont(path)
      expect(font.advance(0x0410), `${path} is missing Cyrillic А`).not.toBeNull()
    }
    const [regular, bold] = FALLBACKS.map(parseFont)
    expect(bold.advance(0x0410)).not.toBeCloseTo(regular.advance(0x0410)!, 1)
  })

  it('carries no Latin, so the brand face always wins there', () => {
    // Satori searches the requested family first, but a fallback carrying Latin
    // would still be dead weight in an edge bundle — and would mask a genuine
    // regression in the Satoshi subset behind a Noto Sans glyph.
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
    const faces = [...BRAND, ...FALLBACKS].map(parseFont)
    const uncovered: Array<[string, number]> = [
      ['CJK', 0x4e2d],
      ['Hangul', 0xac00],
      ['Hiragana', 0x3042],
      ['Arabic', 0x0627],
      ['Hebrew', 0x05d0],
      ['Thai', 0x0e01],
      ['Devanagari', 0x0905],
      ['Misc Symbols (★)', 0x2605],
    ]
    for (const [label, cp] of uncovered) {
      expect(
        faces.some(f => f.advance(cp) !== null),
        `${label} U+${cp.toString(16).toUpperCase()} is now covered — update the residual-gap section of brand.ts`
      ).toBe(false)
    }
  })
})
