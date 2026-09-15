import { describe, it, expect, vi, beforeEach } from 'vitest'

// The list is a client component with its own hooks; this file tests the
// SERVER decisions around it, what renders and what the head says about it,
// so it is stubbed to a marker.
vi.mock('./components/ShowList', () => ({
  ShowList: (): null => null,
}))

type Seed = { queryKey: readonly unknown[]; data: unknown }
const seedFirstScreen = vi.fn(async (seeds: Seed[]) => ({ queries: seeds }))
vi.mock('@/lib/query-hydration', () => ({
  seedFirstScreen: (seeds: Seed[]) => seedFirstScreen(seeds),
}))

const fetchListPayload = vi.fn()
vi.mock('@/lib/ssr/fetchListPayload', () => ({
  fetchListPayload: (options: { url: string }) => fetchListPayload(options),
}))

import {
  buildShowsCalendarMetadata,
  ShowsCalendarContent,
  showsCalendarRouteMetadata,
} from './calendarPage'
import {
  showsCalendarWindowFirstScreenKey,
  showsCalendarWindowFirstScreenUrl,
} from './api'

const NOVEMBER = { year: 2026, month: 11 }
const NOVEMBER_14 = { year: 2026, month: 11, day: 14 }

const HISTOGRAM = {
  months: [
    { year: 2026, month: 10, count: 112 },
    { year: 2026, month: 11, count: 68 },
  ],
  total: 180,
}

function page(total: number) {
  return {
    shows: [],
    total,
    limit: 50,
    offset: 0,
    year: 2026,
    month: 11,
    day: 0,
    days: 0,
  }
}

/**
 * The three reads, answered by URL rather than by call order: they run inside
 * one `Promise.all`, so an order-keyed stub would pin an implementation detail
 * the component is free to change.
 */
function answerWith({
  rows,
  months,
  cities = { cities: [] },
}: {
  rows: unknown
  months: unknown
  cities?: unknown
}) {
  fetchListPayload.mockImplementation(async ({ url }: { url: string }) => {
    if (url.includes('/shows/calendar')) return rows
    if (url.includes('/shows/months')) return months
    return cities
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  seedFirstScreen.mockResolvedValue({ queries: [] })
})

/**
 * EVERY window that reaches this component is a page.
 *
 * Whether the URL exists at all is decided in `proxy.ts` against the
 * addressable span, before the render starts and while a status can still be
 * set. What is left here cannot answer a not-found without committing a 404
 * body at HTTP 200, so it does not try: a window with nothing in it renders the
 * list's quiet state, which keeps the chips, the month axis and the filters
 * that are the way out of it.
 */
describe('ShowsCalendarContent renders every window it is given', () => {
  it.each([
    ['a month that has shows', NOVEMBER, page(68), HISTOGRAM],
    ['a month with nothing in it', NOVEMBER, page(0), HISTOGRAM],
    ['a day with nothing in it', NOVEMBER_14, page(0), HISTOGRAM],
    ['a run with nothing in it', { ...NOVEMBER_14, days: 3 }, page(0), HISTOGRAM],
    // The histogram bounds the month AXIS; the addressable span bounds the URL
    // space, and a month the histogram does not carry is one of the quiet
    // pages this route now serves rather than a question for this component.
    ['a month the histogram does not carry', { year: 2199, month: 1 }, page(0), HISTOGRAM],
    ['a window whose reads failed', NOVEMBER, null, null],
  ])('renders %s', async (_label, window, rows, months) => {
    answerWith({ rows, months })

    await expect(
      ShowsCalendarContent({
        window,
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

})

describe('ShowsCalendarContent, the first-screen seed', () => {
  it('reads page 1 of the window at the URL the hook asks for', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM })

    await ShowsCalendarContent({
      window: NOVEMBER,
      searchParams: Promise.resolve({}),
    })

    expect(fetchListPayload).toHaveBeenCalledWith(
      expect.objectContaining({
        url: showsCalendarWindowFirstScreenUrl(NOVEMBER),
      })
    )
  })

  it('seeds the rows onto the WINDOW entry, never the root list one', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM })

    await ShowsCalendarContent({
      window: NOVEMBER,
      searchParams: Promise.resolve({}),
    })

    const seeds = seedFirstScreen.mock.calls[0][0]
    expect(seeds[0].queryKey).toEqual(showsCalendarWindowFirstScreenKey(NOVEMBER))
  })

  /**
   * A seed attaches to whatever key is current, so seeding page 2 with page 1's
   * slice would look like a cache hit and never correct itself.
   */
  it('reads no rows at all on a deep page', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM })

    await ShowsCalendarContent({
      window: NOVEMBER,
      searchParams: Promise.resolve({ page: '3' }),
    })

    const urls = fetchListPayload.mock.calls.map(
      (call: unknown[]) => (call[0] as { url: string }).url
    )
    expect(urls.some((url: string) => url.includes('/shows/calendar'))).toBe(false)
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })

  // `ShowList` renders its skeleton while EITHER the rows or the cities are
  // loading, so seeding one without the other server-renders the skeleton.
  // A run and its anchor day are different sets of rows, so the seed has to
  // land on the run's own entry or the page renders the skeleton and refetches.
  it('seeds a run onto the run s entry, not the day s', async () => {
    const run = { ...NOVEMBER_14, days: 3 }
    answerWith({ rows: page(9), months: HISTOGRAM })

    await ShowsCalendarContent({
      window: run,
      searchParams: Promise.resolve({}),
    })

    expect(fetchListPayload).toHaveBeenCalledWith(
      expect.objectContaining({ url: showsCalendarWindowFirstScreenUrl(run) })
    )
    const seeds = seedFirstScreen.mock.calls[0][0]
    expect(seeds[0].queryKey).toEqual(showsCalendarWindowFirstScreenKey(run))
    expect(seeds[0].queryKey).not.toEqual(
      showsCalendarWindowFirstScreenKey(NOVEMBER_14)
    )
  })

  it('seeds nothing when the cities read failed', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM, cities: null })

    await ShowsCalendarContent({
      window: NOVEMBER,
      searchParams: Promise.resolve({}),
    })

    expect(seedFirstScreen).not.toHaveBeenCalled()
  })
})

/**
 * The head is where an empty window is kept out of the index. A quiet page is
 * real, worth serving and worth linking out of; it is not worth an index entry,
 * and `follow` stays on because the way out of it is the links on it.
 */
describe('buildShowsCalendarMetadata, which windows are indexable', () => {
  it('leaves a window with shows indexable', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM })

    const metadata = await buildShowsCalendarMetadata(NOVEMBER)

    expect(metadata.robots).toBeUndefined()
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/11'
    )
  })

  it.each([
    ['a month', NOVEMBER],
    ['a day', NOVEMBER_14],
  ])('noindexes %s with nothing in it', async (_label, window) => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    const metadata = await buildShowsCalendarMetadata(window)

    expect(metadata.robots).toEqual({ index: false, follow: true })
  })

  /**
   * Suppression on a POSITIVE zero only. A read that failed is not an answer,
   * and treating it as one would noindex every window on the site during a
   * backend blip.
   */
  it('leaves a window indexable when the read failed', async () => {
    answerWith({ rows: null, months: HISTOGRAM })

    const metadata = await buildShowsCalendarMetadata(NOVEMBER)

    expect(metadata.robots).toBeUndefined()
  })

  /** A run is chrome, so it is noindexed whether or not it has rows. */
  it('noindexes a run that has shows', async () => {
    answerWith({ rows: page(9), months: HISTOGRAM })

    const metadata = await buildShowsCalendarMetadata({
      ...NOVEMBER_14,
      days: 3,
    })

    expect(metadata.robots).toEqual({ index: false, follow: true })
    // Canonical to the anchor day, run parameter dropped.
    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/shows/2026/11/14'
    )
    // A run is suppressed on its shape, so the head reads nothing for one.
    expect(fetchListPayload).not.toHaveBeenCalled()
  })

  /** Segments that cannot be a window need no read at all. */
  it('answers the not-found head without reading anything', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    const metadata = await showsCalendarRouteMetadata(null)

    expect(metadata.robots).toEqual({ index: false, follow: false })
    expect(fetchListPayload).not.toHaveBeenCalled()
  })
})
