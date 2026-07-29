import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSeoList } = vi.hoisted(() => ({ fetchSeoList: vi.fn() }))
vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

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

  it('asks fetchSeoList for the shows collection with an explicit limit', async () => {
    const shows = [{ slug: 'a-show', title: 'A Show', artists: [], venues: [] }]
    fetchSeoList.mockResolvedValue(shows)

    await expect(getUpcomingShows()).resolves.toEqual(shows)
    expect(fetchSeoList).toHaveBeenCalledWith({
      url: expect.stringMatching(
        new RegExp(`/shows/upcoming\\?limit=${UPCOMING_SHOWS_LIMIT}$`)
      ),
      collection: 'shows',
      service: 'shows-listing',
    })
  })
})
