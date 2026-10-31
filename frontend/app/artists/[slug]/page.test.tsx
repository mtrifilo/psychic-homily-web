import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import type { ReactElement } from 'react'
import { renderToString } from 'react-dom/server'
import type { Root } from 'react-dom/client'
import { act, render, waitFor } from '@testing-library/react'
import {
  HydrationBoundary,
  QueryClient,
  QueryClientProvider,
  type DehydratedState,
} from '@tanstack/react-query'
import { okResponse, errorResponse, jsonBodyResponse } from '@/lib/seo/test-helpers'
import { findElements } from '@/test/elementTree'
import {
  ANONYMOUS_VIEWER,
  SAVE_CONTROL_VIEWERS,
  hydrateCollectingErrors,
  type ViewerState,
} from '@/test/saveControlHydration'

// `notFound()` in Next.js throws to halt rendering; mirrored so a test can tell
// that execution stopped.
const NOT_FOUND_SENTINEL = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND_SENTINEL)
  }),
  // Read by every upcoming row's Save control (its sign-in gate).
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => '/artists/headliner-band',
}))

// The viewer the list renders for. Each state is one the route's HTML can be
// served under, and the hydration render below reads the same one.
let viewer: ViewerState = ANONYMOUS_VIEWER
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(viewer) }
})

// The save-count batch answers in the browser after hydration, so the route's
// HTML and the first client render both see it pending.
vi.mock('@/features/shows/hooks/useSavedShows', async importOriginal => ({
  ...(await importOriginal<
    typeof import('@/features/shows/hooks/useSavedShows')
  >()),
  useShowSaveCountBatch: () => ({ data: undefined }),
}))

// `seedFirstScreen` awaits `connection()`, which marks a request-time render and
// has nothing to do outside Next.
vi.mock('next/server', () => ({
  connection: async () => {},
}))

// On the server `getQueryClient()` mints a fresh client per call, so each seed
// helper dehydrates only its own keys. Under jsdom it would hand back the
// BROWSER singleton instead, so the server behaviour is restored here.
vi.mock('@/lib/queryClient', async importOriginal => {
  const actual = await importOriginal<typeof import('@/lib/queryClient')>()
  const { QueryClient } = await import('@tanstack/react-query')
  return { ...actual, getQueryClient: () => new QueryClient() }
})


// Stub the heavy artists feature module so invoking generateMetadata doesn't
// pull in the real ArtistDetail render path.
vi.mock('@/features/artists', () => ({
  ArtistDetail: (): null => null,
}))

// The past archive owns its own requests and its own loader. Stubbed so the
// server renders below speak only for the upcoming section this route seeds.
vi.mock('@/features/artists/components/ArtistPastShows', () => ({
  ArtistPastShows: (): null => null,
}))

// Spied rather than replaced, so the page's server-fetch failure reports can be
// read back.
vi.mock('@sentry/nextjs', async importOriginal => ({
  ...(await importOriginal<typeof import('@sentry/nextjs')>()),
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

import * as Sentry from '@sentry/nextjs'
import ArtistPage, { generateMetadata } from './page'
import { ArtistShowsList } from '@/features/artists/components/ArtistShowsList'
import { ArtistDetail } from '@/features/artists/components/ArtistDetail'
import { ArtistSimilarSidebar } from '@/features/artists/components/RelatedArtists'
import { CreateCollectionDrawerProvider } from '@/features/collections/components/CreateCollectionDrawer'
import { useArtistReleases } from '@/features/releases/hooks/useReleases'
import { resetServerFetchReportThrottleForTests } from '@/lib/ssr/reportServerFetchFailure'

// Each test starts with every route's report window open.
beforeEach(() => {
  resetServerFetchReportThrottleForTests()
})

function buildArtist(overrides: Record<string, unknown> = {}) {
  return {
    name: 'Headliner Band',
    slug: 'headliner-band',
    city: 'Phoenix',
    state: 'AZ',
    social: {},
    ...overrides,
  }
}

const fetchMock = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('artists/[slug] generateMetadata', () => {
  it('puts the location in the title when the artist has one', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.title).toBe('Headliner Band · Phoenix, AZ')
  })

  it('puts the location in the description when the artist has one', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.description).toBe(
      'Headliner Band from Phoenix, AZ: shows, similar artists and connections on Psychic Homily'
    )
  })

  it('uses the bare name and no from clause when the artist has no location', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildArtist({ name: 'Cape Fury', slug: 'cape-fury', city: null, state: null }))
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'cape-fury' }) })

    expect(meta.title).toBe('Cape Fury')
    expect(meta.description).toBe(
      'Cape Fury: shows, similar artists and connections on Psychic Homily'
    )
  })

  it('makes no request beyond the artist read', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('sets the canonical URL to https://psychichomily.com/artists/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/artists/headliner-band'
    )
  })

  it('mirrors the title and description into openGraph and sets url/type', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    expect(meta.openGraph?.title).toBe(meta.title)
    expect(meta.openGraph?.description).toBe(meta.description)
    expect(meta.openGraph?.url).toBe('/artists/headliner-band')
    expect((meta.openGraph as { type?: string })?.type).toBe('website')
  })

  it('falls back to the "Artist" title when the artist is missing', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.title).toBe('Artist')
    expect(meta.description).toBe('View artist details and upcoming shows')
    // No canonical alternate on the fallback shape.
    expect(meta.alternates).toBeUndefined()
    expect(meta.openGraph).toBeUndefined()
  })
})

// ── Page body: the upcoming-shows seed ────────────────────────────────────

const ARTIST_ID = 42

function artistShow(overrides: Record<string, unknown> = {}) {
  return {
    id: 20,
    slug: '2026-11-13-seeded-show',
    title: 'Seeded Show',
    event_date: '2099-11-13T03:00:00Z',
    price: null,
    door_price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: false,
    venue: {
      id: 1,
      slug: 'the-rebel-lounge',
      name: 'The Rebel Lounge',
      city: 'Phoenix',
      state: 'AZ',
      timezone: 'America/Phoenix',
    },
    artists: [
      { id: ARTIST_ID, slug: 'headliner-band', name: 'Headliner Band' },
      { id: 7, slug: 'support-act', name: 'Support Act' },
    ],
    ...overrides,
  }
}

function upcomingBody(shows: unknown[], artistId = ARTIST_ID) {
  return { shows, artist_id: artistId, total: shows.length, limit: 200, offset: 0, year: 0 }
}

function artistRelease(overrides: Record<string, unknown> = {}) {
  return {
    id: 300,
    title: 'Seeded LP',
    slug: 'seeded-lp',
    release_type: 'lp',
    release_year: 2024,
    cover_art_url: null,
    artist_count: 1,
    artists: [{ id: ARTIST_ID, slug: 'headliner-band', name: 'Headliner Band' }],
    label_name: 'Seeded Label',
    label_slug: 'seeded-label',
    role: 'main',
    ...overrides,
  }
}

function releasesBody(releases: unknown[]) {
  return { releases, count: releases.length }
}

function graphNode(id: number, name: string, slug: string) {
  return { id, name, slug, upcoming_show_count: 0 }
}

function graphBody(
  links: unknown[] = [],
  nodes: unknown[] = [],
  centerId = ARTIST_ID,
) {
  return {
    center: graphNode(centerId, 'Headliner Band', 'headliner-band'),
    nodes,
    links,
    user_votes: {},
  }
}

const isReleasesRead = (url: string) => /\/artists\/[^/]+\/releases(\?|$)/.test(url)
const isShowsRead = (url: string) => url.includes('/shows?')
const isGraphRead = (url: string) => /\/artists\/[^/]+\/graph(\?|$)/.test(url)

/** Route the page's four reads by URL; returns every URL requested. */
function stubBackend(
  shows: () => Response,
  artist: () => Response = () => okResponse(buildArtist({ id: ARTIST_ID })),
  releases: () => Response = () => jsonBodyResponse(releasesBody([])),
  graph: () => Response = () => jsonBodyResponse(graphBody()),
) {
  const calls: string[] = []
  fetchMock.mockImplementation(async (url: string) => {
    calls.push(String(url))
    if (isReleasesRead(String(url))) return releases()
    if (isGraphRead(String(url))) return graph()
    return isShowsRead(String(url)) ? shows() : artist()
  })
  return calls
}

const reply = (body: unknown, status = 200) => () =>
  jsonBodyResponse(body, status)

async function renderPage(slug = 'headliner-band') {
  return (await ArtistPage({ params: Promise.resolve({ slug }) })) as ReactElement
}

/** Every dehydrated state the page hands a `HydrationBoundary`, outermost first. */
function boundaryStatesOf(page: ReactElement): DehydratedState[] {
  return findElements(page, node => node.type === HydrationBoundary).map(
    node => (node.props as { state: DehydratedState }).state,
  )
}

/** Every query the page seeds, across all of its boundaries. */
function seededQueries(page: ReactElement) {
  return boundaryStatesOf(page).flatMap(state => state.queries)
}

function seededShowsQuery(page: ReactElement) {
  return seededQueries(page).find(
    q => q.queryKey[0] === 'artists' && q.queryKey[1] === 'shows',
  )
}

/** `child` under the page's own boundaries, nested as the page nests them. */
function underPageBoundaries(
  states: DehydratedState[],
  client: QueryClient,
  child: ReactElement,
) {
  return (
    <QueryClientProvider client={client}>
      {states.reduceRight<ReactElement>(
        (children, state) => <HydrationBoundary state={state}>{children}</HydrationBoundary>,
        child,
      )}
    </QueryClientProvider>
  )
}

/** The real upcoming list under the page's own boundaries. */
function upcomingListUnder(states: DehydratedState[], client: QueryClient) {
  return underPageBoundaries(
    states,
    client,
    <ArtistShowsList
      artistId={ARTIST_ID}
      artistSlug="headliner-band"
      artistName="Headliner Band"
    />,
  )
}

const testClient = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false } } })

/**
 * Server-render the list the way the route's HTML is produced: no client fetch
 * can run inside `renderToString`, so whatever this prints came from the seed.
 */
function serverRenderUpcoming(page: ReactElement): string {
  return renderToString(upcomingListUnder(boundaryStatesOf(page), testClient()))
}

const LOADER = 'animate-spin'

/** A request's query parameters, order-independent. */
const paramsOf = (url: string) =>
  [...new URL(url).searchParams.entries()].sort(([a], [b]) => a.localeCompare(b))

describe('ArtistPage upcoming shows', () => {
  it('asks for the upcoming request by encoded slug, alongside the artist', async () => {
    const calls = stubBackend(reply(upcomingBody([artistShow()])))

    await renderPage('weird/slug?x')

    const showsUrl = calls.find(isShowsRead)
    expect(showsUrl).toBeDefined()
    const url = new URL(showsUrl!)
    expect(url.pathname).toMatch(/\/artists\/weird%2Fslug%3Fx\/shows$/)
    expect(url.searchParams.get('limit')).toBe('200')
    expect(url.searchParams.get('time_filter')).toBe('upcoming')
    expect(calls).toHaveLength(4)
  })

  it('asks for a zero-padded numeric id by its unpadded form', async () => {
    const calls = stubBackend(reply(upcomingBody([artistShow()])))

    await renderPage('0042')

    const url = new URL(calls.find(isShowsRead)!)
    expect(url.pathname).toMatch(/\/artists\/42\/shows$/)
  })

  // Pins the SEED's key spelling as a literal, so it cannot agree with a
  // shared helper while both drift. That the list hook reads this entry is
  // proven by the server-render test below, which runs the real hook.
  it('seeds the rows under the key the list hook registers', async () => {
    const body = upcomingBody([artistShow()])
    stubBackend(reply(body))

    const seeded = seededShowsQuery(await renderPage())

    expect(seeded?.queryKey).toEqual([
      'artists',
      'shows',
      String(ARTIST_ID),
      'upcoming',
      200,
      null,
      null,
    ])
    expect(seeded?.state.data).toEqual(body)
  })

  it('stamps the seeded rows stale, so the browser revalidates them on mount', async () => {
    stubBackend(reply(upcomingBody([artistShow()])))

    const page = await renderPage()

    expect(seededShowsQuery(page)?.state.dataUpdatedAt).toBe(0)
    // The detail keeps its fresh stamp: only the list is re-asked.
    const detail = seededQueries(page).find(q => q.queryKey[1] === 'detail')
    expect(detail?.state.dataUpdatedAt).toBeGreaterThan(0)
  })

  it('puts the upcoming rows in the server render, with no loader', async () => {
    stubBackend(reply(upcomingBody([artistShow()])))

    const html = serverRenderUpcoming(await renderPage())

    expect(html).toContain('href="/shows/2026-11-13-seeded-show"')
    expect(html).toContain('Support Act')
    expect(html).not.toContain(LOADER)
  })

  it('replaces the seeded rows with the live read once mounted', async () => {
    const serverCalls = stubBackend(reply(upcomingBody([artistShow()])))
    const states = boundaryStatesOf(await renderPage())
    // The browser's own read answers with a show the seed did not have.
    const browserCalls = stubBackend(
      reply(
        upcomingBody([artistShow({ id: 21, slug: 'late-addition' })]),
      ),
    )

    const { container } = render(upcomingListUnder(states, testClient()))

    expect(container.innerHTML).toContain('href="/shows/2026-11-13-seeded-show"')
    await waitFor(() =>
      expect(container.innerHTML).toContain('href="/shows/late-addition"'),
    )
    // The server asked the question the hook asks.
    expect(paramsOf(browserCalls[0])).toEqual(
      paramsOf(serverCalls.find(isShowsRead)!),
    )
  })

  it('drops rows that have left the upcoming window since the read', async () => {
    const future = artistShow()
    stubBackend(
      reply({
        ...upcomingBody([
          artistShow({ id: 19, slug: 'long-gone', event_date: '2020-01-01T03:00:00Z' }),
          future,
        ]),
        total: 5,
      }),
    )

    const seeded = seededShowsQuery(await renderPage())

    expect(seeded?.state.data).toMatchObject({ shows: [future], total: 4 })
  })

  it('server-renders the empty state for an artist with no upcoming shows', async () => {
    stubBackend(reply(upcomingBody([])))

    const html = serverRenderUpcoming(await renderPage())

    expect(html).toContain('No upcoming shows yet.')
    expect(html).not.toContain(LOADER)
  })

  it('still renders the page, with the loader, when the shows read fails', async () => {
    stubBackend(reply({}, 500))

    const page = await renderPage()

    expect(seededQueries(page).map(q => q.queryKey.slice(0, 2).join('/'))).toEqual([
      'artists/detail',
      'releases/artist',
      'artists/graph',
    ])
    const html = serverRenderUpcoming(page)
    expect(html).toContain(LOADER)
    expect(html).not.toContain('No upcoming shows yet.')
  })

  it('withholds rows that answer for a different artist', async () => {
    stubBackend(reply(upcomingBody([artistShow()], ARTIST_ID + 1)))

    expect(seededShowsQuery(await renderPage())).toBeUndefined()
  })

  it('404s an unknown artist', async () => {
    stubBackend(reply({}, 404), () => errorResponse(404))

    await expect(renderPage('missing')).rejects.toThrow(NOT_FOUND_SENTINEL)
  })
})

// ── Page body: the discography seed ───────────────────────────────────────

/** Stub the page's reads with no upcoming shows and the given discography answer. */
const stubReleases = (releases: () => Response) =>
  stubBackend(reply(upcomingBody([])), undefined, releases)

/** Every URL the page asked the releases endpoint for. */
const releasesCalls = (calls: string[]) => calls.filter(isReleasesRead)

function seededReleasesQuery(page: ReactElement) {
  return seededQueries(page).find(q => q.queryKey[0] === 'releases')
}

/**
 * Server-render the real `ArtistDetail` under the page's own boundaries, the
 * way the route's HTML is produced: no client fetch can run inside
 * `renderToString`, so any discography row it prints came from the seed.
 */
function serverRenderArtistDetail(page: ReactElement): string {
  return renderToString(
    underPageBoundaries(
      boundaryStatesOf(page),
      testClient(),
      <CreateCollectionDrawerProvider>
        <ArtistDetail artistId="headliner-band" />
      </CreateCollectionDrawerProvider>,
    ),
  )
}

// The discography's loader markup. Another `ArtistDetail` section rendering a
// spinner with the same classes would fail the "no loader" assertions below;
// the failed-read test shows this string is what the discography renders.
const DISCOGRAPHY_LOADER = 'h-6 w-6 animate-spin'
const DISCOGRAPHY_TABLE = 'aria-label="Discography"'

/** Renders the real hook's rows as links, so a test can watch its data change. */
function DiscographyProbe({ slug }: { slug: string }) {
  const { data } = useArtistReleases({ artistIdOrSlug: slug })
  return (
    <ul>
      {data?.releases.map(r => (
        <li key={r.id}>
          <a href={`/releases/${r.slug}`}>{r.title}</a>
        </li>
      ))}
    </ul>
  )
}

describe('ArtistPage discography', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('asks for the discography by encoded slug with no parameters, alongside the artist', async () => {
    const calls = stubBackend(reply(upcomingBody([])))

    await renderPage('weird/slug?x')

    expect(releasesCalls(calls)).toHaveLength(1)
    const url = new URL(releasesCalls(calls)[0])
    expect(url.pathname).toMatch(/\/artists\/weird%2Fslug%3Fx\/releases$/)
    expect(url.search).toBe('')
  })

  it('asks for a zero-padded numeric id by its unpadded form', async () => {
    const calls = stubBackend(reply(upcomingBody([])))

    await renderPage('0042')

    expect(new URL(releasesCalls(calls)[0]).pathname).toMatch(/\/artists\/42\/releases$/)
  })

  it('sends the server-read secret on the discography read', async () => {
    vi.stubEnv('SERVER_READ_SECRET', 'artist-page-secret-0123456789abcdef0123')
    stubBackend(reply(upcomingBody([])))

    await renderPage()

    const call = fetchMock.mock.calls.find(([url]) => isReleasesRead(String(url)))
    expect((call?.[1] as RequestInit).headers).toEqual({
      'X-Server-Read-Secret': 'artist-page-secret-0123456789abcdef0123',
    })
  })

  // Pins the SEED's key spelling as a literal, so it cannot agree with a
  // shared helper while both drift. That `ArtistDetail`'s discography reads
  // this entry is proven by the server-render test below, which runs the real
  // component and hook.
  it('seeds the discography under the key the hook registers for the route slug', async () => {
    const body = releasesBody([artistRelease()])
    stubReleases(reply(body))

    const seeded = seededReleasesQuery(await renderPage())

    expect(seeded?.queryKey).toEqual(['releases', 'artist', 'headliner-band'])
    expect(seeded?.state.data).toEqual(body)
  })

  it('stamps the seeded discography stale, so the browser revalidates it on mount', async () => {
    stubReleases(reply(releasesBody([artistRelease()])))

    expect(seededReleasesQuery(await renderPage())?.state.dataUpdatedAt).toBe(0)
  })

  it('puts the discography rows and their release links in the server render, with no loader', async () => {
    stubReleases(
      reply(
        releasesBody([
          artistRelease(),
          artistRelease({ id: 301, slug: 'guest-spot', title: 'Guest Spot', role: 'featured' }),
        ]),
      ),
    )

    const html = serverRenderArtistDetail(await renderPage())

    expect(html).toContain(DISCOGRAPHY_TABLE)
    expect(html).toContain('href="/releases/seeded-lp"')
    expect(html).toContain('href="/releases/guest-spot"')
    expect(html).toContain('href="/labels/seeded-label"')
    expect(html).toContain('Guest Appearances')
    expect(html).not.toContain(DISCOGRAPHY_LOADER)
  })

  it('server-renders no discography section and no loader for an artist with no releases', async () => {
    stubReleases(reply(releasesBody([])))

    const page = await renderPage()
    const html = serverRenderArtistDetail(page)

    expect(seededReleasesQuery(page)?.state.data).toEqual(releasesBody([]))
    expect(html).not.toContain(DISCOGRAPHY_TABLE)
    expect(html).not.toContain(DISCOGRAPHY_LOADER)
  })

  it('still renders the page, with the discography loader, when the read fails', async () => {
    stubReleases(reply({}, 500))

    const page = await renderPage()
    const html = serverRenderArtistDetail(page)

    expect(seededReleasesQuery(page)).toBeUndefined()
    // The upcoming seed does not depend on the discography read.
    expect(seededShowsQuery(page)).toBeDefined()
    expect(html).toContain(DISCOGRAPHY_LOADER)
    expect(html).not.toContain(DISCOGRAPHY_TABLE)
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'artist-page-releases: API returned 500',
      expect.objectContaining({
        tags: {
          service: 'artist-page-releases',
          path_family: '/artists/{slug}/releases',
          status: '500',
        },
      }),
    )
  })

  it('seeds nothing for a payload with no releases array', async () => {
    stubReleases(reply({ count: 0 }))

    expect(seededReleasesQuery(await renderPage())).toBeUndefined()
  })

  it('treats a 404 from the discography read as no releases, and does not report it', async () => {
    stubReleases(reply({}, 404))

    const page = await renderPage()
    const html = serverRenderArtistDetail(page)

    expect(seededReleasesQuery(page)?.state.data).toEqual(releasesBody([]))
    expect(html).not.toContain(DISCOGRAPHY_TABLE)
    expect(html).not.toContain(DISCOGRAPHY_LOADER)
    expect(Sentry.captureMessage).not.toHaveBeenCalled()
  })

  it('replaces the seeded discography with the live read once mounted', async () => {
    const serverCalls = stubReleases(reply(releasesBody([artistRelease()])))
    const states = boundaryStatesOf(await renderPage())
    // The browser's own read answers with a release the seed did not have.
    const browserCalls = stubReleases(
      reply(
        releasesBody([artistRelease({ id: 302, slug: 'late-addition', title: 'Late Addition' })]),
      ),
    )

    const { container } = render(
      underPageBoundaries(states, testClient(), <DiscographyProbe slug="headliner-band" />),
    )

    expect(container.innerHTML).toContain('href="/releases/seeded-lp"')
    await waitFor(() =>
      expect(container.innerHTML).toContain('href="/releases/late-addition"'),
    )
    // The server asked the question the hook asks.
    expect(releasesCalls(browserCalls)[0]).toBe(releasesCalls(serverCalls)[0])
  })
})

// ── Page body: the Similar artists seed ───────────────────────────────────

const KINDRED = graphNode(51, 'Kindred Spirits', 'kindred-spirits')
const BILL_MATE = graphNode(52, 'Bill Mates', 'bill-mates')

/** Two neighbours with one reason each: a similarity score, and shared bills. */
function richGraphBody(centerId = ARTIST_ID) {
  return graphBody(
    [
      { source_id: centerId, target_id: KINDRED.id, type: 'similar', score: 0.85, votes_up: 3, votes_down: 0 },
      {
        source_id: centerId,
        target_id: BILL_MATE.id,
        type: 'shared_bills',
        score: 0.4,
        votes_up: 0,
        votes_down: 0,
        detail: { shared_count: 4 },
      },
    ],
    [KINDRED, BILL_MATE],
    centerId,
  )
}

/** Stub the page's reads with no upcoming shows and the given graph answer. */
const stubGraph = (graph: () => Response) =>
  stubBackend(reply(upcomingBody([])), undefined, undefined, graph)

function seededGraphQuery(page: ReactElement) {
  return seededQueries(page).find(
    q => q.queryKey[0] === 'artists' && q.queryKey[1] === 'graph',
  )
}

/** The markup of the page's sidebar, which holds the Similar artists list. */
function asideOf(html: string): string {
  const match = html.match(/<aside data-testid="artist-sidebar"[\s\S]*?<\/aside>/)
  expect(match).not.toBeNull()
  return match![0]
}

const occurrences = (html: string, text: string) => html.split(text).length - 1

describe('ArtistPage similar artists', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('asks for the graph by the numeric id, with no parameters, once the artist resolves', async () => {
    const calls = stubGraph(reply(richGraphBody()))

    await renderPage('0042')

    const graphCalls = calls.filter(isGraphRead)
    expect(graphCalls).toHaveLength(1)
    const url = new URL(graphCalls[0])
    expect(url.pathname).toMatch(/\/artists\/42\/graph$/)
    expect(url.search).toBe('')
  })

  // Pins the SEED's key spelling as a literal, so it cannot agree with a
  // shared helper while both drift. That the list reads this entry is proven
  // by the server-render test below, which runs the real component and hook.
  it('seeds the graph under the key the list hook registers', async () => {
    const body = richGraphBody()
    stubGraph(reply(body))

    const seeded = seededGraphQuery(await renderPage())

    expect(seeded?.queryKey).toEqual(['artists', 'graph', String(ARTIST_ID), undefined])
    expect(seeded?.state.data).toEqual(body)
  })

  it('stamps the seeded graph stale, so the browser revalidates it on mount', async () => {
    stubGraph(reply(richGraphBody()))

    expect(seededGraphQuery(await renderPage())?.state.dataUpdatedAt).toBe(0)
  })

  it('puts the similar artists, their links and their reasons in the server-rendered sidebar', async () => {
    stubGraph(reply(richGraphBody()))

    const html = serverRenderArtistDetail(await renderPage())
    const aside = asideOf(html)

    expect(aside).toContain('Similar artists')
    for (const node of [KINDRED, BILL_MATE]) {
      expect(aside).toContain(`href="/artists/${node.slug}"`)
      expect(aside).toContain(node.name)
      // One node for every viewport: the name is nowhere else in the page.
      expect(occurrences(html, node.name)).toBe(1)
    }
    expect(aside).toContain('85% similar')
    expect(aside).toContain('4 shared shows')
    expect(aside).toContain('Shared Bills')
  })

  it('still renders the page, without the list, when the graph read fails', async () => {
    stubGraph(reply({}, 500))

    const page = await renderPage()
    const html = serverRenderArtistDetail(page)

    expect(seededGraphQuery(page)).toBeUndefined()
    // The other seeds do not depend on the graph read.
    expect(seededShowsQuery(page)).toBeDefined()
    expect(seededReleasesQuery(page)).toBeDefined()
    expect(asideOf(html)).not.toContain('Similar artists')
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'artist-page-similar-artists: API returned 500',
      expect.objectContaining({
        tags: {
          service: 'artist-page-similar-artists',
          path_family: '/artists/{id}/graph',
          status: '500',
        },
      }),
    )
  })

  it('withholds a graph centred on a different artist', async () => {
    stubGraph(reply(richGraphBody(ARTIST_ID + 1)))

    expect(seededGraphQuery(await renderPage())).toBeUndefined()
  })

  it('withholds and reports a graph with no links array', async () => {
    const noLinks: Record<string, unknown> = richGraphBody()
    delete noLinks.links
    stubGraph(reply(noLinks))

    expect(seededGraphQuery(await renderPage())).toBeUndefined()
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'artist-page-similar-artists: response has no "links" array',
      expect.objectContaining({ tags: { service: 'artist-page-similar-artists' } }),
    )
  })

  // An artist with no relationships seeds its empty graph, and the anonymous
  // server HTML carries no Similar artists section at all.
  it('server-renders no Similar artists section for an artist with no relationships', async () => {
    stubGraph(reply(graphBody()))

    const page = await renderPage()
    const html = serverRenderArtistDetail(page)

    expect(seededGraphQuery(page)?.state.data).toEqual(graphBody())
    expect(html).not.toContain('Similar artists')
  })

  it('sends the server-read secret on the graph read', async () => {
    vi.stubEnv('SERVER_READ_SECRET', 'artist-page-secret-0123456789abcdef0123')
    stubGraph(reply(richGraphBody()))

    await renderPage()

    const call = fetchMock.mock.calls.find(([url]) => isGraphRead(String(url)))
    expect((call?.[1] as RequestInit).headers).toEqual({
      'X-Server-Read-Secret': 'artist-page-secret-0123456789abcdef0123',
    })
  })

  it('replaces the seeded list with the live read once mounted', async () => {
    const serverCalls = stubGraph(reply(richGraphBody()))
    const states = boundaryStatesOf(await renderPage())
    const lateNode = graphNode(53, 'Late Addition', 'late-addition')
    // The browser's own read answers with a neighbour the seed did not have.
    const browserCalls = stubGraph(
      reply(
        graphBody(
          [{ source_id: ARTIST_ID, target_id: lateNode.id, type: 'similar', score: 0.5, votes_up: 0, votes_down: 0 }],
          [lateNode],
        ),
      ),
    )

    const { container } = render(
      underPageBoundaries(
        states,
        testClient(),
        <ArtistSimilarSidebar artistId={ARTIST_ID} artistSlug="headliner-band" onOpenGraph={() => {}} />,
      ),
    )

    expect(container.innerHTML).toContain('href="/artists/kindred-spirits"')
    await waitFor(() =>
      expect(container.innerHTML).toContain('href="/artists/late-addition"'),
    )
    // The server asked the question the hook asks.
    expect(browserCalls.filter(isGraphRead)[0]).toBe(serverCalls.filter(isGraphRead)[0])
  })
})

describe('ArtistPage upcoming Save column', () => {
  let root: Root | null = null
  let container: HTMLElement | null = null

  afterEach(() => {
    act(() => root?.unmount())
    container?.remove()
    root = null
    container = null
    viewer = ANONYMOUS_VIEWER
  })

  it.each(SAVE_CONTROL_VIEWERS)(
    'serves the control in the HTML and hydrates it unchanged (%s viewer)',
    async (_name, state, accessibleName, disabled) => {
      viewer = state
      stubBackend(reply(upcomingBody([artistShow()])))
      const states = boundaryStatesOf(await renderPage())

      const html = renderToString(upcomingListUnder(states, testClient()))
      container = document.createElement('div')
      container.innerHTML = html
      document.body.appendChild(container)
      const control = container.querySelector(
        'table[aria-label="Upcoming shows"] tbody tr td:last-child button'
      )
      expect(control?.getAttribute('aria-label')).toBe(accessibleName)
      expect(control?.textContent).toBe('Save')
      expect(control?.hasAttribute('disabled')).toBe(disabled)

      const hydrated = await hydrateCollectingErrors(
        container,
        upcomingListUnder(states, testClient())
      )
      root = hydrated.root
      expect(hydrated.errors).toEqual([])
    }
  )
})

describe('artists/[slug] server read', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('sends the server-read secret when one is configured', async () => {
    vi.stubEnv('SERVER_READ_SECRET', 'artist-page-secret-0123456789abcdef0123')
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.headers).toEqual({ 'X-Server-Read-Secret': 'artist-page-secret-0123456789abcdef0123' })
  })

  it('sends no secret header when none is configured', async () => {
    vi.stubEnv('SERVER_READ_SECRET', '')
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.headers).toEqual({})
  })

  it('reports a 429 and still renders the not-found page', async () => {
    fetchMock.mockImplementation(async () => errorResponse(429, { 'Retry-After': '60' }))

    await expect(
      ArtistPage({ params: Promise.resolve({ slug: 'rate-limited' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'artist-page: API returned 429',
      expect.objectContaining({
        level: 'error',
        tags: { service: 'artist-page', path_family: '/artists/{slug}', status: '429' },
        extra: {
          slug: 'rate-limited',
          pathFamily: '/artists/{slug}',
          status: 429,
          retryAfter: '60',
          suppressedSinceLastReport: 0,
        },
      })
    )
  })

  it('reports a 5xx', async () => {
    fetchMock.mockImplementation(async () => errorResponse(500))

    await generateMetadata({ params: Promise.resolve({ slug: 'boom' }) })

    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'artist-page: API returned 500',
      expect.objectContaining({ extra: expect.objectContaining({ status: 500 }) })
    )
  })

  it('stays silent on a 404', async () => {
    fetchMock.mockImplementation(async () => errorResponse(404))

    await expect(
      ArtistPage({ params: Promise.resolve({ slug: 'missing' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    // Neither the artist read nor its upcoming-shows read, which is scoped to
    // the same slug, reports a 404.
    expect(Sentry.captureMessage).not.toHaveBeenCalled()
  })
})
