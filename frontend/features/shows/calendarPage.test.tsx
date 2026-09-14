import { describe, it, expect, vi, beforeEach } from 'vitest'

const NOT_FOUND = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND)
  }),
}))

// The list is a client component with its own hooks; this file tests the
// SERVER decisions around it — which windows are documents, and what gets
// seeded — so it is stubbed to a marker.
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

import { ShowsCalendarContent } from './calendarPage'
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
  return { shows: [], total, limit: 50, offset: 0, year: 2026, month: 11, day: 0 }
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

describe('ShowsCalendarContent — which windows are documents', () => {
  it('renders a month the histogram carries', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: NOVEMBER,
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  /**
   * The histogram is the same source the strip links from and the
   * `shows_months` sitemap family is projected from, so the set announced, the
   * set that renders and the set the strip offers cannot drift apart.
   */
  it('404s a month the histogram does not carry', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: { year: 2199, month: 1 },
        searchParams: Promise.resolve({}),
      })
    ).rejects.toThrow(NOT_FOUND)
  })

  /**
   * Page-independent, which is what makes `?page=2` of a dead month a
   * not-found too. On a deep page the row read is skipped, so the window total
   * is unavailable and the histogram is the only thing that can answer.
   */
  it('404s a dead month on a deep page as well as on page 1', async () => {
    answerWith({ rows: null, months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: { year: 2199, month: 1 },
        searchParams: Promise.resolve({ page: '2' }),
      })
    ).rejects.toThrow(NOT_FOUND)
  })

  // The upcoming histogram is what makes the past-month rule fall out for free:
  // a month that has ended is simply not in it. No redirect, by decision.
  it('404s a past month, which the upcoming histogram never carries', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: { year: 2019, month: 6 },
        searchParams: Promise.resolve({}),
      })
    ).rejects.toThrow(NOT_FOUND)
  })

  it('404s a day inside a live month that has no shows of its own', async () => {
    answerWith({ rows: page(0), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: NOVEMBER_14,
        searchParams: Promise.resolve({}),
      })
    ).rejects.toThrow(NOT_FOUND)
  })

  it('renders a day that has shows', async () => {
    answerWith({ rows: page(4), months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: NOVEMBER_14,
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  /**
   * 404 only on a POSITIVE absence. A failed histogram read is not an answer,
   * and treating it as one turns a backend blip into a not-found body for every
   * month on the site — which the proxy deliberately does not do either.
   */
  it('renders rather than 404s when the histogram read failed', async () => {
    answerWith({ rows: page(68), months: null })

    await expect(
      ShowsCalendarContent({
        window: { year: 2199, month: 1 },
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })

  it('renders rather than 404s when the row read failed', async () => {
    answerWith({ rows: null, months: HISTOGRAM })

    await expect(
      ShowsCalendarContent({
        window: NOVEMBER,
        searchParams: Promise.resolve({}),
      })
    ).resolves.toBeTruthy()
  })
})

describe('ShowsCalendarContent — the first-screen seed', () => {
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

    const urls = fetchListPayload.mock.calls.map(([options]) => options.url)
    expect(urls.some((url: string) => url.includes('/shows/calendar'))).toBe(false)
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })

  // `ShowList` renders its skeleton while EITHER the rows or the cities are
  // loading, so seeding one without the other server-renders the skeleton.
  it('seeds nothing when the cities read failed', async () => {
    answerWith({ rows: page(68), months: HISTOGRAM, cities: null })

    await ShowsCalendarContent({
      window: NOVEMBER,
      searchParams: Promise.resolve({}),
    })

    expect(seedFirstScreen).not.toHaveBeenCalled()
  })
})
