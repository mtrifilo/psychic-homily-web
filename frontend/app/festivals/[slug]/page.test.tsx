import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { errorResponse, jsonBodyResponse } from '@/lib/seo/test-helpers'

// Stub the festival detail module so invoking generateMetadata does not pull
// in the client render path.
vi.mock('@/features/festivals/components', () => ({
  FestivalDetail: (): null => null,
}))

import { generateMetadata } from './page'

function buildFestival(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: 'Riot Fest 2026',
    slug: 'riot-fest-2026',
    series_slug: 'riot-fest',
    edition_year: 2026,
    description: null,
    location_name: 'Douglas Park',
    city: 'Chicago',
    state: 'IL',
    country: 'US',
    start_date: '2026-09-18',
    end_date: '2026-09-20',
    website: null,
    ticket_url: null,
    flyer_url: null,
    status: 'announced',
    social: null,
    artist_count: 102,
    venue_count: 1,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    ...overrides,
  }
}

const fetchMock = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

async function metadataFor(slug = 'riot-fest-2026') {
  return generateMetadata({ params: Promise.resolve({ slug }) })
}

describe('festivals/[slug] generateMetadata', () => {
  it('builds the description from the festival and puts it on the share card too', async () => {
    fetchMock.mockResolvedValueOnce(jsonBodyResponse(buildFestival()))
    const meta = await metadataFor()
    expect(meta.description).toBe(
      'Riot Fest 2026: Sep 18 to 20, 2026, Douglas Park, Chicago, IL. Lineup: 102 artists.'
    )
    expect(meta.openGraph?.description).toBe(meta.description)
  })

  it('keeps the title as it was', async () => {
    fetchMock.mockResolvedValueOnce(jsonBodyResponse(buildFestival()))
    const meta = await metadataFor()
    expect(meta.title).toBe('Riot Fest 2026 - Chicago, IL')
    expect(meta.openGraph?.title).toBe('Riot Fest 2026 - Chicago, IL')
  })

  it('marks a cancelled festival and states more than one venue', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonBodyResponse(buildFestival({ status: 'cancelled', venue_count: 4 })),
    )
    expect((await metadataFor()).description).toBe(
      'Riot Fest 2026 (cancelled): Sep 18 to 20, 2026, Douglas Park, Chicago, IL. Lineup: 102 artists. Venues: 4.'
    )
  })

  it('reads nothing but the festival', async () => {
    fetchMock.mockResolvedValueOnce(jsonBodyResponse(buildFestival()))
    await metadataFor()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('falls back to the "Festival" title when the festival is missing', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))
    const meta = await metadataFor('missing')
    expect(meta.title).toBe('Festival')
    expect(meta.description).toBe('View festival details')
  })
})
