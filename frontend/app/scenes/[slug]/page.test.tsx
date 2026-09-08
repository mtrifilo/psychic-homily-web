import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))

// Stub the heavy scene view so invoking generateMetadata does not pull the
// whole SceneDetail render path (and its map/graph deps) into this suite.
vi.mock('@/features/scenes/components/SceneDetail', () => ({
  SceneDetailView: (): null => null,
}))

// The week fetch feeds only the OG IMAGE descriptor. Returning null keeps
// these cases on the description, which is what this suite is about.
vi.mock('@/features/scenes/sceneWeekApi', () => ({
  fetchSceneWeek: vi.fn(async () => null),
}))

// The calendar slice is rendered here (PSY-1850) rather than inside
// SceneDetail. Stubbed for the same reason the view is: this suite is about
// what the ROUTE fetches and hands down, not about how rows are drawn.
vi.mock('@/features/scenes/components/SceneCalendar', () => ({
  SceneCalendar: (): null => null,
}))

import { JsonLd } from '@/components/seo/JsonLd'
import { countWindowShows } from '@/features/scenes/sceneWindow'
import { fetchSceneWeek } from '@/features/scenes/sceneWeekApi'
import ScenePage, { generateMetadata } from './page'

function buildScene(overrides: Record<string, unknown> = {}) {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
    tagline: null,
    stats: {
      venue_count: 12,
      artist_count: 85,
      upcoming_show_count: 45,
      festival_count: 0,
    },
    pulse: {
      shows_this_month: 30,
      shows_prev_month: 25,
      shows_trend: 5,
      new_artists_30d: 8,
      active_venues_this_month: 10,
      shows_by_month: [20, 22, 25, 28, 30, 30],
    },
    venues: [],
    ...overrides,
  }
}

const GENERATED_DESCRIPTION =
  'Upcoming shows, venues and local artists in the Phoenix, AZ music scene.'

const fetchMock = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// PSY-1848: the authored tagline doubles as the route's og:description.
describe('scenes/[slug] generateMetadata description', () => {
  it('falls back to the generated sentence when no tagline is authored', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.description).toBe(GENERATED_DESCRIPTION)
    expect(meta.openGraph?.description).toBe(GENERATED_DESCRIPTION)
  })

  it('uses the authored tagline for description, og:description and twitter', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildScene({ tagline: 'Where the desert learns to scream' }))
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.description).toBe('Where the desert learns to scream')
    expect(meta.openGraph?.description).toBe('Where the desert learns to scream')
    expect(meta.twitter?.description).toBe('Where the desert learns to scream')
  })

  // A blank tagline must not unfurl as an empty description — same
  // "trimmed-empty is absent" rule the page body applies.
  it('falls back when the tagline is whitespace only', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene({ tagline: '   ' })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.description).toBe(GENERATED_DESCRIPTION)
  })

  // `description` is prose the page deliberately does not render, and it is
  // not a fallback for the tagline anywhere — including in metadata.
  it('never uses description, even when the payload carries one', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildScene({ description: 'A long paragraph about the desert scene.' }))
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.description).toBe(GENERATED_DESCRIPTION)
  })

  // The week fetch survives on this route for exactly one reason: the card the
  // page advertises is the ARCHIVED week card, whose URL carries the week key.
  // Without this the fetch reads as dead weight, and dropping it would fall the
  // route back to its own rolling `opengraph-image`, whose URL never changes.
  it('advertises the archived week card, from the week fetch', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))
    vi.mocked(fetchSceneWeek).mockResolvedValueOnce({
      slug: 'phoenix-az',
      iso_week: '2026-W34',
    } as Awaited<ReturnType<typeof fetchSceneWeek>>)

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.openGraph?.images).toEqual([
      expect.objectContaining({
        url: 'https://psychichomily.com/scenes/phoenix-az/2026-W34/opengraph-image',
        alt: GENERATED_DESCRIPTION,
      }),
    ])
    // Omitted deliberately: Next copies the openGraph descriptor across when
    // Twitter has none, and a bare URL string here would drop the alt.
    expect(meta.twitter?.images).toBeUndefined()
  })

  it('still returns the not-found metadata for a missing scene', async () => {
    fetchMock.mockResolvedValueOnce({ ok: false, status: 404 })

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'nowhere-zz' }) })

    expect(meta.title).toBe('Scene not found')
  })
})

/**
 * PSY-1850: the root's calendar slice is fetched HERE, on the server, from the
 * day endpoint — the same endpoint `/scenes/{slug}/tonight` renders. It replaced
 * a 28-day / 61-row client fetch.
 */
describe('scenes/[slug] calendar slice', () => {
  function buildDay(overrides: Record<string, unknown> = {}) {
    return {
      slug: 'phoenix-az',
      scene_name: 'Phoenix, AZ',
      city: 'Phoenix',
      state: 'AZ',
      date: '2026-08-18',
      timezone: 'America/Phoenix',
      iso_week: '2026-W34',
      prev_date: '2026-08-17',
      next_date: '2026-08-19',
      is_tonight: true,
      is_past_day: false,
      show_count: 0,
      shows: [],
      tracked_venues: [],
      ...overrides,
    }
  }

  /** A show the JSON-LD can describe: it carries a venue and a real instant. */
  function buildShow(overrides: Record<string, unknown> = {}) {
    return {
      id: 1,
      title: '',
      event_date: '2026-08-18',
      starts_at: '2026-08-19T03:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
      slug: 'smooth-hands-valley-bar',
      venue_name: 'Valley Bar',
      venue_slug: 'valley-bar',
      venue_address: '130 N Central Ave',
      venue_city: 'Phoenix',
      venue_state: 'AZ',
      venue_country: 'US',
      venue_timezone: 'America/Phoenix',
      artist_names: ['Smooth Hands'],
      ...overrides,
    }
  }

  /** Every URL the route asked for, in order. */
  function fetchedUrls(): string[] {
    return fetchMock.mock.calls.map(call => String(call[0]))
  }

  type Node = { props?: Record<string, unknown> } | null | undefined

  /** The element handed to `SceneDetailView` as its `calendarSlot`. */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function findCalendarSlot(node: any): any {
    if (!node || typeof node !== 'object') return undefined
    if (Array.isArray(node)) {
      for (const child of node) {
        const found = findCalendarSlot(child)
        if (found) return found
      }
      return undefined
    }
    const props = (node as Node)?.props
    if (props && 'calendarSlot' in props) return props.calendarSlot
    return props ? findCalendarSlot(props.children) : undefined
  }

  /** Every query the route dehydrated into its `<HydrationBoundary>`. */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function dehydratedQueries(node: any): any[] {
    if (!node || typeof node !== 'object') return []
    if (Array.isArray(node)) return node.flatMap(dehydratedQueries)
    const props = (node as Node)?.props
    if (props && 'state' in props) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const queries = (props.state as any)?.queries
      if (Array.isArray(queries)) return queries
    }
    return props ? dehydratedQueries(props.children) : []
  }

  /** The query keys the route seeded into its `<HydrationBoundary>`. */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function dehydratedKeys(node: any): unknown[][] {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return dehydratedQueries(node).map((q: any) => q.queryKey)
  }

  /** The payload seeded under one dehydrated key, or undefined. */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function dehydratedData(node: any, key: unknown[]): unknown {
    const found = dehydratedQueries(node).find(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (q: any) => JSON.stringify(q.queryKey) === JSON.stringify(key)
    )
    return found?.state?.data
  }

  /** Every payload handed to a `<JsonLd>` in the returned tree. */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function findJsonLd(node: any): any[] {
    if (!node || typeof node !== 'object') return []
    if (Array.isArray(node)) return node.flatMap(findJsonLd)
    if (node.type === JsonLd) return [node.props.data]
    return node.props ? findJsonLd(node.props.children) : []
  }

  const CREWS = {
    crews: [{ slug: 'pleiades-series', name: 'Pleiades Series', show_count: 4 }],
  }

  /**
   * Answer by URL rather than by call order, so a case about the crews read
   * cannot be moved by a change in how many requests the slice makes.
   *
   * `crews` is what the crews URL answers with: a Response, or a rejection.
   */
  function stubByUrl(
    scene: Record<string, unknown>,
    crews: Response | Error = okResponse(CREWS)
  ) {
    fetchMock.mockImplementation(async (input: unknown) => {
      const url = String(input)
      if (url.endsWith('/crews')) {
        if (crews instanceof Error) throw crews
        return crews
      }
      if (/\/scenes\/[^/]+$/.test(url)) return okResponse(scene)
      return okResponse(buildDay())
    })
  }

  // The crews chip row sits INSIDE the header, so a client-only fetch would
  // insert it under painted content and push the calendar down. Read here, the
  // payload is seeded under the key the row subscribes to.
  it('seeds the crews row from the server read', async () => {
    stubByUrl(buildScene())

    const tree = await ScenePage({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(
      fetchedUrls().some(u => u.endsWith('/scenes/phoenix-az/crews'))
    ).toBe(true)
    expect(dehydratedData(tree, ['scenes', 'crews', 'phoenix-az'])).toEqual(CREWS)
  })

  // A member-city URL renders its metro's scene, and the row keys on the slug
  // the SCENE payload carries. Seeding the requested spelling would leave the
  // entry orphaned and the row unseeded on exactly those URLs.
  it('seeds the crews row under the canonical slug, not the requested one', async () => {
    stubByUrl(buildScene({ slug: 'phoenix-az' }))

    const tree = await ScenePage({ params: Promise.resolve({ slug: 'tempe-az' }) })

    expect(
      fetchedUrls().some(u => u.endsWith('/scenes/phoenix-az/crews'))
    ).toBe(true)
    expect(dehydratedData(tree, ['scenes', 'crews', 'phoenix-az'])).toEqual(CREWS)
    expect(dehydratedKeys(tree)).not.toContainEqual(['scenes', 'crews', 'tempe-az'])
  })

  // The slice is the page's substance and its chain is several requests deep,
  // so it is issued first. Pinned because the ordering is array order, which a
  // reorder would change silently.
  it('issues the slice chain before the crews read', async () => {
    stubByUrl(buildScene())

    await ScenePage({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    const urls = fetchedUrls()
    expect(urls.findIndex(u => u.includes('/day'))).toBeLessThan(
      urls.findIndex(u => u.endsWith('/crews'))
    )
  })

  // The row must degrade to its own client fetch, never hydrate an empty list
  // and never take the page down with it.
  //
  // A slug of its own per case: the server query client is a module singleton
  // under jsdom, so a key another case seeded would still be in this tree.
  it.each([
    ['a non-2xx answer', 'tucson-az', errorResponse(500)],
    ['a failed request', 'flagstaff-az', new Error('network down')],
  ])('seeds no crews entry on %s', async (_label, slug, crews) => {
    stubByUrl(buildScene({ slug }), crews as Response | Error)

    const tree = await ScenePage({ params: Promise.resolve({ slug }) })

    expect(tree).toBeTruthy()
    expect(dehydratedData(tree, ['scenes', 'crews', slug])).toBeUndefined()
    expect(dehydratedData(tree, ['scenes', 'detail', slug])).toBeTruthy()
  })

  it('reads tonight and the next full day from the day endpoint', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))
    fetchMock.mockResolvedValue(okResponse(buildDay()))

    await ScenePage({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    const urls = fetchedUrls()
    expect(urls.some(u => u.endsWith('/scenes/phoenix-az/day'))).toBe(true)
    expect(urls.some(u => u.endsWith('/scenes/phoenix-az/day/2026-08-19'))).toBe(true)
  })

  // The window this ticket removed. A root that still asked for it would be
  // paying for 28 days of rows it no longer draws.
  it('no longer asks for the 28-day window', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))
    fetchMock.mockResolvedValue(okResponse(buildDay()))

    await ScenePage({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(fetchedUrls().some(u => u.includes('/shows?'))).toBe(false)
    expect(fetchedUrls().some(u => u.includes('days=28'))).toBe(false)
  })

  // The far edge of the servable window. `fetchScenePeriod` resolves the
  // CURRENT period when its key is falsy, so an unguarded second request would
  // fetch tonight AGAIN and the slice would print the same night twice.
  it('makes no second request when there is no next date', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))
    fetchMock.mockResolvedValue(okResponse(buildDay({ next_date: '' })))

    await ScenePage({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    const dayUrls = fetchedUrls().filter(u => u.includes('/day'))
    expect(dayUrls).toEqual([expect.stringContaining('/scenes/phoenix-az/day')])
    expect(dayUrls.some(u => u.includes('/day/'))).toBe(false)
  })

  // A metro MEMBER slug resolves to its principal city, so `/scenes/mesa-az`
  // renders the Phoenix scene. The calendar builds every link off the scene it
  // is handed; handing it the requested spelling would mint a second URL for
  // pages that already have one. (This rule used to be covered in
  // SceneDetail.test.tsx, and moved here with the rendering.)
  it('hands the calendar the canonical scene, not the requested slug', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene({ slug: 'phoenix-az' })))
    fetchMock.mockResolvedValue(okResponse(buildDay()))

    const tree = await ScenePage({ params: Promise.resolve({ slug: 'mesa-az' }) })

    // Read the calendar's props off the returned element tree rather than
    // rendering it: the slot sits behind a client boundary, and the assertion
    // is about what the ROUTE passed down, not about what the DOM ended up
    // with. A blunt string scan would not do — `mesa-az` is legitimately still
    // present as the REQUESTED slug (SceneDetailView resolves it canonically
    // through its own query), and that is exactly the pair being distinguished.
    const slot = findCalendarSlot(tree)
    expect(slot?.props?.scene?.slug).toBe('phoenix-az')

    // The structured data resolves the same way, off the day payload's slug.
    // The trail names a location, so it names the scene the request landed on,
    // not the spelling that was typed. (`alternates.canonical` answers a
    // different question and still carries the requested spelling.)
    const breadcrumb = findJsonLd(tree).find(
      (data: { '@type'?: string }) => data['@type'] === 'BreadcrumbList'
    )
    const leaf = breadcrumb.itemListElement[breadcrumb.itemListElement.length - 1]
    expect(leaf.item).toBe('https://psychichomily.com/scenes/phoenix-az')
  })

  // The unit suite pins the builder; what this pins is the WIRING. The week
  // fetch is mocked to null for the whole file, so an ItemList reaching the
  // markup at all proves the slice is what feeds it, and the counts below are
  // taken from the one slice object both the markup and the calendar receive.
  it('describes exactly the shows the slice it hands the calendar holds', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))
    fetchMock.mockResolvedValueOnce(
      okResponse(
        buildDay({
          shows: [buildShow(), buildShow({ id: 2, slug: 'tournament-rebel-lounge' })],
        })
      )
    )
    // Every request after tonight's answers for the next day. The keyed leg
    // costs TWO of them: `fetchScenePeriod` probes the long window first, and a
    // date that has not happened yet is never frozen, so it always falls
    // through to the short one.
    fetchMock.mockResolvedValue(
      okResponse(
        buildDay({
          date: '2026-08-19',
          is_tonight: false,
          shows: [
            buildShow({
              id: 3,
              slug: 'holy-fawn-crescent',
              event_date: '2026-08-19',
              starts_at: '2026-08-20T03:00:00Z',
            }),
          ],
        })
      )
    )

    const tree = await ScenePage({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    const blocks = findJsonLd(tree)
    // Counted through the helper the calendar's own quiet check goes through,
    // so this figure is not a second spelling of it.
    const slicedShows = countWindowShows(findCalendarSlot(tree).props.slice.days)
    const itemList = blocks.find((data: { '@type'?: string }) => data['@type'] === 'ItemList')
    // FILTERED, not `find`: a second array-valued block would make a positional
    // pick silently assert about the wrong one.
    const eventBlocks = blocks.filter(Array.isArray)

    expect(slicedShows).toBe(3)
    expect(itemList?.numberOfItems).toBe(3)
    expect(eventBlocks).toHaveLength(1)
    expect(eventBlocks[0]).toHaveLength(3)
  })

})

// This route is the existence check the rest of the page trusts, so what it
// asks for and what it accepts back are both part of the guarantee.
describe('scenes/[slug] existence check', () => {
  /** The title the route emits when it has no scene to describe. */
  const NOT_FOUND_TITLE = 'Scene not found'

  // Next decodes route params before this runs, so an unescaped slug would walk
  // out of the path and hit a different backend endpoint.
  it('encodes the slug into the backend URL', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))

    await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az/../artists' }) })

    expect(String(fetchMock.mock.calls[0]?.[0])).toContain(
      '/scenes/phoenix-az%2F..%2Fartists'
    )
  })

  // A 200 is not proof of the right endpoint. A body missing any field the page
  // dereferences would render a scene page that is not about a scene.
  it.each([
    ['a body that is not an object', 'phoenix'],
    ['a null body', null],
    ['a body with no city', buildScene({ city: undefined })],
    ['a body with a blank city', buildScene({ city: '   ' })],
    ['a body with no state', buildScene({ state: undefined })],
    ['a body with no slug', buildScene({ slug: '' })],
    ['a body with no stats', buildScene({ stats: undefined })],
  ])('refuses %s', async (_label, body) => {
    fetchMock.mockResolvedValueOnce(okResponse(body))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.title).toBe(NOT_FOUND_TITLE)
  })

  // A malformed body used to reject PAST the fetch's catch, because the promise
  // was adopted after the try block exited.
  it('refuses a body that is not JSON at all', async () => {
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => {
        throw new SyntaxError('Unexpected token < in JSON')
      },
    })

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.title).toBe(NOT_FOUND_TITLE)
  })

  it('serves an ordinary scene', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.title).toBe('Phoenix, AZ Music Scene')
  })
})
