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
 * - an opacity modifier (`text-primary/80`, and `text-link/80`, since `link`
 *   at any opacity fails 4.5:1 somewhere),
 * - a bare `text-primary` in any other class string: a text link (its size
 *   is usually inherited) or sized text.
 *
 * Known limits: each string literal is judged alone, so a text size passed in
 * a sibling `cn()` argument, a variant spliced in through `${}`, or text that
 * inherits its size inside an icon-sized box is not seen. Every scanned file
 * must parse cleanly, so a parse failure cannot hide a file's literals.
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
  'app/radio/[station-slug]/[show-slug]/[date]/_components/PlaylistTable.tsx': { tokens: ['text-primary', 'text-primary', 'text-primary'], reason: 'aria-hidden linked-artist dots and now arrow' },
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
// and a box that also sets a text size (`w-16 text-[11px]`) holds text. Sizes
// and text sizes may carry variants (`sm:size-5`, `sm:text-sm`), and a text
// size may carry a line height (`text-xs/5`).
const VARIANTS = String.raw`(?:[^\s:]+:)*`
const utility = (prefix: string) =>
  new RegExp(String.raw`(^|\s)${VARIANTS}${prefix}-(?:[\d.]+|\[[\d.]+px\])(\s|$)`)
const SIZE = utility('size')
const HEIGHT = utility('h')
const WIDTH = utility('w')
const TEXT_SIZE = new RegExp(
  String.raw`(^|\s)${VARIANTS}text-(xs|sm|base|lg|xl|\d*xl|\[[\d.]+(px|rem|em)\]|\[length:[^\]]+\])(\/[\w.]+)?(\s|$)`
)
const isIconBox = (literal: string) =>
  (SIZE.test(literal) || (HEIGHT.test(literal) && WIDTH.test(literal))) && !TEXT_SIZE.test(literal)
const PRIMARY = /(^|\s)((?:[^\s:]+:)*)!?text-primary(\/\d+)?!?(?![\w-])/g
const LINK_OPACITY = /(^|\s)((?:[^\s:]+:)*)!?text-link(\/\d+)!?(?![\w-])/g

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
  const kind = fileName.endsWith('.tsx') ? ts.ScriptKind.TSX : ts.ScriptKind.TS
  const file = ts.createSourceFile(fileName, source, ts.ScriptTarget.Latest, false, kind)
  // `parseDiagnostics` is not in the public typings; a missing array fails
  // loudly rather than skipping the check.
  const diagnostics = (file as unknown as { parseDiagnostics?: readonly ts.Diagnostic[] }).parseDiagnostics
  if (!diagnostics) throw new Error('typescript no longer exposes parseDiagnostics')
  if (diagnostics.length > 0) {
    throw new Error(`${fileName} does not parse, so its literals cannot be scanned`)
  }
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
    for (const m of literal.matchAll(LINK_OPACITY)) hits.push(`${m[2]}text-link${m[3]}`)
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
    expect(flagged(`<Check className="h-[18px] w-[18px] sm:size-5 text-primary" />`)).toEqual([])
    expect(flagged(`<span className="h-6 w-6 sm:text-sm text-primary">x</span>`)).toEqual(['text-primary'])
    expect(flagged(`<span className="size-8 text-xs/5 text-primary">x</span>`)).toEqual(['text-primary'])
    expect(flagged(`<a className="text-link hover:text-link/80">x</a>`)).toEqual(['hover:text-link/80'])
    expect(
      flagged(`export const pick = <T,>(x: T): T => x\nexport const c = 'hover:text-primary'`, 'x.ts')
    ).toEqual(['hover:text-primary'])
    expect(() => flagged(`const = 'text-primary'`, 'x.ts')).toThrow(/does not parse/)
    expect(flagged(`<b className="bg-primary text-primary-foreground text-sm">x</b>`)).toEqual([])
    expect(flagged(`// a comment naming text-primary\nconst x = 1`)).toEqual([])
  })
})
