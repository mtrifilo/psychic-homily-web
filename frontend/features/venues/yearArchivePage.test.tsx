import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

// The archive body renders the real client archive; these tests only exercise
// which READS the server makes, so the render path is stubbed out the way the
// route's own test does.
vi.mock('./components/VenuePastShows', () => ({
  VenuePastShows: (): null => null,
  VENUE_PAST_SHOWS_ANCHOR: 'venue-past-shows',
}))

import { VenueYearArchiveShows } from './yearArchivePage'
import type { Venue } from './types'

const venue = {
  id: 7,
  name: 'The Rebel Lounge',
  slug: 'the-rebel-lounge',
  city: 'Phoenix',
  state: 'AZ',
} as unknown as Venue

const years = { venue_id: 7, time_filter: 'past', years: [{ year: 2025, count: 161 }] }

const fetchMock = vi.fn()

/** Every URL the component fetched, in order. */
function fetchedUrls(): string[] {
  return fetchMock.mock.calls.map(call => String(call[0]))
}

/**
 * Render the rows block for one URL's worth of search params.
 *
 * Called as a plain async function rather than through a renderer: it is a
 * server component whose only observable behaviour here is the fetch it does or
 * does not make, and the element it returns is stubbed above.
 */
function renderShows(searchParams: Record<string, string | string[] | undefined>) {
  return VenueYearArchiveShows({
    slug: 'the-rebel-lounge',
    venue,
    venueSlug: 'the-rebel-lounge',
    year: 2025,
    years,
    searchParams: Promise.resolve(searchParams),
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

/**
 * PSY-1770. `VenuePastShows` accepts page 1's rows ONLY when the URL asks for
 * page 1 — seeding another page's key with page 1's slice would look like a
 * cache hit and never correct itself — so before this ticket every `?page=N>1`
 * render fetched fifty rows, dehydrated them, and threw them away.
 */
describe('VenueYearArchiveShows — the page-1 read', () => {
  it('reads page 1 for the bare year URL', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse({ shows: [], venue_id: 7, total: 161, limit: 50, offset: 0, year: 2025 })
    )

    await renderShows({})

    expect(fetchedUrls()).toHaveLength(1)
    expect(fetchedUrls()[0]).toContain('time_filter=past&year=2025')
  })

  it('reads page 1 when the URL explicitly asks for page 1', async () => {
    fetchMock.mockResolvedValueOnce(
      okResponse({ shows: [], venue_id: 7, total: 161, limit: 50, offset: 0, year: 2025 })
    )

    await renderShows({ page: '1' })

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  /** THE assertion this ticket exists for. */
  it.each(['2', '4', '1000'])(
    'makes NO row read for ?page=%s',
    async page => {
      await renderShows({ page })

      expect(fetchMock).not.toHaveBeenCalled()
    }
  )

  /**
   * An unparseable page is page 1 for the client (`parseAsInteger.withDefault`
   * falls back), so it must be page 1 here too. Skipping the read on a URL the
   * hook then asks page 1 for would serve the canonical view unseeded — losing
   * the server-rendered rows PSY-1756 bought — which is the expensive direction
   * to be wrong in.
   */
  it.each(['abc', '', '0', '-3'])(
    'treats the unparseable ?page=%p as page 1 and still seeds it',
    async page => {
      fetchMock.mockResolvedValueOnce(
        okResponse({ shows: [], venue_id: 7, total: 161, limit: 50, offset: 0, year: 2025 })
      )

      await renderShows({ page })

      expect(fetchMock).toHaveBeenCalledTimes(1)
    }
  )

  /**
   * A repeated param arrives as an array. nuqs' server-side parser reads the
   * first entry, exactly as the browser hook does, so the two cannot disagree
   * about which page a duplicated `?page=` names.
   */
  it('reads a repeated ?page= the same way the client does', async () => {
    await renderShows({ page: ['2', '1'] })

    expect(fetchMock).not.toHaveBeenCalled()
  })

  /**
   * A failed read is not a reason to drop the page: the section fetches for
   * itself and owns its error state, which is what it did before the rows were
   * ever seeded server-side.
   */
  it('still renders when the page-1 read fails', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(500))

    await expect(renderShows({})).resolves.toBeTruthy()
  })
})
