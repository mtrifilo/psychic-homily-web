import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Children, isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToString } from 'react-dom/server'
import {
  HydrationBoundary,
  QueryClient,
  QueryClientProvider,
  type DehydratedState,
} from '@tanstack/react-query'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'
import { getQueryClient } from '@/lib/queryClient'

// `notFound()` in Next.js throws to halt rendering; mirrored so a test can tell
// that execution stopped.
const NOT_FOUND_SENTINEL = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND_SENTINEL)
  }),
}))


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

/**
 * Route the page's two reads by URL. The shows read goes through
 * `fetchListPayload`, which reads the body as text, so it gets a real
 * `Response`; a fresh one per call, since a body can be read once.
 */
function stubBackend(shows: () => Response) {
  const calls: string[] = []
  fetchMock.mockImplementation(async (url: string) => {
    calls.push(String(url))
    if (String(url).includes('/shows?')) return shows()
    return okResponse(buildArtist({ id: ARTIST_ID }))
  })
  return calls
}

const jsonResponse = (body: unknown, status = 200) => () =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })

function findElement(
  root: ReactElement,
  match: (node: ReactElement) => boolean,
): ReactElement {
  const queue: unknown[] = [root]
  while (queue.length > 0) {
    const node = queue.shift()
    if (!isValidElement(node)) continue
    if (match(node)) return node
    queue.push(
      ...Children.toArray((node.props as { children?: ReactNode }).children),
    )
  }
  throw new Error('element not found in the page tree')
}

async function renderPage(slug = 'headliner-band') {
  return (await ArtistPage({ params: Promise.resolve({ slug }) })) as ReactElement
}

function dehydratedStateOf(page: ReactElement): DehydratedState {
  const boundary = findElement(page, node => node.type === HydrationBoundary)
  return (boundary.props as { state: DehydratedState }).state
}

/**
 * Server-render the REAL upcoming list under the page's own dehydrated state,
 * the way the route's HTML is produced: no client fetch can run inside
 * `renderToString`, so whatever this prints came from the seed.
 */
function serverRenderUpcoming(state: DehydratedState): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderToString(
    <QueryClientProvider client={client}>
      <HydrationBoundary state={state}>
        <ArtistShowsList
          artistId={ARTIST_ID}
          artistSlug="headliner-band"
          artistName="Headliner Band"
        />
      </HydrationBoundary>
    </QueryClientProvider>,
  )
}

const LOADER = 'animate-spin'

describe('ArtistPage upcoming shows', () => {
  // Under jsdom `getQueryClient()` hands back the BROWSER singleton rather than
  // a fresh server client per call, so each page render would otherwise
  // dehydrate every earlier test's seeds along with its own.
  beforeEach(() => {
    getQueryClient().clear()
  })

  it('asks for the upcoming request by encoded slug, alongside the artist', async () => {
    const calls = stubBackend(jsonResponse(upcomingBody([artistShow()])))

    await renderPage('weird/slug?x')

    const showsUrl = calls.find(u => u.includes('/shows?'))
    expect(showsUrl).toBeDefined()
    const url = new URL(showsUrl!)
    expect(url.pathname).toMatch(/\/artists\/weird%2Fslug%3Fx\/shows$/)
    expect(url.searchParams.get('limit')).toBe('200')
    expect(url.searchParams.get('time_filter')).toBe('upcoming')
    expect(calls).toHaveLength(2)
  })

  // Pinned as a LITERAL, not rebuilt from the same helpers the page uses: the
  // key `useArtistShows` registers for the list's arguments is what the seed
  // must hit, and a helper both sides share would agree with itself while the
  // hook drifted.
  it('seeds the rows under the exact key the list hook reads', async () => {
    const body = upcomingBody([artistShow()])
    stubBackend(jsonResponse(body))

    const state = dehydratedStateOf(await renderPage())
    const seeded = state.queries.find(
      q => q.queryKey[0] === 'artists' && q.queryKey[1] === 'shows',
    )

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

  it('puts the upcoming rows in the server render, with no loader', async () => {
    stubBackend(jsonResponse(upcomingBody([artistShow()])))

    const html = serverRenderUpcoming(dehydratedStateOf(await renderPage()))

    expect(html).toContain('href="/shows/2026-11-13-seeded-show"')
    expect(html).toContain('Support Act')
    expect(html).not.toContain(LOADER)
  })

  it('server-renders the empty state for an artist with no upcoming shows', async () => {
    stubBackend(jsonResponse(upcomingBody([])))

    const html = serverRenderUpcoming(dehydratedStateOf(await renderPage()))

    expect(html).toContain('No upcoming shows yet.')
    expect(html).not.toContain(LOADER)
  })

  it('still renders the page, with the loader, when the shows read fails', async () => {
    stubBackend(jsonResponse({}, 500))

    const page = await renderPage()
    const state = dehydratedStateOf(page)

    // The artist itself is still seeded and the page still renders its detail.
    expect(
      state.queries.map(q => q.queryKey.slice(0, 2).join('/')),
    ).toEqual(['artists/detail'])
    const html = serverRenderUpcoming(state)
    expect(html).toContain(LOADER)
    expect(html).not.toContain('No upcoming shows yet.')
  })

  it('withholds rows that answer for a different artist', async () => {
    stubBackend(jsonResponse(upcomingBody([artistShow()], ARTIST_ID + 1)))

    const state = dehydratedStateOf(await renderPage())

    expect(
      state.queries.some(q => q.queryKey[1] === 'shows'),
    ).toBe(false)
  })

  it('404s an unknown artist without waiting on a seed it will not use', async () => {
    fetchMock.mockImplementation(async (url: string) =>
      String(url).includes('/shows?')
        ? jsonResponse({}, 404)()
        : errorResponse(404),
    )

    await expect(renderPage('missing')).rejects.toThrow(NOT_FOUND_SENTINEL)
  })
})
