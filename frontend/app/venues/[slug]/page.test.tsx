import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Children, isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToString } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'
import type { VenueShowsResponse } from '@/features/venues/types'

// `notFound()` in Next.js throws to halt rendering; mirrored so a test can tell
// that execution stopped.
const NOT_FOUND_SENTINEL = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND_SENTINEL)
  }),
}))


// Stub the heavy venues feature module so invoking generateMetadata doesn't
// pull in the real VenueDetail render path. A NAMED function so the page body
// tests can find the element the route hands its props to.
vi.mock('@/features/venues', () => ({
  VenueDetail: function VenueDetail(): null {
    return null
  },
}))

// What the server render of the real upcoming list needs around it: a
// signed-out reader, no show form, and no past archive (which owns its own
// requests and loader).
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: false }),
}))
vi.mock('@/features/shows', () => ({
  ShowForm: (): null => null,
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

/** The venue shows list the metadata reads its `Next` clause from. */
function upcomingResponse(shows: unknown[]): Response {
  return new Response(
    JSON.stringify({ shows, venue_id: 77, total: shows.length, limit: 7, offset: 0, year: 0 }),
    { status: 200, headers: { 'content-type': 'application/json' } }
  )
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

function upcomingBody(shows: unknown[], venueId = VENUE_ID) {
  return { shows, venue_id: venueId, total: shows.length, limit: 200, offset: 0, year: 0 }
}

/**
 * Route the page's three reads by URL. `/shows?` is the upcoming read; the
 * year histogram is `/shows/years?`, which answers "unavailable" here because
 * it is not what these tests are about.
 */
function stubPageBackend(upcoming: () => Response) {
  const calls: string[] = []
  vi.stubGlobal('fetch', async (url: string) => {
    calls.push(String(url))
    if (String(url).includes('/shows/years')) return errorResponse(500)
    if (String(url).includes('/shows?')) return upcoming()
    return okResponse(buildVenue())
  })
  return calls
}

const jsonResponse = (body: unknown, status = 200) => () =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })

async function venueDetailPropsFrom(
  slug = 'the-rebel-lounge',
): Promise<Record<string, unknown>> {
  const page = (await VenuePage({ params: Promise.resolve({ slug }) })) as ReactElement
  const queue: unknown[] = [page]
  while (queue.length > 0) {
    const node = queue.shift()
    if (!isValidElement(node)) continue
    if (node.type === VenueDetail) return node.props as Record<string, unknown>
    queue.push(
      ...Children.toArray((node.props as { children?: ReactNode }).children),
    )
  }
  throw new Error('VenueDetail was not found in the page tree')
}

/**
 * Server-render the REAL upcoming list with the rows the route handed down,
 * the way the route's HTML is produced: no client fetch can run inside
 * `renderToString`, so whatever this prints came from the seed.
 */
function serverRenderUpcoming(initialUpcomingShows: unknown): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderToString(
    <QueryClientProvider client={client}>
      <VenueShowsList
        venueId={VENUE_ID}
        venueSlug="the-rebel-lounge"
        venueName="The Rebel Lounge"
        venueCity="Phoenix"
        venueState="AZ"
        venueTimezone="America/Phoenix"
        initialUpcomingShows={initialUpcomingShows as VenueShowsResponse | undefined}
      />
    </QueryClientProvider>,
  )
}

const LOADER = 'animate-spin'

describe('VenuePage upcoming shows', () => {
  it('asks for the upcoming request by encoded slug', async () => {
    const calls = stubPageBackend(jsonResponse(upcomingBody([upcomingShow()])))

    await venueDetailPropsFrom('weird/slug?x')

    const upcoming = calls.filter(u => u.includes('/shows?'))
    expect(upcoming).toHaveLength(1)
    const url = new URL(upcoming[0])
    expect(url.pathname).toMatch(/\/venues\/weird%2Fslug%3Fx\/shows$/)
    expect(url.searchParams.get('limit')).toBe('200')
    expect(url.searchParams.get('time_filter')).toBe('upcoming')
  })

  it('hands the rows to the venue tree and they reach the server render', async () => {
    const body = upcomingBody([upcomingShow()])
    stubPageBackend(jsonResponse(body))

    const props = await venueDetailPropsFrom()
    expect(props.initialUpcomingShows).toEqual(body)

    const html = serverRenderUpcoming(props.initialUpcomingShows)
    expect(html).toContain('href="/shows/next-show"')
    expect(html).toContain('Top Billing')
    expect(html).not.toContain(LOADER)
  })

  it('server-renders the empty state for a venue with no upcoming shows', async () => {
    stubPageBackend(jsonResponse(upcomingBody([])))

    const props = await venueDetailPropsFrom()
    const html = serverRenderUpcoming(props.initialUpcomingShows)

    expect(html).toContain('No upcoming shows yet.')
    expect(html).not.toContain(LOADER)
  })

  it('still renders the venue, with the loader, when the upcoming read fails', async () => {
    stubPageBackend(jsonResponse({}, 500))

    const props = await venueDetailPropsFrom()
    expect(props.venueId).toBe('the-rebel-lounge')
    expect(props.initialUpcomingShows).toBeUndefined()

    const html = serverRenderUpcoming(props.initialUpcomingShows)
    expect(html).toContain(LOADER)
    expect(html).not.toContain('No upcoming shows yet.')
  })

  it('withholds rows that answer for a different venue', async () => {
    stubPageBackend(jsonResponse(upcomingBody([upcomingShow()], VENUE_ID + 1)))

    const props = await venueDetailPropsFrom()

    expect(props.initialUpcomingShows).toBeUndefined()
  })

  it('404s an unknown venue', async () => {
    vi.stubGlobal('fetch', async (url: string) =>
      String(url).includes('/shows?') ? jsonResponse({}, 404)() : errorResponse(404),
    )

    await expect(venueDetailPropsFrom('missing')).rejects.toThrow(NOT_FOUND_SENTINEL)
  })
})
