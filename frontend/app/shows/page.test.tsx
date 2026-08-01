import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchListPayload } = vi.hoisted(() => ({ fetchListPayload: vi.fn() }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

import { UPCOMING_SHOWS_LIMIT, getUpcomingShows } from './page'

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

  // Since PSY-1624 this number also sizes the server-rendered first screen,
  // and `UPCOMING_SHOWS_FIRST_SCREEN_KEY` encodes NO `limit` — so the
  // post-hydration revalidation asks for the endpoint's default. They agree
  // only while this IS that default (`GET /shows/upcoming`, `default:"50"`).
  // Raising it without changing what `ShowList` requests would server-render
  // rows that visibly collapse on hydration; this is the assertion that stops
  // that being a silent change.
  it('equals the endpoint default, which the seeded key relies on', () => {
    expect(UPCOMING_SHOWS_LIMIT).toBe(50)
  })

  it('asks for the shows collection with an explicit limit', async () => {
    const shows = [{ slug: 'a-show', title: 'A Show', artists: [], venues: [] }]
    fetchListPayload.mockResolvedValue({ shows, pagination: {}, total: 1 })

    await expect(getUpcomingShows()).resolves.toEqual(shows)
    expect(fetchListPayload).toHaveBeenCalledWith({
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
