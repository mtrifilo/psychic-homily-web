import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

import {
  UPCOMING_SHOWS_LIMIT,
  getScenesForWeekIndex,
  getShowsMonthsPayload,
  getUpcomingShows,
} from './page'
import { showsFirstScreenSeeds } from '@/features/shows/firstScreen'
import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOWS_CALENDAR_FIRST_SCREEN_KEY,
  SHOWS_MONTHS_FIRST_SCREEN_KEY,
} from '@/features/shows/api'

// The bound here was implicit — no `limit` was sent, so the endpoint's
// `default:"50"` applied silently. Asserting it keeps the number a decision
// rather than a default nobody has looked at.
describe('getUpcomingShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('stays within the endpoint maximum of 200', () => {
    expect(UPCOMING_SHOWS_LIMIT).toBeLessThanOrEqual(200)
  })

  // Deliberately NOT asserted equal to the endpoint default. The first screen
  // is fetched separately from this ItemList (see `HydratedShowList`), so this
  // number is free to move without dragging the server-rendered list with it.
  // That independence is the point of the split; pinning it to 50 here would
  // quietly recreate the coupling it removed.

  it('asks for the shows collection with an explicit limit', async () => {
    const shows = [{ slug: 'a-show', title: 'A Show', artists: [], venues: [] }]
    fetchListPayload.mockResolvedValue({ shows, pagination: {}, total: 1 })

    await expect(getUpcomingShows()).resolves.toEqual(shows)
    expect(fetchListPayload).toHaveBeenCalledWith({
      // Anchored, and `?limit=` not `&limit=`: the first-screen URL this is
      // built from is the BARE endpoint, so `&` here would produce a query
      // string with no `?` and silently drop the bound. The anchor also keeps
      // any other param — a viewer timezone above all — from creeping back in.
      url: expect.stringMatching(
        new RegExp(`/shows/upcoming\\?limit=${UPCOMING_SHOWS_LIMIT}$`)
      ),
      collection: 'shows',
      service: 'shows-listing',
      // The BUILD budget, not the helper's request-time default: this one call
      // also feeds the ItemList, which is rendered in the static-shell
      // prerender. Giving up at 2.5s there ships a page with no schema block.
      timeoutMs: 10_000,
    })
  })

  // Since PSY-1624 the ItemList and the hydration seed read ONE response, so
  // this helper has to absorb the fetch failure: `null` means no rows, and the
  // caller drops the schema block on length rather than emitting an empty
  // `ItemList` that asserts the catalogue is empty.
  it('yields no rows when the fetch failed', async () => {
    fetchListPayload.mockResolvedValue(null)

    await expect(getUpcomingShows()).resolves.toEqual([])
  })
})

// PSY-1623: the by-city block is the only inbound link `/shows` gives the
// scene-week pages. `HydratedThisWeekByCity` is a server component and cannot
// be called from here, so the fetch it delegates to is what this pins: the
// endpoint, the collection guard, and the Sentry tag that would name it in an
// outage.
describe('getScenesForWeekIndex', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('asks for the whole scene list, guarded on the scenes collection', async () => {
    const scenes = [{ slug: 'phoenix-az', shows_calendar_week: 22 }]
    fetchListPayload.mockResolvedValue({ scenes, count: 1 })

    await expect(getScenesForWeekIndex()).resolves.toEqual({ scenes, count: 1 })
    expect(fetchListPayload).toHaveBeenCalledWith({
      url: expect.stringMatching(/\/scenes$/),
      collection: 'scenes',
      service: 'shows-this-week-by-city',
      revalidateSeconds: expect.any(Number),
    })
  })

  // The counts in this payload describe a CALENDAR WEEK, so a stale entry does
  // not read as slightly old at the Monday rollover — it reads as the wrong
  // week, beside a heading naming the right one. The default hour is therefore
  // wrong here on purpose, and asserting the direction (rather than the literal
  // 60) keeps the test about the reason instead of the number.
  it('holds the payload far shorter than the default first-screen hour', async () => {
    fetchListPayload.mockResolvedValue({ scenes: [], count: 0 })

    await getScenesForWeekIndex()

    const { revalidateSeconds } = fetchListPayload.mock.calls[0][0]
    expect(revalidateSeconds).toBeLessThanOrEqual(60)
    expect(revalidateSeconds).toBeGreaterThan(0)
  })

  // A failed fetch drops the block rather than throwing: it is supplementary to
  // the list above it, and an API blip should not become an error page.
  it('passes the failure through as null for the caller to drop', async () => {
    fetchListPayload.mockResolvedValue(null)

    await expect(getScenesForWeekIndex()).resolves.toBeNull()
  })
})

// The month histogram is the pager's page labels. It is fetched server-side
// because it is otherwise a third client call on this route against a per-IP
// budget, and it is NOT a gate on the first paint.
describe('getShowsMonthsPayload', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('asks for the histogram, guarded on the months collection', async () => {
    const months = [{ year: 2026, month: 9, count: 64 }]
    fetchListPayload.mockResolvedValue({ months, total: 64 })

    await expect(getShowsMonthsPayload()).resolves.toEqual({ months, total: 64 })
    expect(fetchListPayload).toHaveBeenCalledWith({
      url: expect.stringMatching(/\/shows\/months$/),
      collection: 'months',
      service: 'shows-months-first-screen',
    })
  })

  // Takes the default window ON PURPOSE. Overriding it to the 60s its
  // calendar-scoped payload argues for pulled the whole ROUTE's revalidate from
  // 1h to 1m on a measured build. Pinned so that trade is re-decided
  // deliberately rather than re-made by an edit.
  it('does not override the first-screen revalidate window', async () => {
    fetchListPayload.mockResolvedValue({ months: [], total: 0 })

    await getShowsMonthsPayload()

    expect(fetchListPayload.mock.calls[0][0]).not.toHaveProperty(
      'revalidateSeconds'
    )
  })
})

describe('showsFirstScreenSeeds', () => {
  const shows = {
    shows: [],
    total: 0,
    limit: 50,
    offset: 0,
    year: 0,
    month: 0,
    day: 0,
    days: 0,
  }
  const cities = { cities: [] }
  const months = { months: [], total: 0 }

  it('seeds all three when all three landed', () => {
    const seeds = showsFirstScreenSeeds({ shows, cities, months, calendarKey: SHOWS_CALENDAR_FIRST_SCREEN_KEY, citiesKey: SHOW_CITIES_FIRST_SCREEN_KEY })

    expect(seeds?.map(seed => seed.queryKey)).toEqual([
      SHOWS_CALENDAR_FIRST_SCREEN_KEY,
      SHOW_CITIES_FIRST_SCREEN_KEY,
      SHOWS_MONTHS_FIRST_SCREEN_KEY,
    ])
  })

  // The histogram is not a gate: without it the pager renders bare numerals,
  // which is a far smaller loss than server-rendering the skeleton.
  it('still seeds the rows and cities when the histogram failed', () => {
    const seeds = showsFirstScreenSeeds({ shows, cities, months: null, calendarKey: SHOWS_CALENDAR_FIRST_SCREEN_KEY, citiesKey: SHOW_CITIES_FIRST_SCREEN_KEY })

    expect(seeds?.map(seed => seed.queryKey)).toEqual([
      SHOWS_CALENDAR_FIRST_SCREEN_KEY,
      SHOW_CITIES_FIRST_SCREEN_KEY,
    ])
  })

  // Both of these ARE gates: `ShowList` renders its skeleton while either query
  // is loading, so seeding one alone server-renders the skeleton.
  it('seeds nothing when the rows failed', () => {
    expect(showsFirstScreenSeeds({ shows: null, cities, months, calendarKey: SHOWS_CALENDAR_FIRST_SCREEN_KEY, citiesKey: SHOW_CITIES_FIRST_SCREEN_KEY })).toBeNull()
  })

  it('seeds nothing when the cities failed', () => {
    expect(showsFirstScreenSeeds({ shows, cities: null, months, calendarKey: SHOWS_CALENDAR_FIRST_SCREEN_KEY, citiesKey: SHOW_CITIES_FIRST_SCREEN_KEY })).toBeNull()
  })
})
