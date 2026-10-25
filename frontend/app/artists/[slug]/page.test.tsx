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

import ArtistPage, { generateMetadata } from './page'
import { ArtistShowsList } from '@/features/artists/components/ArtistShowsList'

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

/** Route the page's two reads by URL; returns every URL requested. */
function stubBackend(
  shows: () => Response,
  artist: () => Response = () => okResponse(buildArtist({ id: ARTIST_ID })),
) {
  const calls: string[] = []
  fetchMock.mockImplementation(async (url: string) => {
    calls.push(String(url))
    return String(url).includes('/shows?') ? shows() : artist()
  })
  return calls
}

const showsReply = (body: unknown, status = 200) => () =>
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

function seededShowsQuery(page: ReactElement) {
  return boundaryStatesOf(page)
    .flatMap(state => state.queries)
    .find(q => q.queryKey[0] === 'artists' && q.queryKey[1] === 'shows')
}

/** The real upcoming list under the page's own boundaries, nested as the page nests them. */
function upcomingListUnder(states: DehydratedState[], client: QueryClient) {
  const list = (
    <ArtistShowsList
      artistId={ARTIST_ID}
      artistSlug="headliner-band"
      artistName="Headliner Band"
    />
  )
  return (
    <QueryClientProvider client={client}>
      {states.reduceRight<ReactElement>(
        (children, state) => <HydrationBoundary state={state}>{children}</HydrationBoundary>,
        list,
      )}
    </QueryClientProvider>
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
    const calls = stubBackend(showsReply(upcomingBody([artistShow()])))

    await renderPage('weird/slug?x')

    const showsUrl = calls.find(u => u.includes('/shows?'))
    expect(showsUrl).toBeDefined()
    const url = new URL(showsUrl!)
    expect(url.pathname).toMatch(/\/artists\/weird%2Fslug%3Fx\/shows$/)
    expect(url.searchParams.get('limit')).toBe('200')
    expect(url.searchParams.get('time_filter')).toBe('upcoming')
    expect(calls).toHaveLength(2)
  })

  it('asks for a zero-padded numeric id by its unpadded form', async () => {
    const calls = stubBackend(showsReply(upcomingBody([artistShow()])))

    await renderPage('0042')

    const url = new URL(calls.find(u => u.includes('/shows?'))!)
    expect(url.pathname).toMatch(/\/artists\/42\/shows$/)
  })

  // Pins the SEED's key spelling as a literal, so it cannot agree with a
  // shared helper while both drift. That the list hook reads this entry is
  // proven by the server-render test below, which runs the real hook.
  it('seeds the rows under the key the list hook registers', async () => {
    const body = upcomingBody([artistShow()])
    stubBackend(showsReply(body))

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
    stubBackend(showsReply(upcomingBody([artistShow()])))

    const page = await renderPage()

    expect(seededShowsQuery(page)?.state.dataUpdatedAt).toBe(0)
    // The detail keeps its fresh stamp: only the list is re-asked.
    const detail = boundaryStatesOf(page)
      .flatMap(state => state.queries)
      .find(q => q.queryKey[1] === 'detail')
    expect(detail?.state.dataUpdatedAt).toBeGreaterThan(0)
  })

  it('puts the upcoming rows in the server render, with no loader', async () => {
    stubBackend(showsReply(upcomingBody([artistShow()])))

    const html = serverRenderUpcoming(await renderPage())

    expect(html).toContain('href="/shows/2026-11-13-seeded-show"')
    expect(html).toContain('Support Act')
    expect(html).not.toContain(LOADER)
  })

  it('replaces the seeded rows with the live read once mounted', async () => {
    const serverCalls = stubBackend(showsReply(upcomingBody([artistShow()])))
    const states = boundaryStatesOf(await renderPage())
    // The browser's own read answers with a show the seed did not have.
    const browserCalls = stubBackend(
      showsReply(
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
      paramsOf(serverCalls.find(u => u.includes('/shows?'))!),
    )
  })

  it('drops rows that have left the upcoming window since the read', async () => {
    const future = artistShow()
    stubBackend(
      showsReply({
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
    stubBackend(showsReply(upcomingBody([])))

    const html = serverRenderUpcoming(await renderPage())

    expect(html).toContain('No upcoming shows yet.')
    expect(html).not.toContain(LOADER)
  })

  it('still renders the page, with the loader, when the shows read fails', async () => {
    stubBackend(showsReply({}, 500))

    const page = await renderPage()

    expect(
      boundaryStatesOf(page)
        .flatMap(state => state.queries)
        .map(q => q.queryKey.slice(0, 2).join('/')),
    ).toEqual(['artists/detail'])
    const html = serverRenderUpcoming(page)
    expect(html).toContain(LOADER)
    expect(html).not.toContain('No upcoming shows yet.')
  })

  it('withholds rows that answer for a different artist', async () => {
    stubBackend(showsReply(upcomingBody([artistShow()], ARTIST_ID + 1)))

    expect(seededShowsQuery(await renderPage())).toBeUndefined()
  })

  it('404s an unknown artist', async () => {
    stubBackend(showsReply({}, 404), () => errorResponse(404))

    await expect(renderPage('missing')).rejects.toThrow(NOT_FOUND_SENTINEL)
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
      stubBackend(showsReply(upcomingBody([artistShow()])))
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
    vi.stubEnv('SERVER_READ_SECRET', 'artist-page-secret')
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.headers).toEqual({ 'X-Server-Read-Secret': 'artist-page-secret' })
  })

  it('sends no secret header when none is configured', async () => {
    vi.stubEnv('SERVER_READ_SECRET', '')
    fetchMock.mockResolvedValueOnce(okResponse(buildArtist()))

    await generateMetadata({ params: Promise.resolve({ slug: 'headliner-band' }) })

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.headers).toEqual({})
  })
})
