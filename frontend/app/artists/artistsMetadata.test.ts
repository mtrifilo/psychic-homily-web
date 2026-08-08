import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSeoList } = vi.hoisted(() => ({
  fetchSeoList: vi.fn(),
}))

vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

import { getArtistsForMetadata } from './artistsMetadata'

// The fail-open behaviour itself belongs to fetchSeoList and is tested against
// the real implementation in lib/seo/fetchSeoList.test.ts. What is artists-
// specific — and what would silently regress the JSON-LD if it drifted — is the
// wiring: which endpoint, which collection key, which Sentry tag.
describe('getArtistsForMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // The endpoint is the assertion that matters (PSY-1674). Pointing this back
  // at `GET /artists` would restore a body 2x over the 2 MB cache-item cap, so
  // the entry stops being written: one console.warn buried in the build log,
  // nothing failing, an ItemList that still renders correctly, and every render
  // re-pulling the whole catalogue from origin. The build gate in
  // lib/data-cache-budget is the backstop; this test is the cheap first line.
  it('asks fetchSeoList for the artists listing projection, not the full list', async () => {
    const artists = [{ name: 'Desert Static', slug: 'desert-static' }]
    fetchSeoList.mockResolvedValue(artists)

    await expect(getArtistsForMetadata()).resolves.toEqual(artists)
    expect(fetchSeoList).toHaveBeenCalledWith({
      url: expect.stringMatching(/\/artists\/listing$/),
      collection: 'artists',
      service: 'artists-listing',
    })
  })

  // No timeoutMs: the 30s override was a bandaid for the oversized payload and
  // its reason is gone, so the call must fall through to the shared default
  // rather than carry a stale widened budget.
  it('does not override the shared fetch budget', async () => {
    fetchSeoList.mockResolvedValue([])

    await getArtistsForMetadata()
    expect(fetchSeoList.mock.calls[0][0]).not.toHaveProperty('timeoutMs')
  })
})
