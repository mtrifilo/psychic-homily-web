import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { okResponse, errorResponse } from '@/lib/seo/test-helpers'

// The real `notFound()` is typed `never` and THROWS — control does not return to
// the caller. A plain `vi.fn()` lets execution fall through, which quietly turns
// every "…is a 404" assertion into "…called a function and then carried on", so
// the mock throws a recognisable sentinel instead.
const NOT_FOUND = 'NEXT_NOT_FOUND'
vi.mock('next/navigation', () => ({
  notFound: vi.fn(() => {
    throw new Error(NOT_FOUND)
  }),
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

beforeEach(async () => {
  // reset, not clear: several tests below deliberately queue a response the
  // component must NOT consume, and `clearAllMocks` leaves a `…Once` queue
  // behind — it would surface as the NEXT test's venue read returning a
  // histogram.
  fetchMock.mockReset()
  // The notFound mock is module-scoped, so its call log outlives each test;
  // without this every `not.toHaveBeenCalled()` below would see the previous
  // test's 404.
  vi.mocked((await import('next/navigation')).notFound).mockClear()
  vi.stubGlobal('fetch', fetchMock)
})

/** Renders the archive and reports whether it 404'd rather than rendering. */
async function renderedNotFound(
  searchParams: Record<string, string | string[] | undefined> = {}
): Promise<boolean> {
  try {
    await renderArchive(searchParams)
    return false
  } catch (error) {
    if (error instanceof Error && error.message === NOT_FOUND) return true
    throw error
  }
}

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

    expect(await renderedNotFound()).toBe(false)
  })
})

/**
 * The three not-found rules, which MOVED into this module in PSY-1770 — they
 * used to live in the route body, and their tests did not come with them.
 *
 * The distinction they encode is the highest-consequence behaviour on this
 * route: 404 only on a POSITIVE absence. "The backend said this is not there"
 * and "the backend did not answer" are different facts, and collapsing them is
 * what turns a transient blip into a not-found body for every archive on the
 * site — while `generateMetadata` stamps `noindex` on it, at HTTP 200, with no
 * retry signal. The proxy deliberately fails open on exactly those responses; a
 * fail-closed page here would cancel that out.
 */
describe('VenueYearArchiveContent — what is and is not a 404', () => {
  it('404s a venue the backend says is gone', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(404))
    fetchMock.mockResolvedValueOnce(errorResponse(404))
    mockRows()

    expect(await renderedNotFound()).toBe(true)
  })

  it('404s a year the histogram does not carry', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    fetchMock.mockResolvedValueOnce(okResponse(buildYears([{ year: 2019, count: 4 }])))
    mockRows()

    // The component is scoped to 2025; the histogram knows only 2019.
    expect(await renderedNotFound()).toBe(true)
  })

  it('404s a year the histogram carries with a zero count', async () => {
    fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
    fetchMock.mockResolvedValueOnce(okResponse(buildYears([{ year: 2025, count: 0 }])))
    mockRows()

    expect(await renderedNotFound()).toBe(true)
  })

  /**
   * THE rule. A 5xx on the histogram is "could not tell", and an archive that
   * exists must keep rendering through it.
   */
  it.each([500, 429, 503])(
    'does NOT 404 when the histogram answered %d',
    async status => {
      fetchMock.mockResolvedValueOnce(okResponse(buildVenue()))
      fetchMock.mockResolvedValueOnce(errorResponse(status))
      mockRows()

      expect(await renderedNotFound()).toBe(false)
    }
  )

  /**
   * The venue read is the one exception, and deliberately so: without the venue
   * there is nothing to hang the page on, so an unavailable venue read still
   * falls through to the not-found body — by way of the page rather than the
   * head. Pinned because it is the asymmetry a reader would otherwise call a
   * bug.
   */
  it('404s when the venue read is merely unavailable, unlike the histogram', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(500))
    fetchMock.mockResolvedValueOnce(okResponse(buildYears([{ year: 2025, count: 161 }])))
    mockRows()

    expect(await renderedNotFound()).toBe(true)
  })
})
