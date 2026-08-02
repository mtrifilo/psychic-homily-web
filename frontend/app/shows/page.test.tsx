import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

import { UPCOMING_SHOWS_LIMIT, getScenesForWeekIndex, getUpcomingShows } from './page'

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
      url: expect.stringMatching(
        new RegExp(
          `/shows/upcoming\\?limit=${UPCOMING_SHOWS_LIMIT}&timezone=America%2FLos_Angeles$`
        )
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
    const scenes = [{ slug: 'phoenix-az', shows_this_week: 28 }]
    fetchListPayload.mockResolvedValue({ scenes, count: 1 })

    await expect(getScenesForWeekIndex()).resolves.toEqual({ scenes, count: 1 })
    expect(fetchListPayload).toHaveBeenCalledWith({
      url: expect.stringMatching(/\/scenes$/),
      collection: 'scenes',
      service: 'shows-this-week-by-city',
    })
  })

  // A failed fetch drops the block rather than throwing: it is supplementary to
  // the list above it, and an API blip should not become an error page.
  it('passes the failure through as null for the caller to drop', async () => {
    fetchListPayload.mockResolvedValue(null)

    await expect(getScenesForWeekIndex()).resolves.toBeNull()
  })
})
