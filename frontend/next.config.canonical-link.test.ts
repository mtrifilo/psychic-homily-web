import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { existsSync, readdirSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import type { Metadata } from 'next'
import { PHASE_PRODUCTION_SERVER } from 'next/constants'
import { getPathMatch } from 'next/dist/shared/lib/router/utils/path-match'
import { compileNonPath } from 'next/dist/shared/lib/router/utils/prepare-destination'
import { modifyRouteRegex } from 'next/dist/lib/redirect-status'
import nextConfigForPhase, { SELF_CANONICAL_ROUTES } from './next.config'
import { SITE_URL } from './lib/seo/siteMetadata'
import { jsonBodyResponse } from './lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error('NEXT_NOT_FOUND')
  }),
  redirect: vi.fn(),
}))

// The collection page reads the session cookie before its fetch; an anonymous
// request is the crawler's view.
vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({ get: () => undefined })),
  headers: vi.fn(async () => new Headers()),
}))

import { generateMetadata as artistMetadata } from './app/artists/[slug]/page'
import { generateMetadata as releaseMetadata } from './app/releases/[slug]/page'
import { generateMetadata as venueMetadata } from './app/venues/[slug]/page'
import { generateMetadata as labelMetadata } from './app/labels/[slug]/page'
import { generateMetadata as festivalMetadata } from './app/festivals/[slug]/page'
import { generateMetadata as tagMetadata } from './app/tags/[slug]/page'
import { generateMetadata as collectionMetadata } from './app/collections/[slug]/page'
import { generateMetadata as showMetadata } from './app/shows/[slug]/page'
import { generateMetadata as sceneMetadata } from './app/scenes/[slug]/page'
import { generateMetadata as sceneTonightMetadata } from './app/scenes/[slug]/tonight/page'
import { generateMetadata as radioStationMetadata } from './app/radio/[station-slug]/page'
import { generateMetadata as radioShowMetadata } from './app/radio/[station-slug]/[show-slug]/page'
import { generateMetadata as radioEpisodeMetadata } from './app/radio/[station-slug]/[show-slug]/[date]/page'
import { generateMetadata as radioChannelMetadata } from './app/radio/[station-slug]/channel/[sub-slug]/page'
import { generateMetadata as blogMetadata } from './app/blog/[slug]/page'
import { generateMetadata as mixMetadata } from './app/dj-sets/[slug]/page'
import { generateMetadata as categoryMetadata } from './app/categories/[category]/page'
import { getAllCategories, getBlogSlugs, getCategorySlug } from './features/blog/utils/blog'
import { getMixSlugs } from './features/blog/utils/mixes'

type HeaderRule = { source: string; headers: { key: string; value: string }[] }

async function loadHeaderRules(): Promise<HeaderRule[]> {
  return (await nextConfigForPhase(PHASE_PRODUCTION_SERVER).headers!()) as HeaderRule[]
}

/**
 * The loaded config's `headers()` rules, compiled with the options
 * `server/lib/router-utils/filesystem.js` uses.
 */
const compiledRules = loadHeaderRules().then(rules =>
  rules.map(rule => ({
    headers: rule.headers,
    match: getPathMatch(rule.source, {
      strict: true,
      removeUnnamedParams: true,
      regexModifier: (regex: string) => modifyRouteRegex(regex),
    }),
  })),
)

/**
 * The response headers Next's router adds for `pathname`: values substituted
 * with `compileNonPath` as `resolve-routes.js` does, later rules overwriting
 * earlier ones key by key.
 */
async function configHeadersFor(pathname: string): Promise<Record<string, string>> {
  const out: Record<string, string> = {}
  for (const { match, headers } of await compiledRules) {
    const params = match(pathname)
    if (params === false) continue
    const hasParams = Object.keys(params).length > 0
    for (const { key, value } of headers) {
      out[hasParams ? compileNonPath(key, params) : key] = hasParams
        ? compileNonPath(value, params)
        : value
    }
  }
  return out
}

/** The URL a `Link: <url>; rel="canonical"` response header declares, or null. */
async function headerCanonicalFor(pathname: string): Promise<string | null> {
  const link = (await configHeadersFor(pathname)).Link
  const match = link?.match(/^<([^>]+)>; rel="canonical"$/)
  return match ? match[1] : null
}

function canonicalOf(meta: Metadata): string | undefined {
  const canonical = meta.alternates?.canonical
  return typeof canonical === 'string' ? canonical : canonical?.toString()
}

/**
 * The record every backend read answers with. It carries the fields each page's
 * existence check and snippet builders read. Its `slug` is deliberately NOT the
 * requested one, so a page that takes its canonical from loaded data rather
 * than from the request path fails the agreement test.
 */
function entityRecord(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 7,
    slug: 'slug-as-stored',
    name: 'Sample',
    title: 'Sample',
    city: 'Chicago',
    state: 'IL',
    social: {},
    stats: {},
    artists: [{ id: 1, name: 'Sample Band', slug: 'sample-band', is_headliner: true }],
    venues: [{ id: 2, name: 'Sample Room', slug: 'sample-room', city: 'Chicago', state: 'IL' }],
    event_date: '2026-12-01T03:00:00Z',
    shows: [],
    usage_count: 0,
    is_public: true,
    show_name: 'Sample Show',
    station_name: 'Sample Station',
    air_date: '2026-07-24',
    play_count: 0,
    network: null,
    ...overrides,
  }
}

const fetchMock = vi.fn()

beforeEach(() => {
  fetchMock.mockImplementation(async () => jsonBodyResponse(entityRecord()))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const params = <T,>(value: T) => ({ params: Promise.resolve(value) })

type CoveredCase = [pathname: string, pageMetadata: () => Promise<Metadata>]

const blogSlugs = getBlogSlugs()
const mixSlugs = getMixSlugs()
const categorySlugs = getAllCategories().map(getCategorySlug)

/**
 * One sample request per covered family, with the metadata its page declares
 * for that request. Content-backed families run over everything they publish.
 */
const COVERED_CASES: CoveredCase[] = [
  ['/artists/the-hoyle-brothers', () => artistMetadata(params({ slug: 'the-hoyle-brothers' }))],
  ['/releases/a-long-way-from-kansas', () => releaseMetadata(params({ slug: 'a-long-way-from-kansas' }))],
  ['/venues/thalia-hall-chicago-il', () => venueMetadata(params({ slug: 'thalia-hall-chicago-il' }))],
  ['/labels/12xu', () => labelMetadata(params({ slug: '12xu' }))],
  ['/festivals/levitation-2026', () => festivalMetadata(params({ slug: 'levitation-2026' }))],
  ['/tags/post-punk', () => tagMetadata(params({ slug: 'post-punk' }))],
  ['/collections/desert-noise', () => collectionMetadata(params({ slug: 'desert-noise' }))],
  // The show page canonicalizes to the LOADED show's slug. A non-numeric
  // segment is looked up by exact slug match, so the loaded slug is the
  // requested one; that backend contract is what makes this family coverable.
  [
    '/shows/dev-lemons-2026-02-13',
    () => {
      fetchMock.mockImplementation(async () =>
        jsonBodyResponse(entityRecord({ slug: 'dev-lemons-2026-02-13' })),
      )
      return showMetadata(params({ slug: 'dev-lemons-2026-02-13' }))
    },
  ],
  ['/scenes/chicago-il', () => sceneMetadata(params({ slug: 'chicago-il' }))],
  ['/scenes/st.-louis-mo', () => sceneMetadata(params({ slug: 'st.-louis-mo' }))],
  [
    '/radio/kexp/drive-time',
    () => radioShowMetadata(params({ 'station-slug': 'kexp', 'show-slug': 'drive-time' })),
  ],
  [
    '/radio/kexp/drive-time/2026-07-24',
    () =>
      radioEpisodeMetadata(
        params({ 'station-slug': 'kexp', 'show-slug': 'drive-time', date: '2026-07-24' }),
      ),
  ],
  [
    '/radio/wfmu/channel/drummer',
    () => radioChannelMetadata(params({ 'station-slug': 'wfmu', 'sub-slug': 'drummer' })),
  ],
  ...blogSlugs.map((slug): CoveredCase => [`/blog/${slug}`, () => blogMetadata(params({ slug }))]),
  ...mixSlugs.map((slug): CoveredCase => [`/dj-sets/${slug}`, () => mixMetadata(params({ slug }))]),
  ...categorySlugs.map(
    (category): CoveredCase => [`/categories/${category}`, () => categoryMetadata(params({ category }))],
  ),
]

describe('canonical Link header on self-canonical entity routes', () => {
  it('samples published content in every content-backed family', () => {
    expect(blogSlugs.length).toBeGreaterThan(0)
    expect(mixSlugs.length).toBeGreaterThan(0)
    expect(categorySlugs.length).toBeGreaterThan(0)
  })

  it.each(COVERED_CASES)('declares the page canonical for %s', async (pathname, pageMetadata) => {
    const pageCanonical = canonicalOf(await pageMetadata())

    expect(pageCanonical).toBe(`${SITE_URL}${pathname}`)
    expect(await headerCanonicalFor(pathname)).toBe(pageCanonical)
  })

  it('has one header rule per self-canonical route', async () => {
    const linkSources = (await loadHeaderRules())
      .filter(rule => rule.headers.some(header => header.key === 'Link'))
      .map(rule => rule.source)

    expect(linkSources).toEqual(SELF_CANONICAL_ROUTES.map(route => route.source))
  })
})

describe('no canonical Link header where the page declares a different URL', () => {
  it('leaves a numeric show URL alone, whose page canonical is the slug URL', async () => {
    fetchMock.mockImplementation(async () =>
      jsonBodyResponse(entityRecord({ slug: 'dev-lemons-2026-02-13' })),
    )
    const pageCanonical = canonicalOf(await showMetadata(params({ slug: '2044' })))

    expect(pageCanonical).toBe(`${SITE_URL}/shows/dev-lemons-2026-02-13`)
    expect(await headerCanonicalFor('/shows/2044')).toBeNull()
  })

  it('leaves a radio station alone, whose page canonical can be its channel URL', async () => {
    fetchMock.mockImplementation(async () =>
      jsonBodyResponse(
        entityRecord({
          slug: 'wfmu-drummer',
          network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
        }),
      ),
    )
    const pageCanonical = canonicalOf(
      await radioStationMetadata(params({ 'station-slug': 'wfmu-drummer' })),
    )

    expect(pageCanonical).toBe(`${SITE_URL}/radio/wfmu/channel/drummer`)
    expect(await headerCanonicalFor('/radio/wfmu-drummer')).toBeNull()
    expect(await headerCanonicalFor('/radio/kexp')).toBeNull()
  })

  it('leaves scene windows alone, whose page canonical names the slug the backend returns', async () => {
    fetchMock.mockImplementation(async () =>
      jsonBodyResponse(
        entityRecord({
          slug: 'phoenix-az',
          date: '2026-11-19',
          iso_week: '2026-W47',
          prev_date: '2026-11-18',
          next_date: '2026-11-20',
          is_tonight: true,
        }),
      ),
    )
    const pageCanonical = canonicalOf(await sceneTonightMetadata(params({ slug: 'mesa-az' })))

    expect(pageCanonical).toBe(`${SITE_URL}/scenes/phoenix-az/tonight`)
    for (const pathname of [
      '/scenes/mesa-az/tonight',
      '/scenes/mesa-az/week',
      '/scenes/mesa-az/this-weekend',
      '/scenes/mesa-az/next-4-weeks',
      '/scenes/mesa-az/2026-W31',
      '/scenes/mesa-az/2026-07-31',
    ]) {
      expect(await headerCanonicalFor(pathname), pathname).toBeNull()
    }
  })

  it.each([
    '/shows/saved',
    '/shows/2026',
    '/artists/42',
    '/radio/kexp/drive-time/24-07-2026',
    '/scenes/espa%C3%B1ola-nm',
    '/artists',
    '/venues/thalia-hall-chicago-il/shows/2025',
  ])('sends no canonical Link header for %s', async pathname => {
    expect(await headerCanonicalFor(pathname)).toBeNull()
  })
})

const APP_DIR = join(__dirname, 'app')

/** Every `page.tsx` under app/, as its route directory relative to app/. */
function pageRouteDirs(dir = APP_DIR): string[] {
  const found: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue
    const child = join(dir, entry.name)
    if (existsSync(join(child, 'page.tsx'))) found.push(relative(APP_DIR, child).split(sep).join('/'))
    found.push(...pageRouteDirs(child))
  }
  return found
}

/**
 * Every dynamic page route, mapped to the self-canonical route that covers it
 * or to the reason it sends no canonical header. A new dynamic page fails the
 * inventory test until it is placed here.
 */
const DYNAMIC_PAGE_DISPOSITIONS: Record<string, { covered: string } | { excluded: string }> = {
  'artists/[slug]': { covered: '/artists/:slug' },
  'releases/[slug]': { covered: '/releases/:slug' },
  'venues/[slug]': { covered: '/venues/:slug' },
  'labels/[slug]': { covered: '/labels/:slug' },
  'festivals/[slug]': { covered: '/festivals/:slug' },
  'tags/[slug]': { covered: '/tags/:slug' },
  'collections/[slug]': { covered: '/collections/:slug' },
  'shows/[slug]': { covered: '/shows/:slug' },
  'scenes/[slug]': { covered: '/scenes/:slug' },
  'radio/[station-slug]/[show-slug]': { covered: '/radio/:station/:show' },
  'radio/[station-slug]/[show-slug]/[date]': { covered: '/radio/:station/:show/:date' },
  'radio/[station-slug]/channel/[sub-slug]': { covered: '/radio/:station/channel/:channel' },
  'blog/[slug]': { covered: '/blog/:slug' },
  'dj-sets/[slug]': { covered: '/dj-sets/:slug' },
  'categories/[category]': { covered: '/categories/:category' },
  'radio/[station-slug]': { excluded: "canonical follows the station's network" },
  'scenes/[slug]/tonight': { excluded: 'canonical names the scene slug the backend returns' },
  'scenes/[slug]/week': { excluded: 'canonical names the scene slug the backend returns' },
  'scenes/[slug]/this-weekend': { excluded: 'canonical names the scene slug the backend returns' },
  'scenes/[slug]/next-4-weeks': { excluded: 'canonical names the scene slug the backend returns' },
  'scenes/[slug]/[period]': { excluded: 'canonical names the scene slug and date the backend returns' },
  'shows/[slug]/[month]': { excluded: 'calendar list; canonical comes from the parsed window' },
  'shows/[slug]/[month]/[day]': { excluded: 'calendar list; canonical comes from the parsed window' },
  'venues/[slug]/shows/[year]': { excluded: 'year archive; canonical comes from the parsed year' },
  'charts/[module]': { excluded: 'chart list; canonical depends on module and window validation' },
  'charts/[module]/[period]': { excluded: 'chart archive; canonical depends on period validation' },
  'requests/[id]': { excluded: 'declares no canonical' },
  'users/[username]': { excluded: 'declares no canonical' },
}

describe('route inventory', () => {
  const pageDirs = pageRouteDirs()

  it('places every dynamic page route', () => {
    expect(pageDirs.filter(dir => dir.includes('[')).sort()).toEqual(
      Object.keys(DYNAMIC_PAGE_DISPOSITIONS).sort(),
    )
  })

  it('covers exactly the self-canonical routes the config declares', () => {
    const covered = Object.values(DYNAMIC_PAGE_DISPOSITIONS).flatMap(disposition =>
      'covered' in disposition ? [disposition.covered] : [],
    )
    expect(covered.sort()).toEqual(SELF_CANONICAL_ROUTES.map(route => route.path).sort())
  })

  // A fixed-path page that sits beside a covered dynamic segment (`shows/submit`
  // beside `shows/[slug]`) is not that entity, so it must not match the rule.
  it('sends no canonical Link header to a fixed-path page beside a covered route', async () => {
    const parentOf = (dir: string) => dir.split('/').slice(0, -1).join('/')
    const coveredParents = new Set(
      Object.entries(DYNAMIC_PAGE_DISPOSITIONS)
        .filter(([, disposition]) => 'covered' in disposition)
        .map(([dir]) => parentOf(dir)),
    )
    const fixedSiblings = pageDirs.filter(
      dir => coveredParents.has(parentOf(dir)) && !dir.split('/').pop()!.startsWith('['),
    )

    expect(fixedSiblings).toContain('shows/submit')
    for (const dir of fixedSiblings) {
      expect(await headerCanonicalFor(`/${dir}`), dir).toBeNull()
    }
  })
})

describe('htmlLimitedBots', () => {
  // Unset on purpose. Adding a script-running crawler such as Googlebot to it
  // sends that crawler past the cached shell on Vercel to a static-generation
  // render, which answers HTTP 500 on the entity routes; the canonical reaches
  // Google through the Link header instead.
  it('is not configured', () => {
    expect(nextConfigForPhase(PHASE_PRODUCTION_SERVER).htmlLimitedBots).toBeUndefined()
  })
})
