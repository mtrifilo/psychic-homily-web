import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSeoList } = vi.hoisted(() => ({
  fetchSeoList: vi.fn(),
}))

vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

import { getArtistsForMetadata } from './artistsMetadata'

// The fail-open behaviour itself belongs to fetchSeoList and is tested against
// the real implementation in lib/seo/fetchSeoList.test.ts. What is artists-
// specific — and what would silently regress the JSON-LD if it drifted — is the
// wiring: which endpoint, which collection key, which Sentry tag, and the
// deliberately widened budget for an unpaginated response.
describe('getArtistsForMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('asks fetchSeoList for the artists collection on a thirty-second budget', async () => {
    const artists = [{ name: 'Desert Static', slug: 'desert-static' }]
    fetchSeoList.mockResolvedValue(artists)

    await expect(getArtistsForMetadata()).resolves.toEqual(artists)
    expect(fetchSeoList).toHaveBeenCalledWith({
      url: expect.stringMatching(/\/artists$/),
      collection: 'artists',
      service: 'artists-listing',
      timeoutMs: 30_000,
    })
  })
})
