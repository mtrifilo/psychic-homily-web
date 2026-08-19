import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse } from '@/lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))

// Stub the heavy scene view so invoking generateMetadata does not pull the
// whole SceneDetail render path (and its map/graph deps) into this suite.
vi.mock('@/features/scenes/components/SceneDetail', () => ({
  SceneDetailView: (): null => null,
}))

// The week fetch feeds only the OG IMAGE descriptor. Returning null keeps
// these cases on the description, which is what this suite is about.
vi.mock('@/features/scenes/sceneWeekApi', () => ({
  fetchSceneWeek: vi.fn(async () => null),
}))

import { generateMetadata } from './page'

function buildScene(overrides: Record<string, unknown> = {}) {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
    tagline: null,
    stats: {
      venue_count: 12,
      artist_count: 85,
      upcoming_show_count: 45,
      festival_count: 0,
    },
    pulse: {
      shows_this_month: 30,
      shows_prev_month: 25,
      shows_trend: 5,
      new_artists_30d: 8,
      active_venues_this_month: 10,
      shows_by_month: [20, 22, 25, 28, 30, 30],
    },
    venues: [],
    ...overrides,
  }
}

const GENERATED_DESCRIPTION =
  'Upcoming shows, venues and local artists in the Phoenix, AZ music scene.'

const fetchMock = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// PSY-1848: the authored tagline doubles as the route's og:description.
describe('scenes/[slug] generateMetadata description', () => {
  it('falls back to the generated sentence when no tagline is authored', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene()))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.description).toBe(GENERATED_DESCRIPTION)
    expect(meta.openGraph?.description).toBe(GENERATED_DESCRIPTION)
  })

  it('uses the authored tagline for description, og:description and twitter', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildScene({ tagline: 'Where the desert learns to scream' }))
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.description).toBe('Where the desert learns to scream')
    expect(meta.openGraph?.description).toBe('Where the desert learns to scream')
    expect(meta.twitter?.description).toBe('Where the desert learns to scream')
  })

  // A blank tagline must not unfurl as an empty description — same
  // "trimmed-empty is absent" rule the page body applies.
  it('falls back when the tagline is whitespace only', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildScene({ tagline: '   ' })))

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.description).toBe(GENERATED_DESCRIPTION)
  })

  // `description` is prose the page deliberately does not render, and it is
  // not a fallback for the tagline anywhere — including in metadata.
  it('never uses description, even when the payload carries one', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse(buildScene({ description: 'A long paragraph about the desert scene.' }))
    )

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'phoenix-az' }) })

    expect(meta.description).toBe(GENERATED_DESCRIPTION)
  })

  it('still returns the not-found metadata for a missing scene', async () => {
    fetchMock.mockResolvedValueOnce({ ok: false, status: 404 })

    const meta = await generateMetadata({ params: Promise.resolve({ slug: 'nowhere-zz' }) })

    expect(meta.title).toBe('Scene not found')
  })
})
