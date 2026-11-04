import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

/**
 * Source guard for the 12px text floor: text that carries information is
 * `text-xs` (12px) or larger. Three literal shapes under 12px are flagged in
 * app/, components/, features/ and lib/ (.ts and .tsx, tests excluded) and in
 * app/globals.css:
 *
 * - a bracketed Tailwind size, any variant (`text-[11px]`, `sm:text-[0.625rem]`),
 * - a CSS declaration, including one built into a cssText string (`font-size: 11px`),
 * - a React style string (`fontSize: '10px'`).
 *
 * The scan reads raw source, comments included. Known limits: a size computed
 * at runtime (`font-size: ${px}px`, `sceneLabelSizePx()`), a unitless number
 * (`fontSize: 10`, the canvas, SVG and OG-image renderers), and arbitrary
 * values such as `text-[length:...]` or `em` units are not seen.
 *
 * Exceptions go in ALLOWED with the exact tokens in that file and the reason,
 * so a new violation cannot hide behind a removed exception.
 */

const FRONTEND = path.resolve(__dirname, '..')
const FLOOR_PX = 12
const ROOT_PX = 16

const ALLOWED: Record<string, { tokens: string[]; reason: string }> = {
  'components/shared/UnreadCountBadge.tsx': { tokens: ['text-[0.625rem]', 'text-[0.625rem]'], reason: 'aria-hidden rem-sized count chip over an icon (class and its comment)' },
  'features/home/components/LatestRadioShows.tsx': { tokens: ['text-[8px]'], reason: 'aria-hidden live dot glyph' },
  'features/profile/components/ContributorProfilePreview.tsx': { tokens: ['text-[8px]'], reason: 'aria-hidden visibility dot glyph' },
  'features/profile/components/TierAdvancementCard.tsx': { tokens: ['text-[8px]'], reason: 'aria-hidden unmet-requirement dot glyph' },
  'features/scenes/components/GlobeCanvas.tsx': { tokens: ['font-size: 11px'], reason: 'aria-hidden venue-stack count inside a fixed-size map pin; Atlas map label sizes are their own decision' },
}

const SIZE = /text-\[(\d*\.?\d+)(px|rem)\]|font-size:\s*(\d*\.?\d+)(px|rem)|fontSize:\s*['"](\d*\.?\d+)(px|rem)['"]/g

const toPx = (value: string, unit: string) => Number(value) * (unit === 'rem' ? ROOT_PX : 1)

/** Literal sub-floor sizes in one source file. */
function flagged(source: string): string[] {
  const hits: string[] = []
  for (const m of source.matchAll(SIZE)) {
    const value = m[1] ?? m[3] ?? m[5]
    const unit = m[2] ?? m[4] ?? m[6]
    if (toPx(value, unit) < FLOOR_PX) hits.push(m[0])
  }
  return hits
}

const sourceFiles = [
  'app/globals.css',
  ...['app', 'components', 'features', 'lib']
    .flatMap(dir =>
      fs
        .readdirSync(path.join(FRONTEND, dir), { recursive: true, encoding: 'utf8' })
        .map(f => path.join(dir, f).split(path.sep).join('/'))
    )
    .filter(f => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f)),
]

describe('text sizes stay at or above the 12px floor', () => {
  const found = Object.fromEntries(
    sourceFiles
      .map(file => [file, flagged(fs.readFileSync(path.join(FRONTEND, file), 'utf8'))] as const)
      .filter(([, hits]) => hits.length > 0)
  )

  it('flags no sub-12px text size outside the allowlist', () => {
    const sorted = (tokens: readonly string[]) => [...tokens].sort().join(' ')
    const unexpected = Object.entries(found)
      .filter(([file, hits]) => sorted(hits) !== sorted(ALLOWED[file]?.tokens ?? []))
      .map(
        ([file, hits]) =>
          `${file}: ${hits.join(', ')} (use text-xs or 12px; for an aria-hidden glyph, update ALLOWED with the tokens and reason)`
      )
    expect(unexpected).toEqual([])
  })

  it('keeps every allowlist entry live', () => {
    const stale = Object.keys(ALLOWED).filter(file => !(file in found))
    expect(stale).toEqual([])
  })

  it('catches each shape it guards', () => {
    expect(flagged(`<span className="text-[11px]">x</span>`)).toEqual(['text-[11px]'])
    expect(flagged(`'sm:text-[10.5px] uppercase'`)).toEqual(['text-[10.5px]'])
    expect(flagged(`'[&_th]:text-[9px]'`)).toEqual(['text-[9px]'])
    expect(flagged(`'text-[0.625rem]'`)).toEqual(['text-[0.625rem]'])
    expect(flagged(`.x { font-size: 11px; }`)).toEqual(['font-size: 11px'])
    expect(flagged(`['color: red', 'font-size:10px'].join(';')`)).toEqual(['font-size:10px'])
    expect(flagged(`style={{ fontSize: '10px' }}`)).toEqual([`fontSize: '10px'`])
  })

  it('passes sizes at or above the floor', () => {
    expect(flagged(`'text-xs text-[12px] text-[12.5px] lg:text-[13px] text-[0.75rem]'`)).toEqual([])
    expect(flagged(`.x { font-size: 12px; } .y { font-size: 0.875rem; }`)).toEqual([])
    expect(flagged(`style={{ fontSize: '12px' }}`)).toEqual([])
  })
})
