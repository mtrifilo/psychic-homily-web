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
  it('carries every glyph the cards actually draw', () => {
    const required = 'psychichomily.com0123456789 ·–ABCDEFGHIJKLMNOPQRSTUVWXYZ'
    for (const [, path] of SANS) {
      const font = parseFont(path)
      for (const char of required) {
        expect(
          font.advance(char.codePointAt(0)!),
          `${path} is missing '${char}'`
        ).not.toBeNull()
      }
    }
  })
})
