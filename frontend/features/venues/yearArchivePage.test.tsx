import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

vi.mock('next/navigation', () => ({
  notFound: vi.fn(),
}))

// The archive body renders the real client archive; these tests only exercise
// which READS the server makes, so the render path is stubbed out the way the
// route's own test does.
vi.mock('./components/VenuePastShows', () => ({
  VenuePastShows: (): null => null,
  VENUE_PAST_SHOWS_ANCHOR: 'venue-past-shows',
}))

import { VenueYearArchiveContent } from './yearArchivePage'

function buildVenue(overrides: Record<string, unknown> = {}) {
  return {
    id: 7,
    name: 'The Rebel Lounge',
    slug: 'the-rebel-lounge',
    city: 'Phoenix',
    state: 'AZ',
    ...overrides,
  }
}

function buildYears(years: Array<{ year: number; count: number }>) {
  return { venue_id: 7, time_filter: 'past', years }
}

const fetchMock = vi.fn()

/** Every URL the component fetched, in order. */
function fetchedUrls(): string[] {
  return fetchMock.mock.calls.map(call => String(call[0]))
}

/** The row read is the only one carrying a page window. */
function rowReads(): string[] {
  return fetchedUrls().filter(url => url.includes('limit='))
}

/**
 * Render the archive for one URL's worth of search params.
 *
 * Called as a plain async function rather than through a renderer: it is a
 * server component whose observable behaviour here is which reads it makes, and
 * the client subtree it returns is stubbed above.
 */
function renderArchive(
  searchParams: Record<string, string | string[] | undefined>
) {
  return VenueYearArchiveContent({
    slug: 'the-rebel-lounge',
    year: 2025,
    searchParams: Promise.resolve(searchParams),
  })
}

/** The venue and the past-year histogram, in the order the component reads them. */
function mockVenueAndYears() {
  fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
  fetchMock.mockResolvedValueOnce(okResponse(buildYears([{ year: 2025, count: 161 }])))
}

function mockRows() {
  fetchMock.mockResolvedValueOnce(
    okResponse({ shows: [], venue_id: 7, total: 161, limit: 50, offset: 0, year: 2025 })
  )
}

beforeEach(() => {
  // reset, not clear: several tests below deliberately queue a response the
  // component must NOT consume, and `clearAllMocks` leaves a `…Once` queue
  // behind — it would surface as the NEXT test's venue read returning a
  // histogram.
  fetchMock.mockReset()
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
describe('VenueYearArchiveContent — the page-1 read', () => {
  it('reads page 1 for the bare year URL', async () => {
    mockVenueAndYears()
    mockRows()

    await renderArchive({})

    expect(rowReads()).toHaveLength(1)
    expect(rowReads()[0]).toContain('time_filter=past&year=2025')
  })

  it('reads page 1 when the URL explicitly asks for page 1', async () => {
    mockVenueAndYears()
    mockRows()

    await renderArchive({ page: '1' })

    expect(rowReads()).toHaveLength(1)
  })

  /** THE assertion this ticket exists for. */
  it.each(['2', '4', '1000'])('makes NO row read for ?page=%s', async page => {
    mockVenueAndYears()

    await renderArchive({ page })

    expect(rowReads()).toHaveLength(0)
    // The venue and the histogram are still read — the archive still renders,
    // it just does not seed rows the client would refuse.
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  /**
   * An unparseable page is page 1 for the client (`archivePageParser` falls back
   * to its default), so it must be page 1 here too. Skipping the read on a URL
   * the hook then asks page 1 for would serve the canonical view unseeded —
   * losing the server-rendered rows PSY-1756 bought — which is the expensive
   * direction to be wrong in.
   */
  it.each(['abc', '', '0', '-3'])(
    'treats the unparseable ?page=%p as page 1 and still seeds it',
    async page => {
      mockVenueAndYears()
      mockRows()

      await renderArchive({ page })

      expect(rowReads()).toHaveLength(1)
    }
  )

  /**
   * A repeated param arrives as an array. nuqs' server-side parser reads the
   * first entry, exactly as the browser hook does, so the two cannot disagree
   * about which page a duplicated `?page=` names.
   */
  it('reads a repeated ?page= the same way the client does', async () => {
    mockVenueAndYears()

    await renderArchive({ page: ['2', '1'] })

    expect(rowReads()).toHaveLength(0)
  })

  /**
   * All three reads go out TOGETHER. Deciding the page first and only then
   * fetching would put the rows behind the venue and the histogram, turning one
   * round trip into two on the cold canonical URL — a latency regression hiding
   * inside a performance ticket. Asserted by counting the fetches issued before
   * any of them is allowed to resolve.
   */
  it('starts the row read without waiting on the venue or the histogram', async () => {
    let releaseVenue: (value: unknown) => void = () => {}
    const blocked = new Promise(resolve => {
      releaseVenue = resolve
    })
    fetchMock.mockReturnValueOnce(blocked.then(() => okResponse(buildVenue())))
    fetchMock.mockResolvedValueOnce(
      okResponse(buildYears([{ year: 2025, count: 161 }]))
    )
    mockRows()

    const rendering = renderArchive({})
    // Let the microtask that awaits `searchParams` run, but keep the venue read
    // outstanding. The row read must already have been issued.
    await Promise.resolve()
    await Promise.resolve()
    expect(rowReads()).toHaveLength(1)

    releaseVenue(undefined)
    await rendering
  })

  /**
   * A failed read is not a reason to drop the page: the section fetches for
   * itself and owns its error state, which is what it did before the rows were
   * ever seeded server-side.
   */
  it('still renders when the page-1 read fails', async () => {
    mockVenueAndYears()
    fetchMock.mockResolvedValueOnce(errorResponse(500))

    await expect(renderArchive({})).resolves.toBeTruthy()
  })
})
