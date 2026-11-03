import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

/**
 * Source guard for the 12px text floor: text that carries information is
 * `text-xs` (12px) or larger. A bracketed size under 12px (`text-[11px]`,
 * `text-[0.625rem]`, any variant) in app/, components/, features/ or lib/, or
 * a `font-size` under 12px in app/globals.css, is flagged.
 *
 * The scan reads raw source, comments included, so a size cannot hide in a
 * class string a regex over JSX would miss. Exceptions are `aria-hidden`
 * decorative glyphs and the rem-sized unread chip; each goes in ALLOWED with
 * the exact tokens in that file and the reason, so a new violation cannot hide
 * behind a removed exception.
 */

const FRONTEND = path.resolve(__dirname, '..')
const FLOOR_PX = 12
const ROOT_PX = 16

const ALLOWED: Record<string, { tokens: string[]; reason: string }> = {
  'components/shared/UnreadCountBadge.tsx': { tokens: ['text-[0.625rem]', 'text-[0.625rem]'], reason: 'aria-hidden rem-sized count chip over an icon (class and its comment)' },
  'features/home/components/LatestRadioShows.tsx': { tokens: ['text-[8px]'], reason: 'aria-hidden live dot glyph' },
  'features/profile/components/ContributorProfilePreview.tsx': { tokens: ['text-[8px]'], reason: 'aria-hidden visibility dot glyph' },
  'features/profile/components/TierAdvancementCard.tsx': { tokens: ['text-[8px]'], reason: 'aria-hidden unmet-requirement dot glyph' },
}

const BRACKET_SIZE = /text-\[(\d*\.?\d+)(px|rem)\]/g
const CSS_FONT_SIZE = /font-size:\s*(\d*\.?\d+)(px|rem)/g

const toPx = (value: string, unit: string) => Number(value) * (unit === 'rem' ? ROOT_PX : 1)

/** Sub-floor size tokens in one source file. */
function flagged(source: string): string[] {
  const hits: string[] = []
  for (const m of source.matchAll(BRACKET_SIZE)) {
    if (toPx(m[1], m[2]) < FLOOR_PX) hits.push(m[0])
  }
  return hits
}

/** Sub-floor `font-size` declarations in a stylesheet. */
function flaggedCss(source: string): string[] {
  const hits: string[] = []
  for (const m of source.matchAll(CSS_FONT_SIZE)) {
    if (toPx(m[1], m[2]) < FLOOR_PX) hits.push(m[0])
  }
  return hits
}

const sourceFiles = ['app', 'components', 'features', 'lib']
  .flatMap(dir =>
    fs
      .readdirSync(path.join(FRONTEND, dir), { recursive: true, encoding: 'utf8' })
      .map(f => path.join(dir, f).split(path.sep).join('/'))
  )
  .filter(f => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f))

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
          `${file}: ${hits.join(', ')} (use text-xs; for an aria-hidden glyph, update ALLOWED with the tokens and reason)`
      )
    expect(unexpected).toEqual([])
  })

  it('keeps every allowlist entry live', () => {
    const stale = Object.keys(ALLOWED).filter(file => !(file in found))
    expect(stale).toEqual([])
  })

  it('sets no sub-12px font-size in globals.css', () => {
    expect(flaggedCss(fs.readFileSync(path.join(FRONTEND, 'app/globals.css'), 'utf8'))).toEqual([])
  })

  it('catches each shape it guards', () => {
    expect(flagged(`<span className="text-[11px]">x</span>`)).toEqual(['text-[11px]'])
    expect(flagged(`'sm:text-[10.5px] uppercase'`)).toEqual(['text-[10.5px]'])
    expect(flagged(`'[&_th]:text-[9px]'`)).toEqual(['text-[9px]'])
    expect(flagged(`'text-[0.625rem]'`)).toEqual(['text-[0.625rem]'])
    expect(flaggedCss(`.x { font-size: 11px; }`)).toEqual(['font-size: 11px'])
  })

  it('passes sizes at or above the floor', () => {
    expect(flagged(`'text-xs text-[12px] text-[12.5px] lg:text-[13px] text-[0.75rem]'`)).toEqual([])
    expect(flaggedCss(`.x { font-size: 12px; } .y { font-size: 0.875rem; }`)).toEqual([])
  })
})
