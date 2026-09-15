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
  it('renders a month that has shows', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: NOVEMBER,
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  it('renders a month with nothing in it', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: NOVEMBER,
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  it('renders a day with nothing in it', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: NOVEMBER_14,
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  it('renders a run with nothing in it', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: { ...NOVEMBER_14, days: 3 },
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  /**
   * A month the histogram does not carry is no longer the component's question.
   * The histogram bounds the month AXIS; the span bounds the URL space, and a
   * quiet month inside the span is exactly the page this route now serves.
   */
  it('renders a month the histogram does not carry', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: { year: 2199, month: 1 },
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  it('renders when a read failed', async () => {
    answerWith({ rows: null, months: null })

    await expect(
      ShowsCalendarContent({
        window: NOVEMBER,
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  /**
   * A deep page skips the row read: a seed attaches to whatever key is current,
   * so seeding page 2 with page 1's slice would look like a cache hit and never
   * correct itself.
   */
  it('seeds page 1 and skips the row read on a deep page', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM })

    await ShowsCalendarContent({
      window: NOVEMBER,
      searchParams: Promise.resolve({}),
    })
    const firstScreenUrl = showsCalendarWindowFirstScreenUrl(NOVEMBER)
    expect(
      fetchListPayload.mock.calls.some(
        (call: unknown[]) => (call[0] as { url: string }).url === firstScreenUrl
      )
    ).toBe(true)
    expect(seedFirstScreen).toHaveBeenCalledWith(
      expect.arrayContaining([
        expect.objectContaining({
          queryKey: showsCalendarWindowFirstScreenKey(NOVEMBER),
        }),
      ])
    )

    vi.clearAllMocks()
    seedFirstScreen.mockResolvedValue({ queries: [] })
    answerWith({ rows: page(68), months: HISTOGRAM })

    await ShowsCalendarContent({
      window: NOVEMBER,
      searchParams: Promise.resolve({ page: '2' }),
    })
    expect(
      fetchListPayload.mock.calls.some(
        (call: unknown[]) => (call[0] as { url: string }).url === firstScreenUrl
      )
    ).toBe(false)
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
  })

  /** Segments that cannot be a window need no read at all. */
  it('answers the not-found head without reading anything', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    const metadata = await showsCalendarRouteMetadata(null)

    expect(metadata.robots).toEqual({ index: false, follow: false })
    expect(fetchListPayload).not.toHaveBeenCalled()
  })
})
