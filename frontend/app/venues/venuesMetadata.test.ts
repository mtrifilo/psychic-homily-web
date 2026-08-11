import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSeoList, captureMessage } = vi.hoisted(() => ({
  fetchSeoList: vi.fn(),
  captureMessage: vi.fn(),
}))

vi.mock('@/lib/seo/fetchSeoList', () => ({ fetchSeoList }))
vi.mock('@sentry/nextjs', () => ({ captureMessage }))

import { getVenuesForMetadata } from './venuesMetadata'

// The fail-open behaviour and the shortfall report both belong to fetchSeoList
// and are tested against the real implementation in lib/seo/fetchSeoList.test.ts.
// What is venues-specific — and what would silently regress the JSON-LD if it
// drifted — is the wiring and the slug guard.
describe('getVenuesForMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('asks fetchSeoList for the venues listing projection, not the full list', async () => {
    const venues = [{ name: 'The Rebel Lounge', slug: 'the-rebel-lounge' }]
    fetchSeoList.mockResolvedValue(venues)

    await expect(getVenuesForMetadata()).resolves.toEqual(venues)
    // Deliberately not anchored, and deliberately a whole-object match: the
    // anchor would subsume the no-limit assertion below, and the whole-object
    // equality is what pins that no `timeoutMs` override creeps back in — the
    // 30s one on the artists twin was a bandaid for an oversized payload, which
    // is the condition a projection exists to prevent rather than tolerate.
    expect(fetchSeoList).toHaveBeenCalledWith({
      url: expect.stringContaining('/venues/listing'),
      collection: 'venues',
      service: 'venues-listing',
    })
  })

  // THE assertion this endpoint exists for (PSY-1764). The previous call asked
  // for a page of a set several times larger, so the ItemList advertised a
  // fraction of the catalogue and nothing said so. A limit reappearing here is
  // that defect returning, and it would look healthy from every other angle:
  // 200 OK, well-formed JSON, an ItemList that renders.
  it('carries no limit, so the ItemList cannot be a truncated prefix', async () => {
    fetchSeoList.mockResolvedValue([])

    await getVenuesForMetadata()
    expect(fetchSeoList.mock.calls[0][0].url).not.toMatch(/[?&]limit=/)
  })

  // The boundary check the endpoint's contract is supposed to make unnecessary.
  // If it ever fires, the ItemList would otherwise advertise
  // `https://psychichomily.com/venues/` — and `total`/`count` would both have
  // counted that row, so the shortfall report cannot see it.
  it('drops an entry with no slug and reports it', async () => {
    fetchSeoList.mockResolvedValue([
      { name: 'Linkable', slug: 'linkable' },
      { name: 'Unlinkable', slug: '' },
    ])

    await expect(getVenuesForMetadata()).resolves.toEqual([
      { name: 'Linkable', slug: 'linkable' },
    ])
    expect(captureMessage).toHaveBeenCalledWith(
      expect.stringContaining('entry without a slug reached the ItemList'),
      expect.objectContaining({
        level: 'error',
        tags: { service: 'venues-listing' },
        extra: expect.objectContaining({ received: 2, linkable: 1 }),
      })
    )
  })

  it('says nothing when every entry is linkable', async () => {
    fetchSeoList.mockResolvedValue([{ name: 'Linkable', slug: 'linkable' }])

    await getVenuesForMetadata()
    expect(captureMessage).not.toHaveBeenCalled()
  })
})
