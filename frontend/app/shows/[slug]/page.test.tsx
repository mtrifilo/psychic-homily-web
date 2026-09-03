import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Children, isValidElement, type ReactElement } from 'react'
import * as Sentry from '@sentry/nextjs'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'
import { JsonLd } from '@/components/seo/JsonLd'

// `notFound()` in Next.js throws a control-flow error to halt rendering. We
// mirror that here so tests can assert BOTH that it was called and that
// execution stopped (no JSON-LD / ShowDetail render after a 404).
const NOT_FOUND_SENTINEL = 'NEXT_NOT_FOUND'
const notFoundMock = vi.fn(() => {
  throw new Error(NOT_FOUND_SENTINEL)
})

vi.mock('next/navigation', () => ({
  notFound: () => notFoundMock(),
}))

// `connection()` marks the subtree dynamic at request time and has nothing to
// do here. Stubbed so the tests below can invoke the async component that
// awaits it and read the props it hands the client tree.
vi.mock('next/server', () => ({
  connection: async () => {},
}))


// These tests exercise the page-level metadata + JSON-LD + notFound wiring.
// They CALL `ShowPage()` and walk the returned element tree; nothing is ever
// rendered, so ShowDetail's module is never loaded and needs no stub. (Do not
// read that as "the dynamic() boundary is covered" — it is not exercised here
// at all. If a future test renders the tree, stub the component FILE, not this
// module: since PSY-1772 the page imports it by path, not through the barrel.)
vi.mock('@/features/shows/utils', () => ({
  // The page reads the show's timezone inputs to compute the status stripe's
  // lifecycle. This is the only value the page takes from the feature.
  showTimingInput: (show: { event_date: string }) => ({
    eventDate: show.event_date,
    state: null,
    timezone: null,
  }),
}))

import ShowPage, { generateMetadata } from './page'
import {
  VENUE_RAIL_FETCH_LIMIT,
  VENUE_RAIL_TIME_FILTER,
} from '@/features/shows/showRails'

/**
 * The props the route hands the client tree.
 *
 * The page returns `ShowDetailWithLifecycle` as an UNINVOKED element inside its
 * Suspense boundary, so nothing below it runs until it is called — which is the
 * whole reason the rail seeding had no coverage. Calling it here executes the
 * real component (its `connection()` is stubbed above) and returns the
 * `<ShowDetail>` element without loading that module, because `dynamic()` only
 * resolves on render.
 */
async function showDetailPropsFrom(
  result: ReactElement
): Promise<Record<string, unknown>> {
  const queue: unknown[] = [result]
  while (queue.length > 0) {
    const node = queue.shift()
    if (!isValidElement(node)) continue
    const type = node.type as { name?: string }
    if (
      typeof node.type === 'function' &&
      type.name === 'ShowDetailWithLifecycle'
    ) {
      const rendered = await (
        node.type as (props: unknown) => Promise<ReactElement>
      )(node.props)
      return rendered.props as Record<string, unknown>
    }
    queue.push(
      ...Children.toArray(
        (node.props as { children?: React.ReactNode }).children
      )
    )
  }
  throw new Error('ShowDetailWithLifecycle was not found in the page tree')
}

// Minimal show payload — only the fields generateMetadata / the page body read.
function buildShow(overrides: Record<string, unknown> = {}) {
  return {
    title: 'Test Show',
    event_date: '2026-03-15T20:00:00Z',
    slug: 'test-show',
    description: null as string | null,
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 77,
        name: 'The Rebel Lounge',
        slug: 'the-rebel-lounge',
        city: 'Phoenix',
        state: 'AZ',
      },
    ],
    artists: [
      { name: 'Headliner Band', slug: 'headliner-band', is_headliner: true, socials: {} },
    ],
    ...overrides,
  }
}

/**
 * Pull the MusicEvent block out of the page's rendered `<JsonLd>` children.
 *
 * The page emits two JSON-LD scripts (MusicEvent + BreadcrumbList) as direct
 * children of a fragment, so matching on the component type and `@type` is
 * enough — no renderer needed for what is a pure data assertion.
 */
function musicEventSchemaFrom(result: ReactElement): Record<string, unknown> {
  const children = Children.toArray(
    (result.props as { children?: React.ReactNode }).children
  )
  for (const child of children) {
    if (!isValidElement(child) || child.type !== JsonLd) continue
    const { data } = child.props as { data?: Record<string, unknown> }
    if (data?.['@type'] === 'MusicEvent') return data
  }
  // Throw rather than return undefined: if the page ever wraps its JSON-LD in
  // another element, the assertions below would otherwise fail with a confusing
  // message about `offers` instead of naming the real cause.
  throw new Error('No MusicEvent JSON-LD found among the page\'s direct children')
}

/**
 * `hasShowStarted` reads the real clock and the page has no seam to inject one, so
 * these sit far enough either side of "now" that the wall clock cannot flip
 * them. (`buildSceneWeekJsonLd` takes an injectable `now`; this page does not.)
 */
const PAST_DATE = '2020-03-15T20:00:00Z'
const FUTURE_DATE = '2099-03-15T20:00:00Z'

/**
 * The SHOW read's mock. Every test below queues its case on this one, and the
 * queue is positional, so it must see only the show request.
 */
const fetchMock = vi.fn()

/**
 * The TIMELINE read's mock, kept separate because the page starts that fetch
 * BEFORE it awaits the show, so the two reads overlap on one round trip. A
 * single shared mock would hand the timeline the response the test queued for
 * the show, and every assertion here would then be reading the wrong request.
 *
 * Defaulted to an empty timeline: these tests are about metadata, JSON-LD and
 * the notFound wiring, and the timeline is two supplementary lines the page
 * renders past. A test that wants a populated one overrides it.
 */
const timelineFetchMock = vi.fn()

/**
 * The two DISCOVERY RAIL reads, mocked per endpoint for the same reason the
 * timeline is: the page starts all of them around the show read, so one shared
 * mock would hand each whichever response the test queued for another.
 */
const alsoTonightFetchMock = vi.fn()
const venueRailFetchMock = vi.fn()

const EMPTY_RAIL = { shows: [], date: '2026-03-15', timezone: 'America/Phoenix' }
const EMPTY_VENUE_RAIL = { shows: [], venue_id: 77, total: 0 }

/**
 * A rail response stub, which needs `text()` where `okResponse` offers only
 * `json()`: the rails are read through `fetchListPayload`, which weighs the
 * body against the Data Cache budget on the way through and therefore reads it
 * as text. A stub without it fails every rail read as a thrown TypeError,
 * which the helper swallows into the same `null` a real outage produces — so
 * the seeding assertions would pass their "no seed" half and never the other.
 */
function railResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response
}

const EMPTY_TIMELINE = {
  previous: null,
  next: null,
  recurrence: [],
  headliner_artist_id: 0,
}

beforeEach(() => {
  vi.clearAllMocks()
  timelineFetchMock.mockResolvedValue(okResponse(EMPTY_TIMELINE))
  alsoTonightFetchMock.mockResolvedValue(railResponse(EMPTY_RAIL))
  venueRailFetchMock.mockResolvedValue(railResponse(EMPTY_VENUE_RAIL))
  vi.stubGlobal('fetch', (url: string, init?: RequestInit) => {
    const at = String(url)
    if (at.includes('/timeline')) return timelineFetchMock(url, init)
    if (at.includes('/also-tonight')) return alsoTonightFetchMock(url, init)
    if (at.includes('/venues/')) return venueRailFetchMock(url, init)
    return fetchMock(url, init)
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('generateMetadata', () => {
  it('uses "{headliner} at {venue}" as the title when the show is found', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.title).toBe('Headliner Band at The Rebel Lounge')
  })

  it('prefers the headliner over the first artist for the title', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildShow({
          artists: [
            { name: 'Opener', slug: 'opener', is_headliner: false, socials: {} },
            { name: 'Real Headliner', slug: 'real-headliner', is_headliner: true, socials: {} },
          ],
        })
      )
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.title).toBe('Real Headliner at The Rebel Lounge')
  })

  it('falls back to the "Show" title when the show is missing', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.title).toBe('Show')
    expect(meta.description).toBe('View show details')
    // No canonical alternate on the fallback shape.
    expect(meta.alternates).toBeUndefined()
  })

  it('truncates a long description at 155 chars and appends "..."', async () => {
    const longDescription = 'x'.repeat(300)
    fetchMock.mockResolvedValueOnce(okResponse(buildShow({ description: longDescription })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.description).toBe('x'.repeat(155) + '...')
    expect(meta.description).toHaveLength(158)
  })

  it('does not append "..." when the description is exactly 155 chars', async () => {
    const exactDescription = 'y'.repeat(155)
    fetchMock.mockResolvedValueOnce(okResponse(buildShow({ description: exactDescription })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.description).toBe(exactDescription)
    expect(meta.description).toHaveLength(155)
  })

  it('generates a description from show details when none is provided', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow({ description: null })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    // Generated form: "{headliner} live at {venue} on {date}".
    expect(meta.description).toContain('Headliner Band live at The Rebel Lounge on')
  })

  it('dates the generated description on the venue calendar', async () => {
    // 03:00Z is the previous evening in Phoenix, so a description built on the
    // raw instant would name the wrong day in the search result.
    fetchMock.mockResolvedValueOnce(
      okResponse(buildShow({ event_date: '2026-09-10T03:00:00Z' }))
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.description).toContain('on Wednesday, September 9, 2026')
  })

  it('marks the day in the description when the venue zone is a guess', async () => {
    // PSY-1964: the meta description is one of the four date renders on this
    // page, and they mark together or the page contradicts itself.
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildShow({
          event_date: '2026-09-10T03:00:00Z',
          venues: [
            { name: 'Hall Ohne Zone', slug: 'hall', city: 'Berlin', state: '' },
          ],
        })
      )
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.description).toContain('on ~Wednesday, September 9, 2026')
  })

  it('sets the canonical URL to https://psychichomily.com/shows/{slug}', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.alternates?.canonical).toBe('https://psychichomily.com/shows/test-show')
    expect(meta.openGraph?.url).toBe('/shows/test-show')
  })

  it('declares a large-image Twitter card mirroring the OG title/description', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.twitter).toMatchObject({
      card: 'summary_large_image',
      title: 'Headliner Band at The Rebel Lounge',
    })
    expect(meta.twitter?.description).toBe(meta.openGraph?.description)
    expect(meta.twitter?.title).toBe(meta.openGraph?.title)
  })

  // Leaving `images` unset is what lets the per-show opengraph-image through —
  // see page.tsx. A proxy for a Next-internal merge, so it would not catch the
  // resolver changing shape on an upgrade (verified against Next 16.1.4).
  it('leaves twitter.images unset so the per-show OG image is used', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(meta.twitter).toBeDefined()
    expect(meta.twitter && 'images' in meta.twitter).toBe(false)
  })

  it('omits the Twitter card on the not-found fallback shape', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'missing' }) })

    expect(meta.twitter).toBeUndefined()
  })
})

describe('ShowPage', () => {
  // An empty slug 404s rather than rendering an inline placeholder.
  it('calls notFound() when the slug is empty', async () => {
    await expect(
      ShowPage({ params: Promise.resolve({ slug: '' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(notFoundMock).toHaveBeenCalledTimes(1)
    // Empty slug short-circuits before any fetch.
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('calls notFound() when getShow returns null (404 from API)', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    await expect(
      ShowPage({ params: Promise.resolve({ slug: 'missing' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(notFoundMock).toHaveBeenCalledTimes(1)
  })

  // The reads overlap, so each must be addressed to its own endpoint. A page
  // that sent two of them to the same URL would still pass every assertion
  // below while making two identical requests.
  it('addresses the show, the timeline and both rails as separate reads', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][0]).toContain('/shows/test-show')
    expect(fetchMock.mock.calls[0][0]).not.toContain('/timeline')
    expect(timelineFetchMock).toHaveBeenCalledTimes(1)
    expect(timelineFetchMock.mock.calls[0][0]).toContain(
      '/shows/test-show/timeline'
    )
    expect(alsoTonightFetchMock).toHaveBeenCalledTimes(1)
    expect(venueRailFetchMock).toHaveBeenCalledTimes(1)
  })

  // The seed and the rail's own hook must ask the same QUESTION: the rows the
  // server reads become the only page the rail ever filters. Asserted against
  // the real constants the hook sends, never a restated literal.
  it('asks the venue rail for the page the rail hook asks for', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })

    const url = String(venueRailFetchMock.mock.calls[0][0])
    expect(url).toContain(`limit=${VENUE_RAIL_FETCH_LIMIT}`)
    expect(url).toContain(`time_filter=${VENUE_RAIL_TIME_FILTER}`)
    expect(url).toContain('/venues/77/shows')
  })

  // Next hands route params through DECODED, so an unencoded `%2F` would
  // re-point this server-side read at another backend path.
  it('encodes the slug in the night rail request', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    await ShowPage({ params: Promise.resolve({ slug: 'a/b?c' }) })

    expect(String(alsoTonightFetchMock.mock.calls[0][0])).toContain(
      '/shows/a%2Fb%3Fc/also-tonight'
    )
  })

  it('asks for no venue rail when the show has no venue', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow({ venues: [] })))

    await ShowPage({ params: Promise.resolve({ slug: 'venue-less-show' }) })

    expect(venueRailFetchMock).not.toHaveBeenCalled()
  })

  // Both rails are discovery asides. A failing one degrades to no seed, which
  // leaves that rail to the client query it already had — it must never take
  // the page down or knock it out of its prerender.
  it('still renders the show when both rail reads fail', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))
    alsoTonightFetchMock.mockRejectedValue(new Error('scene down'))
    venueRailFetchMock.mockResolvedValue(errorResponse(500))

    const result = await ShowPage({ params: Promise.resolve({ slug: 'rails-down-show' }) })
    const props = await showDetailPropsFrom(result)

    expect(notFoundMock).not.toHaveBeenCalled()
    expect(props.initialAlsoTonight).toBeUndefined()
    expect(props.initialVenueShows).toBeUndefined()
  })

  it('seeds both rails and one clock into the client tree', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildShow({ event_date: FUTURE_DATE }))
    )

    const result = await ShowPage({ params: Promise.resolve({ slug: 'seeded-show' }) })
    const props = await showDetailPropsFrom(result)

    expect(props.initialAlsoTonight).toEqual(EMPTY_RAIL)
    expect(props.initialVenueShows).toEqual(EMPTY_VENUE_RAIL)
    // The rails order the night against this instant rather than the reader's
    // clock, so it has to reach them.
    expect(Date.parse(String(props.renderedAt))).not.toBeNaN()
    expect(props.lifecycle).toBe('upcoming')
  })

  // An archive page must not offer a reader other shows they equally cannot
  // attend, so the night rail's rows are dropped rather than seeded.
  it('withholds the night rail seed on a past show', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildShow({ event_date: PAST_DATE }))
    )

    const result = await ShowPage({ params: Promise.resolve({ slug: 'archive-show' }) })
    const props = await showDetailPropsFrom(result)

    expect(props.lifecycle).toBe('past')
    expect(props.initialAlsoTonight).toBeUndefined()
    // The room's rail is forward-looking whatever the subject show's date.
    expect(props.initialVenueShows).toEqual(EMPTY_VENUE_RAIL)
  })

  // The timeline is two supplementary lines. A show page must not 404 because
  // an archive query failed, and it must not report the show read as the
  // casualty of it.
  it('still renders the show when the timeline read fails', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))
    timelineFetchMock.mockRejectedValue(new Error('archive down'))

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(notFoundMock).not.toHaveBeenCalled()
    expect(musicEventSchemaFrom(result)['@type']).toBe('MusicEvent')
  })

  it('renders without calling notFound() when the show is found', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildShow()))

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })

    expect(notFoundMock).not.toHaveBeenCalled()
    expect(result).toBeTruthy()
  })
})

// The generator already guards these; these tests cover the wiring, which is
// what was missing — the page never told it whether the show had happened.
describe('ShowPage MusicEvent offers', () => {
  it('emits no offers for a show that already happened', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildShow({ event_date: PAST_DATE, price: 20 }))
    )

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })
    const schema = musicEventSchemaFrom(result)

    expect(schema.offers).toBeUndefined()
  })

  // A show whose date the backend never gave us must not advertise tickets
  // forever — the failure directions here are not symmetric.
  //
  // Venue-less on purpose: with a venue, the builder formats the start time in
  // the venue's zone and an unparseable date throws out of `toZonedISOString`
  // before any of this is reached. That crash is pre-existing and out of scope
  // here, so this pins the branch on the path that survives to the offer.
  it('emits no offers when the event date is unparseable', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildShow({ event_date: 'not-a-date', price: 20, venues: [] }))
    )

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })
    const schema = musicEventSchemaFrom(result)

    expect(schema.offers).toBeUndefined()
  })

  it('emits an offer for an upcoming show', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildShow({ event_date: FUTURE_DATE, price: 20 }))
    )

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })
    const schema = musicEventSchemaFrom(result)

    expect(schema.offers).toMatchObject({
      '@type': 'Offer',
      price: 20,
      availability: 'https://schema.org/InStock',
    })
  })

  // PSY-1864: the builder gates the whole offers block on a price, so a show
  // priced only at the door would drop out of search-result pricing entirely
  // unless the door price feeds it. Posture is unchanged — price + seller,
  // still no url.
  it('emits an offer from the door price when there is no advance price', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildShow({ event_date: FUTURE_DATE, price: null, door_price: 15 })
      )
    )

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })
    const schema = musicEventSchemaFrom(result)

    expect(schema.offers).toMatchObject({ '@type': 'Offer', price: 15 })
    expect(schema.offers).not.toHaveProperty('url')
  })

  // The advance price stays the offer price when both are recorded.
  it('prefers the advance price over the door price in the offer', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildShow({ event_date: FUTURE_DATE, price: 35, door_price: 40 })
      )
    )

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })
    const schema = musicEventSchemaFrom(result)

    expect(schema.offers).toMatchObject({ '@type': 'Offer', price: 35 })
  })

  /**
   * The show page must not publish a purchase URL — neither the vendor's nor
   * its own. See PSY-1669's Linear thread for the full decision trail: the
   * original acceptance criteria asked for the vendor's `ticket_url`, that was
   * reversed because this site has no referral arrangement and would be giving
   * away sales for free, and the self-referencing URL that replaced it was
   * dropped too because our show page is not the "landing page that clearly
   * and predominantly provides the opportunity to buy" that Google's field
   * expects. `offers.url` is Recommended, not required; omitting it costs only
   * the "ticket purchase option" placement, while price and sold-out status
   * still surface. This test exists so neither URL is reintroduced by a future
   * agent reading the original ticket.
   */
  it('publishes no purchase URL, even for a show with a vendor ticket link', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildShow({
          event_date: FUTURE_DATE,
          price: 20,
          ticket_url: 'https://dice.fm/event/abc',
        })
      )
    )

    const result = await ShowPage({ params: Promise.resolve({ slug: 'test-show' }) })
    const offers = musicEventSchemaFrom(result).offers as Record<string, unknown>

    expect('url' in offers).toBe(false)
    expect(JSON.stringify(offers)).not.toContain('dice.fm')
    // The vendor is named, not linked.
    expect(offers.seller).toEqual({ '@type': 'Organization', name: 'DICE' })
  })
})

describe('getShow error reporting (via the page body)', () => {
  it('reports a 5xx response with Sentry.captureMessage and then 404s', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(503))

    await expect(
      ShowPage({ params: Promise.resolve({ slug: 'boom' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(Sentry.captureMessage).toHaveBeenCalledTimes(1)
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'Show page: API returned 503',
      expect.objectContaining({
        level: 'error',
        tags: { service: 'show-page' },
        extra: { slug: 'boom', status: 503 },
      })
    )
    // 5xx is reported, not swallowed as an exception.
    expect(Sentry.captureException).not.toHaveBeenCalled()
  })

  it('does NOT report a 404 response to Sentry', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))

    await expect(
      ShowPage({ params: Promise.resolve({ slug: 'missing' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(Sentry.captureMessage).not.toHaveBeenCalled()
    expect(Sentry.captureException).not.toHaveBeenCalled()
  })

  it('reports a thrown fetch error with Sentry.captureException and then 404s', async () => {
    const networkError = new Error('network down')
    fetchMock.mockRejectedValueOnce(networkError)

    await expect(
      ShowPage({ params: Promise.resolve({ slug: 'flaky' }) })
    ).rejects.toThrow(NOT_FOUND_SENTINEL)

    expect(Sentry.captureException).toHaveBeenCalledTimes(1)
    expect(Sentry.captureException).toHaveBeenCalledWith(
      networkError,
      expect.objectContaining({
        level: 'error',
        tags: { service: 'show-page' },
        extra: { slug: 'flaky' },
      })
    )
    expect(Sentry.captureMessage).not.toHaveBeenCalled()
  })

  it('captures a 5xx in generateMetadata as well (both call sites share getShow)', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(500))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'boom' }) })

    // generateMetadata swallows the null and returns the fallback shape.
    expect(meta.title).toBe('Show')
    expect(Sentry.captureMessage).toHaveBeenCalledWith(
      'Show page: API returned 500',
      expect.objectContaining({ extra: { slug: 'boom', status: 500 } })
    )
  })
})
