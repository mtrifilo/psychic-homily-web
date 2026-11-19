import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { existsSync, readdirSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import type { Metadata } from 'next'
import type { NextRequest } from 'next/server'
import { PHASE_PRODUCTION_SERVER } from 'next/constants'
import { getPathMatch } from 'next/dist/shared/lib/router/utils/path-match'
import { parse as parseQuery } from 'node:querystring'
import type { RouteHas } from 'next/dist/lib/load-custom-routes'
import * as pageStaticInfo from 'next/dist/build/analysis/get-page-static-info'
import type { ProxyMatcher } from 'next/dist/build/analysis/get-page-static-info'
import { getMiddlewareRouteMatcher } from 'next/dist/shared/lib/router/utils/middleware-route-matcher'
import { compileNonPath, matchHas } from 'next/dist/shared/lib/router/utils/prepare-destination'
import { modifyRouteRegex } from 'next/dist/lib/redirect-status'
import nextConfigForPhase, { SELF_CANONICAL_ROUTES } from './next.config'
import { clearExistenceVerdictsForTests, config as proxyConfig, proxy } from './proxy'
import { SITE_URL } from './lib/seo/siteMetadata'
import { jsonBodyResponse } from './lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error('NEXT_NOT_FOUND')
  }),
  redirect: vi.fn(),
}))

import { generateMetadata as artistMetadata } from './app/artists/[slug]/page'
import { generateMetadata as releaseMetadata } from './app/releases/[slug]/page'
import { generateMetadata as venueMetadata } from './app/venues/[slug]/page'
import { generateMetadata as labelMetadata } from './app/labels/[slug]/page'
import { generateMetadata as festivalMetadata } from './app/festivals/[slug]/page'
import { generateMetadata as tagMetadata } from './app/tags/[slug]/page'
import { generateMetadata as showMetadata } from './app/shows/[slug]/page'
import { generateMetadata as showsMonthMetadata } from './app/shows/[slug]/[month]/page'
import { generateMetadata as showsDayMetadata } from './app/shows/[slug]/[month]/[day]/page'
import { generateMetadata as venueYearMetadata } from './app/venues/[slug]/shows/[year]/page'
import { generateMetadata as sceneTonightMetadata } from './app/scenes/[slug]/tonight/page'
import { generateMetadata as radioStationMetadata } from './app/radio/[station-slug]/page'

type HeaderRule = {
  source: string
  has?: RouteHas[]
  missing?: RouteHas[]
  headers: { key: string; value: string }[]
}

async function loadHeaderRules(): Promise<HeaderRule[]> {
  return (await nextConfigForPhase(PHASE_PRODUCTION_SERVER).headers!()) as HeaderRule[]
}

/** A route `source` compiled with the options `server/lib/router-utils/filesystem.js` uses. */
function compileSource(source: string) {
  return getPathMatch(source, {
    strict: true,
    removeUnnamedParams: true,
    regexModifier: (regex: string) => modifyRouteRegex(regex),
  })
}

const compiledRules = loadHeaderRules().then(rules =>
  rules.map(rule => ({ ...rule, match: compileSource(rule.source) })),
)

/** The path of a request target, without its query string. */
const pathOf = (target: string) => target.split('?')[0]

/**
 * The response headers Next's router adds for a request target (path plus
 * optional query): `has`/`missing` checked with `matchHas` and values
 * substituted with `compileNonPath`, as `resolve-routes.js` does, later rules
 * overwriting earlier ones key by key.
 */
async function configHeadersFor(target: string): Promise<Record<string, string>> {
  const [pathname, search = ''] = target.split('?')
  const query = parseQuery(search)
  const out: Record<string, string> = {}
  for (const { match, has, missing, headers } of await compiledRules) {
    const pathParams = match(pathname)
    if (pathParams === false) continue
    const conditionParams = matchHas(
      { headers: {} } as unknown as Parameters<typeof matchHas>[0],
      query,
      has,
      missing,
    )
    if (conditionParams === false) continue
    const params = { ...pathParams, ...conditionParams }
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
    years: [{ year: 2025, count: 3 }],
    total: 3,
    usage_count: 0,
    network: null,
    ...overrides,
  }
}

const fetchMock = vi.fn()

/** Every backend read answers with `entityRecord(overrides)` until the next call. */
function backendAnswers(overrides: Record<string, unknown> = {}) {
  fetchMock.mockImplementation(async () => jsonBodyResponse(entityRecord(overrides)))
}

beforeEach(() => {
  backendAnswers()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const params = <T,>(value: T) => ({ params: Promise.resolve(value) })
const noSearch = { searchParams: Promise.resolve({}) }

type CoveredCase = [pathname: string, pageMetadata: () => Promise<Metadata>]

/**
 * The covered dynamic page routes, keyed by route directory under app/, each
 * with the self-canonical route that covers it and sample requests paired with
 * the metadata its page declares for them.
 *
 * `calendar` marks a list window rather than an entity: every well-formed
 * window inside the addressable span is a real page at HTTP 200 (an empty one
 * is noindexed, not missing), and the proxy 404s one outside the span, which
 * proxy.shows-calendar.test.ts pins.
 */
const COVERED: Record<string, { route: string; cases: CoveredCase[]; calendar?: true }> = {
  'artists/[slug]': {
    route: '/artists/:slug',
    cases: [['/artists/the-hoyle-brothers', () => artistMetadata(params({ slug: 'the-hoyle-brothers' }))]],
  },
  'releases/[slug]': {
    route: '/releases/:slug',
    cases: [
      ['/releases/a-long-way-from-kansas', () => releaseMetadata(params({ slug: 'a-long-way-from-kansas' }))],
    ],
  },
  'venues/[slug]': {
    route: '/venues/:slug',
    cases: [
      ['/venues/thalia-hall-chicago-il', () => venueMetadata(params({ slug: 'thalia-hall-chicago-il' }))],
    ],
  },
  'labels/[slug]': {
    route: '/labels/:slug',
    cases: [['/labels/12xu', () => labelMetadata(params({ slug: '12xu' }))]],
  },
  'festivals/[slug]': {
    route: '/festivals/:slug',
    cases: [['/festivals/levitation-2026', () => festivalMetadata(params({ slug: 'levitation-2026' }))]],
  },
  'tags/[slug]': {
    route: '/tags/:slug',
    cases: [['/tags/post-punk', () => tagMetadata(params({ slug: 'post-punk' }))]],
  },
  // The show page and the venue year archive canonicalize to the LOADED
  // entity's slug. A non-numeric slug is looked up by exact match, so the
  // loaded slug is the requested one; that backend contract is what makes
  // these two coverable, and the stubs below stand for it.
  'shows/[slug]': {
    route: '/shows/:slug',
    cases: [
      [
        '/shows/dev-lemons-2026-02-13',
        () => {
          backendAnswers({ slug: 'dev-lemons-2026-02-13' })
          return showMetadata(params({ slug: 'dev-lemons-2026-02-13' }))
        },
      ],
    ],
  },
  'venues/[slug]/shows/[year]': {
    route: '/venues/:slug/shows/:year',
    cases: [
      [
        '/venues/thalia-hall-chicago-il/shows/2025',
        () => {
          backendAnswers({ slug: 'thalia-hall-chicago-il' })
          return venueYearMetadata(params({ slug: 'thalia-hall-chicago-il', year: '2025' }))
        },
      ],
    ],
  },
  'shows/[slug]/[month]': {
    route: '/shows/:year/:month',
    calendar: true,
    cases: [
      ['/shows/2026/11', () => showsMonthMetadata(params({ slug: '2026', month: '11' }))],
      ['/shows/2100/01', () => showsMonthMetadata(params({ slug: '2100', month: '01' }))],
    ],
  },
  'shows/[slug]/[month]/[day]': {
    route: '/shows/:year/:month/:day',
    calendar: true,
    cases: [
      [
        '/shows/2026/11/14',
        () => showsDayMetadata({ ...params({ slug: '2026', month: '11', day: '14' }), ...noSearch }),
      ],
    ],
  },
}

/**
 * Every other dynamic page route, with the reason it sends no canonical header.
 * A new dynamic page fails the inventory test until it is placed here or in
 * `COVERED`.
 */
const EXCLUDED: Record<string, string> = {
  'scenes/[slug]':
    'a metro member city (`/scenes/tempe-az`) and any case variant render the metro under their own URL; whether those are canonical is undecided',
  'scenes/[slug]/tonight': 'canonical names the scene slug the backend returns',
  'scenes/[slug]/week': 'canonical names the scene slug the backend returns',
  'scenes/[slug]/this-weekend': 'canonical names the scene slug the backend returns',
  'scenes/[slug]/next-4-weeks': 'canonical names the scene slug the backend returns',
  'scenes/[slug]/[period]': 'canonical names the scene slug and date the backend returns',
  'collections/[slug]': 'outside the proxy existence checks: a missing or private collection answers HTTP 200',
  'radio/[station-slug]': "canonical follows the station's network",
  'radio/[station-slug]/[show-slug]':
    'outside the proxy existence checks (a missing show answers HTTP 200), and the station segment is not checked',
  'radio/[station-slug]/[show-slug]/[date]':
    'outside the proxy existence checks (a missing episode answers HTTP 200), and the station segment is not checked',
  'radio/[station-slug]/channel/[sub-slug]': 'outside the proxy existence checks: a missing station answers HTTP 200',
  'blog/[slug]': 'prerendered with the canonical inside <head>; unknown slugs are outside the proxy existence checks',
  'dj-sets/[slug]': 'prerendered with the canonical inside <head>; unknown slugs are outside the proxy existence checks',
  'categories/[category]':
    'prerendered with the canonical inside <head>; unknown slugs are outside the proxy existence checks',
  'charts/[module]':
    'a year archive before launch or in the future passes the proxy and soft-404s at HTTP 200; the module drill-downs are self-canonical and proxy-checked but are not announced in any sitemap',
  'charts/[module]/[period]':
    'a quarter before launch or in the future passes the proxy shape check and soft-404s at HTTP 200',
  'requests/[id]': 'declares no canonical',
  'users/[username]': 'declares no canonical',
}

const COVERED_CASES = Object.values(COVERED).flatMap(family => family.cases)

describe('canonical Link header on self-canonical routes', () => {
  it.each(COVERED_CASES)('declares the page canonical for %s', async (target, pageMetadata) => {
    const pageCanonical = canonicalOf(await pageMetadata())

    expect(pageCanonical).toBe(`${SITE_URL}${pathOf(target)}`)
    expect(await headerCanonicalFor(target)).toBe(pageCanonical)
  })

  it('samples every self-canonical route through exactly its own rule', () => {
    expect(Object.values(COVERED).map(family => family.route).sort()).toEqual(
      SELF_CANONICAL_ROUTES.map(route => route.path).sort(),
    )
    for (const [dir, { route, cases }] of Object.entries(COVERED)) {
      expect(cases.length, `${dir} has no sample request`).toBeGreaterThan(0)
      for (const [target] of cases) {
        const matching = SELF_CANONICAL_ROUTES.filter(
          candidate => compileSource(candidate.source)(pathOf(target)) !== false,
        ).map(candidate => candidate.path)
        expect(matching, target).toEqual([route])
      }
    }
  })

  it('has one header rule per self-canonical route', async () => {
    const linkSources = (await loadHeaderRules())
      .filter(rule => rule.headers.some(header => header.key === 'Link'))
      .map(rule => rule.source)

    expect(linkSources).toEqual(SELF_CANONICAL_ROUTES.map(route => route.source))
  })
})

/** A proxy-shaped request, as the proxy tests build one. */
function proxyRequestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

describe('the proxy answers for every covered route', () => {
  afterEach(() => {
    clearExistenceVerdictsForTests()
  })

  // The proxy's `config.matcher` compiled and applied the way
  // `next/experimental/testing/server`'s `unstable_doesMiddlewareMatch` does,
  // for a plain navigation (no prefetch headers).
  // `getMiddlewareMatchers` is exported at runtime but missing from Next's
  // published declarations.
  const { getMiddlewareMatchers } = pageStaticInfo as unknown as {
    getMiddlewareMatchers: (matcher: unknown, nextConfig: object) => ProxyMatcher[]
  }
  const routesThroughProxy = getMiddlewareRouteMatcher(
    getMiddlewareMatchers(proxyConfig.matcher, {}),
  )

  it.each(COVERED_CASES)('routes %s through the proxy', target => {
    const url = new URL(target, 'http://localhost:3000')
    const request = { headers: {}, cookies: {} } as unknown as Parameters<typeof routesThroughProxy>[1]

    expect(
      routesThroughProxy(url.pathname, request, Object.fromEntries(url.searchParams)),
    ).toBe(true)
  })

  // When the backend reports the entity missing, the proxy's 404 lands before
  // anything streams; without it the page would serve a not-found body at
  // HTTP 200 under a header that calls that URL canonical.
  const entityFamilies = Object.entries(COVERED).filter(([, family]) => !family.calendar)

  it.each(entityFamilies)('404s a missing entity on %s', async (_dir, { cases }) => {
    fetchMock.mockImplementation(
      async () =>
        new Response(null, { status: 404, headers: { 'content-type': 'application/problem+json' } }),
    )
    for (const [target] of cases) {
      clearExistenceVerdictsForTests()
      expect((await proxy(proxyRequestFor(target))).status, target).toBe(404)
    }
  })
})

describe('no canonical Link header where the page declares a different URL', () => {
  it('leaves a numeric show URL alone, whose page canonical is the slug URL', async () => {
    backendAnswers({ slug: 'dev-lemons-2026-02-13' })
    const pageCanonical = canonicalOf(await showMetadata(params({ slug: '2044' })))

    expect(pageCanonical).toBe(`${SITE_URL}/shows/dev-lemons-2026-02-13`)
    expect(await headerCanonicalFor('/shows/2044')).toBeNull()
  })

  it('leaves a radio station alone, whose page canonical can be its channel URL', async () => {
    backendAnswers({
      slug: 'wfmu-drummer',
      network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
    })
    const pageCanonical = canonicalOf(
      await radioStationMetadata(params({ 'station-slug': 'wfmu-drummer' })),
    )

    expect(pageCanonical).toBe(`${SITE_URL}/radio/wfmu/channel/drummer`)
    expect(await headerCanonicalFor('/radio/wfmu-drummer')).toBeNull()
  })

  it('leaves scene windows alone, whose page canonical names the slug the backend returns', async () => {
    backendAnswers({
      slug: 'phoenix-az',
      date: '2026-11-19',
      iso_week: '2026-W47',
      prev_date: '2026-11-18',
      next_date: '2026-11-20',
      is_tonight: true,
    })
    const pageCanonical = canonicalOf(await sceneTonightMetadata(params({ slug: 'mesa-az' })))

    expect(pageCanonical).toBe(`${SITE_URL}/scenes/phoenix-az/tonight`)
    expect(await headerCanonicalFor('/scenes/mesa-az/tonight')).toBeNull()
  })

  const day = params({ slug: '2026', month: '11', day: '14' })

  // Next's router keeps the header for an empty `days=` and Vercel's withholds
  // it; the page names the day root either way, so neither disagrees with it.
  it('keeps an empty days= consistent with its page under either router', async () => {
    const meta = await showsDayMetadata({ ...day, searchParams: Promise.resolve({ days: '' }) })
    const header = await headerCanonicalFor('/shows/2026/11/14?days=')

    expect(canonicalOf(meta)).toBe(`${SITE_URL}/shows/2026/11/14`)
    expect(header === null || header === canonicalOf(meta)).toBe(true)
  })

  it('leaves a day run alone, which is noindexed and canonical to the day root', async () => {
    const meta = await showsDayMetadata({ ...day, searchParams: Promise.resolve({ days: '3' }) })

    expect(meta.robots).toMatchObject({ index: false })
    expect(await headerCanonicalFor('/shows/2026/11/14?days=3')).toBeNull()
  })

  it.each(['15', '0', '03', 'abc', ['3', '4']])(
    'leaves a refused run (days=%s) alone, whose page declares no canonical',
    async days => {
      const meta = await showsDayMetadata({ ...day, searchParams: Promise.resolve({ days }) })
      const search = (Array.isArray(days) ? days : [days]).map(value => `days=${value}`).join('&')

      expect(canonicalOf(meta)).toBeUndefined()
      expect(await headerCanonicalFor(`/shows/2026/11/14?${search}`)).toBeNull()
    },
  )

  it.each([
    '/shows/saved',
    '/shows/2026',
    '/shows/1999/11',
    '/shows/2026/13',
    '/shows/2026/11/32',
    '/shows/2026/1/14',
    '/artists/42',
    '/artists/espa%C3%B1ola',
    '/venues/thalia-hall-chicago-il/shows/0999',
    '/venues/thalia-hall-chicago-il/shows/25',
    '/scenes/chicago-il',
    '/scenes/mesa-az/2026-07-31',
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

describe('route inventory', () => {
  const pageDirs = pageRouteDirs()

  it('places every dynamic page route', () => {
    expect(pageDirs.filter(dir => dir.includes('[')).sort()).toEqual(
      [...Object.keys(COVERED), ...Object.keys(EXCLUDED)].sort(),
    )
  })

  // Any other page, fixed-path or dynamic, reached with a plausible slug in each
  // dynamic segment. A match means a rule claims a page it was not written for:
  // narrow the rule (as `/shows/:slug` excludes `submit`), or cover the page.
  it('sends no canonical Link header to a page route that is not covered', async () => {
    for (const dir of pageDirs.filter(candidate => !(candidate in COVERED))) {
      const pathname = `/${dir.replace(/\[[^\]]+\]/g, 'sample-slug')}`
      expect(await headerCanonicalFor(pathname), `${dir} (${pathname})`).toBeNull()
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
