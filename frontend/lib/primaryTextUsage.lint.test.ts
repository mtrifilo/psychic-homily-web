import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

/**
 * Source guard for the rule in globals.css beside `--link`: orange text below
 * WCAG large size uses `text-link`, never `text-primary`, because light
 * `primary` fails 4.5:1 on every surface. `text-primary` stays legal on icons,
 * decorative glyphs and large text. A class string that sizes an icon box
 * (`h-4`, `w-5`, `size-4`) may carry a bare `text-primary`; everything else is
 * flagged:
 *
 * - a state variant (`hover:text-primary`, `focus-visible:text-primary`, ...),
 * - an opacity modifier (`text-primary/80`),
 * - a bare `text-primary` in a class string with no icon box size, which is
 *   how a text link usually looks (its size is inherited).
 *
 * A legitimate exception (a glyph span, an icon-only button's hover, a large
 * heading) goes in ALLOWED with the count of flagged tokens in that file and
 * the reason.
 */

const FRONTEND = path.resolve(__dirname, '..')

const ALLOWED: Record<string, { count: number; reason: string }> = {
  'features/home/components/HomeSectionList.tsx': { count: 1, reason: 'icon-only reorder button hover' },
  'features/scenes/components/GlobeCanvas.tsx': { count: 1, reason: 'aria-hidden chevron glyph button hover' },
  'features/shows/components/DayGroupedShowRow.tsx': { count: 1, reason: 'icon-only details button hover' },
  'features/shows/components/ShowCard.tsx': { count: 2, reason: 'icon-only details button hover' },
  'features/charts/components/ChartsPage.tsx': { count: 2, reason: 'text-3xl bold scene picker trigger, large text' },
  'features/shows/components/ShowHeader.tsx': { count: 1, reason: 'aria-hidden middot separator' },
  'features/festivals/components/FestivalLineup.tsx': { count: 1, reason: 'select-none middot separator' },
  'components/shared/EntityCardTitle.tsx': { count: 1, reason: 'expanded density is 20px bold, large text' },
  'app/admin/_components/ImportDropZone.tsx': { count: 1, reason: 'upload icon, sized in a sibling string' },
  'app/radio/_components/DialStationStrip.tsx': { count: 2, reason: 'aria-hidden note glyphs' },
  'app/radio/[station-slug]/[show-slug]/_components/RadioShowDetail.tsx': { count: 1, reason: 'aria-hidden on-air dot' },
  'app/radio/[station-slug]/[show-slug]/[date]/_components/EpisodeDateDetail.tsx': { count: 1, reason: 'aria-hidden on-air dot' },
  'app/radio/[station-slug]/[show-slug]/[date]/_components/PlaylistTable.tsx': { count: 2, reason: 'aria-hidden linked-artist dots' },
  'features/graph/components/ReplayScrubber.tsx': { count: 1, reason: 'played bar fill, a graphic' },
  'features/notifications/components/NotificationList.tsx': { count: 1, reason: 'aria-hidden icon tile, sized in a sibling string' },
  'features/radio/components/StationOnAirBox.tsx': { count: 2, reason: 'aria-hidden on-air dot and note glyph' },
  'features/scenes/components/AtlasSceneList.tsx': { count: 1, reason: 'role=img followed-scene star' },
  'features/shows/components/ReportShowDialog.tsx': { count: 1, reason: 'selected option icon wrapper' },
  'features/shows/components/ShowGigTimeline.tsx': { count: 1, reason: 'aria-hidden current-stop arrow' },
  'features/tags/components/TagOfficialIndicator.tsx': { count: 1, reason: 'badge icon; the Official label is text-link' },
  'features/home/components/LatestRadioShows.tsx': { count: 1, reason: 'aria-hidden live dot' },
}

const ICON_BOX = /(^|\s)(h|w|size)-[\d.]+(\s|$)/
const PRIMARY = /(^|[\s'"`])((?:[\w[\]&_.=-]+:)*)text-primary(\/\d+)?(?![\w-])/g

const sourceFiles = ['app', 'components', 'features', 'lib']
  .flatMap(dir =>
    fs
      .readdirSync(path.join(FRONTEND, dir), { recursive: true, encoding: 'utf8' })
      .map(f => path.join(dir, f).split(path.sep).join('/'))
  )
  .filter(f => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f))

/** Comments may name the classes; only code counts. */
const code = (source: string) =>
  source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')

/** Flagged `text-primary` tokens in one file's string literals. */
function flagged(source: string): string[] {
  const hits: string[] = []
  for (const quoted of code(source).match(/(["'`])(?:\\.|(?!\1)[^\\])*\1/g) ?? []) {
    const literal = quoted.slice(1, -1)
    for (const m of literal.matchAll(PRIMARY)) {
      const [, , variant, opacity = ''] = m
      if (variant || opacity || !ICON_BOX.test(literal)) hits.push(`${variant}text-primary${opacity}`)
    }
  }
  return hits
}

describe('small orange text uses the link token', () => {
  const found = Object.fromEntries(
    sourceFiles
      .map(file => [file, flagged(fs.readFileSync(path.join(FRONTEND, file), 'utf8'))] as const)
      .filter(([, hits]) => hits.length > 0)
  )

  it('flags no text-sized text-primary outside the allowlist', () => {
    const unexpected = Object.entries(found)
      .filter(([file, hits]) => hits.length !== (ALLOWED[file]?.count ?? 0))
      .map(([file, hits]) => `${file}: ${hits.join(', ')}`)
    expect(unexpected).toEqual([])
  })

  it('keeps every allowlist entry live', () => {
    const stale = Object.keys(ALLOWED).filter(file => !(file in found))
    expect(stale).toEqual([])
  })

  it('catches each shape it guards', () => {
    expect(flagged(`<a className="text-sm text-primary">x</a>`)).toEqual(['text-primary'])
    expect(flagged(`<Link className="font-medium text-primary hover:underline">x</Link>`)).toEqual(['text-primary'])
    expect(flagged(`<a className="text-foreground hover:text-primary">x</a>`)).toEqual(['hover:text-primary'])
    expect(flagged(`<span className="text-primary/80">x</span>`)).toEqual(['text-primary/80'])
    expect(flagged(`<Star className="h-4 w-4 text-primary" />`)).toEqual([])
    expect(flagged(`<b className="bg-primary text-primary-foreground text-sm">x</b>`)).toEqual([])
  })
})
