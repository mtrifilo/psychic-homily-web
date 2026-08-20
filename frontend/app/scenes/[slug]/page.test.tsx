import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse } from '@/lib/seo/test-helpers'

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
  })
})
