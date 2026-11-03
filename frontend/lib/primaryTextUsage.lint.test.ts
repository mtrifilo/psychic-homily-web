import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'
import ts from 'typescript'

/**
 * Source guard for the rule in globals.css beside `--link`: orange text below
 * WCAG large size uses `text-link`, never `text-primary`, because light
 * `primary` fails 4.5:1 on every surface. `text-primary` stays legal on icons,
 * decorative glyphs and large text. A class string that sizes an icon box
 * (`size-4`, or both `h-4` and `w-4`) and sets no text size may carry a bare
 * `text-primary`; everything else is flagged:
 *
 * - a state variant (`hover:text-primary`, `focus-visible:text-primary`, ...),
 * - an opacity modifier (`text-primary/80`),
 * - a bare `text-primary` in any other class string: a text link (its size
 *   is usually inherited) or sized text.
 *
 * A legitimate exception (a glyph span, an icon-only button's hover, a large
 * heading) goes in ALLOWED with the exact flagged tokens in that file and the
 * reason, so a new violation cannot hide behind a removed exception.
 */

const FRONTEND = path.resolve(__dirname, '..')

const ALLOWED: Record<string, { tokens: string[]; reason: string }> = {
  'features/home/components/HomeSectionList.tsx': { tokens: ['hover:text-primary'], reason: 'icon-only reorder button hover' },
  'features/scenes/components/GlobeCanvas.tsx': { tokens: ['hover:text-primary'], reason: 'aria-hidden chevron glyph button hover' },
  'features/shows/components/DayGroupedShowRow.tsx': { tokens: ['hover:text-primary'], reason: 'icon-only details button hover' },
  'features/shows/components/ShowCard.tsx': { tokens: ['hover:text-primary', 'hover:text-primary'], reason: 'icon-only details button hover' },
  'features/charts/components/ChartsPage.tsx': { tokens: ['text-primary'], reason: 'text-3xl bold scene picker trigger, large text' },
  'features/shows/components/ShowHeader.tsx': { tokens: ['text-primary/60'], reason: 'aria-hidden middot separator' },
  'features/festivals/components/FestivalLineup.tsx': { tokens: ['text-primary/30'], reason: 'select-none middot separator' },
  'components/shared/EntityCardTitle.tsx': { tokens: ['group-hover:text-primary'], reason: 'expanded density is 20px bold, large text' },
  'app/admin/_components/ImportDropZone.tsx': { tokens: ['text-primary'], reason: 'upload icon, sized in a sibling string' },
  'app/radio/_components/DialStationStrip.tsx': { tokens: ['text-primary', 'text-primary'], reason: 'aria-hidden note glyphs' },
  'app/radio/[station-slug]/[show-slug]/_components/RadioShowDetail.tsx': { tokens: ['text-primary'], reason: 'aria-hidden on-air dot' },
  'app/radio/[station-slug]/[show-slug]/[date]/_components/EpisodeDateDetail.tsx': { tokens: ['text-primary'], reason: 'aria-hidden on-air dot' },
  'app/radio/[station-slug]/[show-slug]/[date]/_components/PlaylistTable.tsx': { tokens: ['text-primary', 'text-primary'], reason: 'aria-hidden linked-artist dots' },
  'features/graph/components/ReplayScrubber.tsx': { tokens: ['text-primary'], reason: 'played bar fill, a graphic' },
  'features/notifications/components/NotificationList.tsx': { tokens: ['text-primary'], reason: 'aria-hidden icon tile, sized in a sibling string' },
  'features/radio/components/StationOnAirBox.tsx': { tokens: ['text-primary', 'text-primary'], reason: 'aria-hidden on-air dot and note glyph' },
  'features/scenes/components/AtlasSceneList.tsx': { tokens: ['text-primary'], reason: 'role=img followed-scene star' },
  'features/shows/components/ReportShowDialog.tsx': { tokens: ['text-primary'], reason: 'selected option icon wrapper' },
  'features/shows/components/ShowGigTimeline.tsx': { tokens: ['text-primary'], reason: 'aria-hidden current-stop arrow' },
  'features/tags/components/TagOfficialIndicator.tsx': { tokens: ['text-primary'], reason: 'badge icon; the Official label is text-link' },
  'features/home/components/LatestRadioShows.tsx': { tokens: ['text-primary'], reason: 'aria-hidden live dot' },
  'app/radio/[station-slug]/[show-slug]/_components/EpisodeArchiveTable.tsx': { tokens: ['text-primary'], reason: 'aria-hidden live dot' },
  'features/comments/components/CommentVoteControls.tsx': { tokens: ['text-primary'], reason: 'icon-only upvote button, sized in the enclosing template' },
}

// An icon box: a square size, or both a width and a height, with no text size
// beside it. A lone height is a control height (`h-7 px-2` on a text button),
// and a box that also sets a text size (`w-16 text-[11px]`) holds text.
const utility = (prefix: string) => new RegExp(`(^|\\s)${prefix}-[\\d.]+(\\s|$)`)
const SIZE = utility('size')
const HEIGHT = utility('h')
const WIDTH = utility('w')
const TEXT_SIZE = /(^|\s)text-(xs|sm|base|lg|xl|\d*xl|\[[\d.]+(px|rem|em)\])(\s|$)/
const isIconBox = (literal: string) =>
  (SIZE.test(literal) || (HEIGHT.test(literal) && WIDTH.test(literal))) && !TEXT_SIZE.test(literal)
const PRIMARY = /(^|\s)((?:[^\s:]+:)*)!?text-primary(\/\d+)?!?(?![\w-])/g

const sourceFiles = ['app', 'components', 'features', 'lib']
  .flatMap(dir =>
    fs
      .readdirSync(path.join(FRONTEND, dir), { recursive: true, encoding: 'utf8' })
      .map(f => path.join(dir, f).split(path.sep).join('/'))
  )
  .filter(f => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f))

/**
 * Every string literal's static text, read from the TypeScript AST so
 * comments, regex literals and JSX text never shift what counts as a string.
 * A template literal's static parts are joined into one string, because they
 * render into one class attribute.
 */
function literals(source: string, fileName = 'x.tsx'): string[] {
  const file = ts.createSourceFile(fileName, source, ts.ScriptTarget.Latest, false, ts.ScriptKind.TSX)
  const out: string[] = []
  const visit = (node: ts.Node) => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      out.push(node.text)
    } else if (ts.isTemplateExpression(node)) {
      out.push([node.head.text, ...node.templateSpans.map(span => span.literal.text)].join(' '))
    }
    ts.forEachChild(node, visit)
  }
  visit(file)
  return out
}

/** Flagged `text-primary` tokens in one file's string literals. */
function flagged(source: string, fileName?: string): string[] {
  const hits: string[] = []
  for (const literal of literals(source, fileName)) {
    for (const m of literal.matchAll(PRIMARY)) {
      const [, , variant, opacity = ''] = m
      if (variant || opacity || !isIconBox(literal)) hits.push(`${variant}text-primary${opacity}`)
    }
  }
  return hits
}

describe('small orange text uses the link token', () => {
  const found = Object.fromEntries(
    sourceFiles
      .map(file => [file, flagged(fs.readFileSync(path.join(FRONTEND, file), 'utf8'), file)] as const)
      .filter(([, hits]) => hits.length > 0)
  )

  it('flags no text-sized text-primary outside the allowlist', () => {
    const sorted = (tokens: readonly string[]) => [...tokens].sort().join(' ')
    const unexpected = Object.entries(found)
      .filter(([file, hits]) => sorted(hits) !== sorted(ALLOWED[file]?.tokens ?? []))
      .map(
        ([file, hits]) =>
          `${file}: ${hits.join(', ')} (use text-link for text; for an icon, glyph or large text, update ALLOWED with the tokens and reason)`
      )
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
    expect(flagged(`<a className="group-hover/row:text-primary">x</a>`)).toEqual(['group-hover/row:text-primary'])
    expect(flagged(`<a className="[&>a]:text-primary">x</a>`)).toEqual(['[&>a]:text-primary'])
    expect(flagged(`<a className="*:text-primary">x</a>`)).toEqual(['*:text-primary'])
    expect(flagged(`<a className="text-primary! hover:!text-primary">x</a>`)).toEqual(['text-primary', 'hover:text-primary'])
    expect(flagged(`<span className="w-16 shrink-0 text-[11px] text-primary">x</span>`)).toEqual(['text-primary'])
    expect(flagged('const c = `${base} text-primary`')).toEqual(['text-primary'])
    expect(
      flagged(`const n = 1 // doesn't pin\nconst r = /a="?b"/\nconst c = cn('x', 'hover:text-primary')`)
    ).toEqual(['hover:text-primary'])
    expect(flagged(`<Button className="h-7 px-2 text-primary">Confirm</Button>`)).toEqual(['text-primary'])
    expect(flagged(`<Star className="h-4 w-4 text-primary" />`)).toEqual([])
    expect(flagged(`<Check className="size-4 shrink-0 text-primary" />`)).toEqual([])
    expect(flagged(`<b className="bg-primary text-primary-foreground text-sm">x</b>`)).toEqual([])
    expect(flagged(`// a comment naming text-primary\nconst x = 1`)).toEqual([])
  })
})
