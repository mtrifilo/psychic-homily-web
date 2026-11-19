import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { Metadata } from 'next'
import { PHASE_PRODUCTION_SERVER } from 'next/constants'
import { getPathMatch } from 'next/dist/shared/lib/router/utils/path-match'
import { compileNonPath } from 'next/dist/shared/lib/router/utils/prepare-destination'
import { modifyRouteRegex } from 'next/dist/lib/redirect-status'
import nextConfigForPhase, { SELF_CANONICAL_ROUTES } from './next.config'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error('NEXT_NOT_FOUND')
  }),
  redirect: vi.fn(),
  permanentRedirect: vi.fn(),
  usePathname: vi.fn(() => '/'),
  useRouter: vi.fn(() => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() })),
  useSearchParams: vi.fn(() => new URLSearchParams()),
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

/**
 * The response headers Next's router adds for `pathname` from the loaded
 * config's `headers()`: rules compiled with the options
 * `server/lib/router-utils/filesystem.js` uses, values substituted with
 * `compileNonPath` as `resolve-routes.js` does, later rules overwriting
 * earlier ones key by key.
 */
async function configHeadersFor(pathname: string): Promise<Record<string, string>> {
  const rules = (await nextConfigForPhase(PHASE_PRODUCTION_SERVER).headers!()) as HeaderRule[]
  const out: Record<string, string> = {}
  for (const rule of rules) {
    const match = getPathMatch(rule.source, {
      strict: true,
      removeUnnamedParams: true,
      regexModifier: (regex: string) => modifyRouteRegex(regex),
    })
    const params = match(pathname)
    if (params === false) continue
    const hasParams = Object.keys(params).length > 0
    for (const { key, value } of rule.headers) {
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

// Every backend read in the pages under test resolves to this record. It
// carries the fields each page's existence check and snippet builders read; the
// `slug` echoes the requested one, which is the backend's exact-match lookup.
function entityRecordFor(url: string): Record<string, unknown> {
  const lastSegment = decodeURIComponent(new URL(url).pathname.split('/').filter(Boolean).pop() ?? '')
  return {
    id: 7,
    slug: lastSegment,
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
  }
}

async function answerWithEntityRecord(input: RequestInfo | URL): Promise<Response> {
  const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
  return new Response(JSON.stringify(entityRecordFor(url)), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  })
}

const fetchMock = vi.fn(answerWithEntityRecord)

beforeEach(() => {
  fetchMock.mockImplementation(answerWithEntityRecord)
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const params = <T,>(value: T) => ({ params: Promise.resolve(value) })

/**
 * One sample request per covered family, with the canonical its page declares
 * for that request. Content-backed families run over every post they publish.
 */
const COVERED_CASES: Array<[pathname: string, pageCanonical: () => Promise<Metadata>]> = [
  ['/artists/the-hoyle-brothers', () => artistMetadata(params({ slug: 'the-hoyle-brothers' }))],
  ['/releases/a-long-way-from-kansas', () => releaseMetadata(params({ slug: 'a-long-way-from-kansas' }))],
  ['/venues/thalia-hall-chicago-il', () => venueMetadata(params({ slug: 'thalia-hall-chicago-il' }))],
  ['/labels/run-for-cover-records', () => labelMetadata(params({ slug: 'run-for-cover-records' }))],
  ['/festivals/levitation-2026', () => festivalMetadata(params({ slug: 'levitation-2026' }))],
  ['/tags/post-punk', () => tagMetadata(params({ slug: 'post-punk' }))],
  ['/collections/desert-noise', () => collectionMetadata(params({ slug: 'desert-noise' }))],
  ['/shows/dev-lemons-2026-02-13', () => showMetadata(params({ slug: 'dev-lemons-2026-02-13' }))],
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
  ...getBlogSlugs().map(
    (slug): [string, () => Promise<Metadata>] => [`/blog/${slug}`, () => blogMetadata(params({ slug }))],
  ),
  ...getMixSlugs().map(
    (slug): [string, () => Promise<Metadata>] => [`/dj-sets/${slug}`, () => mixMetadata(params({ slug }))],
  ),
]

describe('canonical Link header on self-canonical entity routes', () => {
  it.each(COVERED_CASES)('declares the page canonical for %s', async (pathname, pageMetadata) => {
    const pageCanonical = canonicalOf(await pageMetadata())

    expect(pageCanonical).toBe(`https://psychichomily.com${pathname}`)
    expect(await headerCanonicalFor(pathname)).toBe(pageCanonical)
  })

  it('declares the page canonical for every published category', async () => {
    const categorySlugs = getAllCategories().map(getCategorySlug)
    expect(categorySlugs.length).toBeGreaterThan(0)
    for (const category of categorySlugs) {
      const pageCanonical = canonicalOf(await categoryMetadata(params({ category })))
      expect(pageCanonical, category).toBe(`https://psychichomily.com/categories/${category}`)
      expect(await headerCanonicalFor(`/categories/${category}`), category).toBe(pageCanonical)
    }
  })

  it('ignores the query string, as the page canonical does', async () => {
    const pageCanonical = canonicalOf(await artistMetadata(params({ slug: 'the-hoyle-brothers' })))

    expect(await headerCanonicalFor('/artists/the-hoyle-brothers')).toBe(pageCanonical)
  })

  it('has one header rule per self-canonical route', async () => {
    const rules = (await nextConfigForPhase(PHASE_PRODUCTION_SERVER).headers!()) as HeaderRule[]
    const linkSources = rules
      .filter(rule => rule.headers.some(header => header.key === 'Link'))
      .map(rule => rule.source)

    expect(linkSources).toEqual(SELF_CANONICAL_ROUTES.map(route => route.source))
  })
})

describe('no canonical Link header where the page declares a different URL', () => {
  it('leaves a numeric show URL alone, whose page canonical is the slug URL', async () => {
    fetchMock.mockImplementationOnce(async () =>
      new Response(JSON.stringify({ ...entityRecordFor('https://x/shows/dev-lemons-2026-02-13') }), {
        status: 200,
      }),
    )
    const pageCanonical = canonicalOf(await showMetadata(params({ slug: '2044' })))

    expect(pageCanonical).toBe('https://psychichomily.com/shows/dev-lemons-2026-02-13')
    expect(await headerCanonicalFor('/shows/2044')).toBeNull()
  })

  it('leaves a radio station alone, whose page canonical can be its channel URL', async () => {
    fetchMock.mockImplementationOnce(async () =>
      new Response(
        JSON.stringify({
          ...entityRecordFor('https://x/radio-stations/wfmu-drummer'),
          network: { slug: 'wfmu', name: 'WFMU', is_flagship: false },
        }),
        { status: 200 },
      ),
    )
    const pageCanonical = canonicalOf(
      await radioStationMetadata(params({ 'station-slug': 'wfmu-drummer' })),
    )

    expect(pageCanonical).toBe('https://psychichomily.com/radio/wfmu/channel/drummer')
    expect(await headerCanonicalFor('/radio/wfmu-drummer')).toBeNull()
    expect(await headerCanonicalFor('/radio/kexp')).toBeNull()
  })

  it('leaves scene windows alone, whose page canonical names the slug the backend returns', async () => {
    fetchMock.mockImplementation(async () =>
      new Response(
        JSON.stringify({
          ...entityRecordFor('https://x/scenes/phoenix-az'),
          date: '2026-11-19',
          iso_week: '2026-W47',
          prev_date: '2026-11-18',
          next_date: '2026-11-20',
          is_tonight: true,
        }),
        { status: 200 },
      ),
    )
    const pageCanonical = canonicalOf(await sceneTonightMetadata(params({ slug: 'mesa-az' })))

    expect(pageCanonical).toBe('https://psychichomily.com/scenes/phoenix-az/tonight')
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
    '/shows/submit',
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

describe('htmlLimitedBots', () => {
  // Unset on purpose. Adding a script-running crawler such as Googlebot to it
  // sends that crawler past the cached shell on Vercel to a static-generation
  // render, which answers HTTP 500 on the entity routes; the canonical reaches
  // Google through the Link header instead.
  it('is not configured', () => {
    expect(nextConfigForPhase(PHASE_PRODUCTION_SERVER).htmlLimitedBots).toBeUndefined()
  })
})
