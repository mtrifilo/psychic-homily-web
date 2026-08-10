import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSeoList } = vi.hoisted(() => ({
  fetchSeoList: vi.fn(),
}))

vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))

import { getVenuesForMetadata } from './venuesMetadata'

// The fail-open behaviour and the shortfall report both belong to fetchSeoList
// and are tested against the real implementation in lib/seo/fetchSeoList.test.ts.
// What is venues-specific — and what would silently regress the JSON-LD if it
// drifted — is the wiring: which endpoint, which collection key, which Sentry tag.
describe('getVenuesForMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('asks fetchSeoList for the venues listing projection, not the full list', async () => {
    const venues = [{ name: 'The Rebel Lounge', slug: 'the-rebel-lounge' }]
    fetchSeoList.mockResolvedValue(venues)

    await expect(getVenuesForMetadata()).resolves.toEqual(venues)
    expect(fetchSeoList).toHaveBeenCalledWith({
      url: expect.stringMatching(/\/venues\/listing$/),
      collection: 'venues',
      service: 'venues-listing',
    })
  })

  // THE assertion this endpoint exists for (PSY-1764). The previous call was
  // `GET /venues?limit=100` against a set of 297, so the ItemList advertised a
  // third of the catalogue and nothing said so. A limit reappearing here is that
  // defect returning, and it would look healthy from every other angle: 200 OK,
  // well-formed JSON, an ItemList that renders.
  it('carries no limit, so the ItemList cannot be a truncated prefix', async () => {
    fetchSeoList.mockResolvedValue([])

    await getVenuesForMetadata()
    expect(fetchSeoList.mock.calls[0][0].url).not.toMatch(/[?&]limit=/)
  })

  // The shared budget, not a widened one: an override here would be a bandaid
  // for a payload that had grown too big, which is the condition the projection
  // exists to prevent rather than to tolerate.
  it('does not override the shared fetch budget', async () => {
    fetchSeoList.mockResolvedValue([])

    await getVenuesForMetadata()
    expect(fetchSeoList.mock.calls[0][0]).not.toHaveProperty('timeoutMs')
  })
})
