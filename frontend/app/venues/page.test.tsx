import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSeoList } = vi.hoisted(() => ({ fetchSeoList: vi.fn() }))
vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

import { VENUE_LIST_LIMIT, getVenues } from './page'

// This is the value that silently broke production: the page asked for
// `limit=200`, `GET /venues` declares `maximum:"100"`, huma rejected it with a
// 422 before the handler ran, and the fail-open turned that into a missing
// JSON-LD ItemList on every render for months. Nothing failed. The bound is
// asserted here so raising it past the backend maximum breaks a test instead.
describe('getVenues', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('stays within the endpoint maximum of 100', () => {
    expect(VENUE_LIST_LIMIT).toBeLessThanOrEqual(100)
  })

  it('asks fetchSeoList for the venues collection within that limit', async () => {
    const venues = [{ slug: 'the-rebel-lounge', name: 'The Rebel Lounge' }]
    fetchSeoList.mockResolvedValue(venues)

    await expect(getVenues()).resolves.toEqual(venues)
    expect(fetchSeoList).toHaveBeenCalledWith({
      url: expect.stringMatching(/\/venues\?limit=100$/),
      collection: 'venues',
      service: 'venues-listing',
    })
  })
})
