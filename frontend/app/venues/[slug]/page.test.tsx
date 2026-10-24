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
import { findElement, findElements } from '@/test/elementTree'
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
  usePathname: () => '/venues/the-rebel-lounge',
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


// Stub the heavy venues feature module so invoking generateMetadata doesn't
// pull in the real VenueDetail render path. A NAMED function so the page body
// tests can find the element the route hands its props to.
vi.mock('@/features/venues', () => ({
  VenueDetail: function VenueDetail(): null {
    return null
  },
}))

// What the server render of the real upcoming list needs around it: a viewer
// (signed out unless a test says otherwise), no show form, and no past archive
// (which owns its own requests and loader). Each viewer state is one the
// route's HTML can be served under, and the hydration render reads the same.
let viewer: ViewerState = ANONYMOUS_VIEWER
vi.mock('@/lib/context/AuthContext', async () => {
  const { deriveMockAuthSignals } = await import('@/test/authFixture')
  return { useAuthContext: () => deriveMockAuthSignals(viewer) }
})
vi.mock('@/features/shows', async importOriginal => ({
  ...(await importOriginal<typeof import('@/features/shows')>()),
  ShowForm: (): null => null,
}))
// The save-count batch answers in the browser after hydration, so the route's
// HTML and the first client render both see it pending.
vi.mock('@/features/shows/hooks/useSavedShows', async importOriginal => ({
  ...(await importOriginal<
    typeof import('@/features/shows/hooks/useSavedShows')
  >()),
  useShowSaveCountBatch: () => ({ data: undefined }),
}))
vi.mock('@/features/venues/components/VenuePastShows', () => ({
  VenuePastShows: (): null => null,
}))

import VenuePage, { generateMetadata } from './page'
import { VenueDetail } from '@/features/venues'
import { VenueShowsList } from '@/features/venues/components/VenueShowsList'

function buildVenue(overrides: Record<string, unknown> = {}) {
  return {
    id: 77,
    timezone: 'America/Phoenix',
    name: 'The Rebel Lounge',
    slug: 'the-rebel-lounge',
    address: '2303 E Indian School Rd',
    city: 'Phoenix',
    state: 'AZ',
    zip_code: '85016',
    ...overrides,
  }
}

const fetchMock = vi.fn()
const upcomingFetchMock = vi.fn()

/**
 * A venue upcoming-shows response: the list the metadata reads its `Next`
 * clause from, and the rows the page body seeds.
 */
function upcomingResponse(shows: unknown[], venueId = 77): Response {
  return jsonBodyResponse({
    shows,
    venue_id: venueId,
    total: shows.length,
    limit: 7,
    offset: 0,
    year: 0,
  })
}

// 8:00 PM on Friday, September 11, 2026 in Phoenix.
function upcomingShow(overrides: Record<string, unknown> = {}) {
  return {
    id: 901,
    slug: 'next-show',
    title: 'Next Show',
    event_date: '2026-09-12T03:00:00Z',
    city: 'Phoenix',
    state: 'AZ',
    is_cancelled: false,
    is_sold_out: false,
    artists: [
      { id: 1, slug: 'opener', name: 'Opener', is_headliner: false },
      { id: 2, slug: 'top-billing', name: 'Top Billing', is_headliner: true },
    ],
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  upcomingFetchMock.mockResolvedValue(upcomingResponse([]))
  vi.stubGlobal('fetch', (url: string, init?: RequestInit) => {
    if (String(url).includes('/shows?')) return upcomingFetchMock(url, init)
    return fetchMock(url, init)
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('venues/[slug] generateMetadata', () => {
  it('puts the city and state in the title', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.title).toBe('The Rebel Lounge · Phoenix, AZ')
  })

  it('names the next show in the description', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    upcomingFetchMock.mockResolvedValueOnce(upcomingResponse([upcomingShow()]))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.description).toBe(
      'Upcoming shows at The Rebel Lounge in Phoenix, AZ. Next: Top Billing, Sep 11.'
    )
  })

  it('asks for the venue upcoming shows by id', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(upcomingFetchMock).toHaveBeenCalledTimes(1)
    const url = new URL(String(upcomingFetchMock.mock.calls[0][0]))
    expect(url.pathname).toMatch(/\/venues\/77\/shows$/)
    expect(url.searchParams.get('time_filter')).toBe('upcoming')
  })

  it('skips a cancelled show when naming the next one', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    upcomingFetchMock.mockResolvedValueOnce(
      upcomingResponse([
        upcomingShow({
          is_cancelled: true,
          artists: [{ id: 3, slug: 'called-off', name: 'Called Off', is_headliner: true }],
        }),
        upcomingShow(),
      ])
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.description).toContain('Next: Top Billing, Sep 11.')
    expect(meta.description).not.toContain('Called Off')
  })

  it('omits the Next clause when the venue has no upcoming show', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.description).toBe('Upcoming shows at The Rebel Lounge in Phoenix, AZ.')
  })

  it('omits the Next clause when the upcoming read fails', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    upcomingFetchMock.mockResolvedValueOnce(errorResponse(500))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.title).toBe('The Rebel Lounge · Phoenix, AZ')
    expect(meta.description).toBe('Upcoming shows at The Rebel Lounge in Phoenix, AZ.')
  })

  it('makes no upcoming read for a venue without a usable id', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue({ id: 'not-a-number' })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(upcomingFetchMock).not.toHaveBeenCalled()
    expect(meta.description).toBe('Upcoming shows at The Rebel Lounge in Phoenix, AZ.')
  })

  it('sets the canonical URL to https://psychichomily.com/venues/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/venues/the-rebel-lounge'
    )
  })

  it('mirrors the title and description into openGraph and sets url/type', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'the-rebel-lounge' }) })

    expect(meta.openGraph?.title).toBe(meta.title)
    expect(meta.openGraph?.description).toBe(meta.description)
    expect(meta.openGraph?.url).toBe('/venues/the-rebel-lounge')
    expect((meta.openGraph as { type?: string })?.type).toBe('website')
  })

  it('falls back to the "Venue" title when the venue is missing', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.title).toBe('Venue')
    expect(meta.description).toBe('View venue details and upcoming shows')
    // No canonical alternate on the fallback shape.
    expect(meta.alternates).toBeUndefined()
    expect(meta.openGraph).toBeUndefined()
  })
})

// ── Page body: the upcoming-shows seed ────────────────────────────────────

const VENUE_ID = 77

/**
 * A show that is still upcoming whenever this suite runs. The page drops rows
 * whose night has provably passed, and `upcomingShow`'s own date (used by the
 * metadata tests above) is fixed in 2026.
 */
const futureShow = (overrides: Record<string, unknown> = {}) =>
  upcomingShow({ event_date: '2099-09-12T03:00:00Z', ...overrides })

/**
 * The page body's non-list reads: the venue, and the past-year histogram,
 * which answers "unavailable" because it is not what these tests are about.
 * The upcoming read goes to `upcomingFetchMock` through the file's router.
 */
function stubVenueReads(venue: () => Response = () => okResponse(buildVenue())) {
  fetchMock.mockImplementation(async (url: string) =>
    String(url).includes('/shows/years') ? errorResponse(500) : venue(),
  )
}

async function renderPage(slug = 'the-rebel-lounge') {
  return (await VenuePage({ params: Promise.resolve({ slug }) })) as ReactElement
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
    .find(q => q.queryKey[0] === 'venues' && q.queryKey[1] === 'shows')
}

/** The real upcoming list under the page's own boundaries, nested as the page nests them. */
function upcomingListUnder(states: DehydratedState[], client: QueryClient) {
  const list = (
    <VenueShowsList
      venueId={VENUE_ID}
      venueSlug="the-rebel-lounge"
      venueName="The Rebel Lounge"
      venueCity="Phoenix"
      venueState="AZ"
      venueTimezone="America/Phoenix"
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

/** The page body's upcoming read, as distinct from the metadata's smaller one. */
const pageUpcomingCalls = () =>
  upcomingFetchMock.mock.calls
    .map(([url]) => new URL(String(url)))
    .filter(url => url.searchParams.get('limit') === '200')

const LOADER = 'animate-spin'

/** A request's query parameters, order-independent. */
const paramsOf = (url: string) =>
  [...new URL(url).searchParams.entries()].sort(([a], [b]) => a.localeCompare(b))

describe('VenuePage upcoming shows', () => {
  beforeEach(() => {
    stubVenueReads()
  })

  it('asks for the upcoming request by encoded slug', async () => {
    await renderPage('weird/slug?x')

    expect(upcomingFetchMock).toHaveBeenCalledTimes(1)
    const url = new URL(String(upcomingFetchMock.mock.calls[0][0]))
    expect(url.pathname).toMatch(/\/venues\/weird%2Fslug%3Fx\/shows$/)
    expect(url.searchParams.get('limit')).toBe('200')
    expect(url.searchParams.get('time_filter')).toBe('upcoming')
  })

  it('asks for a zero-padded numeric id by its unpadded form', async () => {
    await renderPage('0077')

    expect(pageUpcomingCalls().map(url => url.pathname)).toEqual([
      expect.stringMatching(/\/venues\/77\/shows$/),
    ])
  })

  // Pins the SEED's key spelling as a literal, so it cannot agree with a
  // shared helper while both drift. That the list hook reads this entry is
  // proven by the server-render test below, which runs the real hook.
  it('seeds the rows under the key the list hook registers, stamped stale', async () => {
    upcomingFetchMock.mockImplementation(async () => upcomingResponse([futureShow()]))

    const seeded = seededShowsQuery(await renderPage())

    expect(seeded?.queryKey).toEqual([
      'venues',
      'shows',
      String(VENUE_ID),
      'upcoming',
      200,
      null,
      null,
    ])
    expect(seeded?.state.data).toMatchObject({ shows: [futureShow()], venue_id: VENUE_ID })
    expect(seeded?.state.dataUpdatedAt).toBe(0)
  })

  it('puts the upcoming rows in the server render, with no loader', async () => {
    upcomingFetchMock.mockImplementation(async () => upcomingResponse([futureShow()]))

    const html = serverRenderUpcoming(await renderPage())

    expect(html).toContain('href="/shows/next-show"')
    expect(html).toContain('Top Billing')
    expect(html).not.toContain(LOADER)
  })

  it('replaces the seeded rows with the live read once mounted', async () => {
    upcomingFetchMock.mockImplementation(async () => upcomingResponse([futureShow()]))
    const states = boundaryStatesOf(await renderPage())
    // The browser's own read answers with a show the seed did not have.
    upcomingFetchMock.mockImplementation(async () =>
      upcomingResponse([futureShow({ id: 902, slug: 'late-addition' })]),
    )

    const { container } = render(upcomingListUnder(states, testClient()))

    expect(container.innerHTML).toContain('href="/shows/next-show"')
    await waitFor(() =>
      expect(container.innerHTML).toContain('href="/shows/late-addition"'),
    )
    // The server asked the question the hook asks.
    const [serverUrl, browserUrl] = upcomingFetchMock.mock.calls.map(([url]) => String(url))
    expect(paramsOf(browserUrl)).toEqual(paramsOf(serverUrl))
  })

  it('drops rows that have left the upcoming window since the read', async () => {
    const future = futureShow()
    upcomingFetchMock.mockImplementation(async () =>
      jsonBodyResponse({
        shows: [futureShow({ id: 900, slug: 'long-gone', event_date: '2020-01-01T03:00:00Z' }), future],
        venue_id: VENUE_ID,
        total: 5,
        limit: 200,
        offset: 0,
        year: 0,
      }),
    )

    const seeded = seededShowsQuery(await renderPage())

    expect(seeded?.state.data).toMatchObject({ shows: [future], total: 4 })
  })

  it('server-renders the empty state for a venue with no upcoming shows', async () => {
    const html = serverRenderUpcoming(await renderPage())

    expect(html).toContain('No upcoming shows yet.')
    expect(html).not.toContain(LOADER)
  })

  it('still renders the venue, with the loader, when the upcoming read fails', async () => {
    upcomingFetchMock.mockImplementation(async () => errorResponse(500))

    const page = await renderPage()

    expect(findElement(page, node => node.type === VenueDetail).props).toMatchObject({
      venueId: 'the-rebel-lounge',
    })
    expect(seededShowsQuery(page)).toBeUndefined()
    const html = serverRenderUpcoming(page)
    expect(html).toContain(LOADER)
    expect(html).not.toContain('No upcoming shows yet.')
  })

  it('withholds rows that answer for a different venue', async () => {
    upcomingFetchMock.mockImplementation(async () =>
      upcomingResponse([futureShow()], VENUE_ID + 1),
    )

    expect(seededShowsQuery(await renderPage())).toBeUndefined()
  })

  it('404s an unknown venue', async () => {
    stubVenueReads(() => errorResponse(404))

    await expect(renderPage('missing')).rejects.toThrow(NOT_FOUND_SENTINEL)
  })
})

describe('VenuePage upcoming Save column', () => {
  let root: Root | null = null
  let container: HTMLElement | null = null

  beforeEach(() => {
    stubVenueReads()
  })

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
      upcomingFetchMock.mockImplementation(async () =>
        upcomingResponse([futureShow()])
      )
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
