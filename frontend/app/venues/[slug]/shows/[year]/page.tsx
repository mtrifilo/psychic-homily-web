import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  buildVenueYearArchiveMetadata,
  VenueYearArchiveContent,
} from '@/features/venues/yearArchivePage'
import {
  archiveData,
  archiveYearExists,
  getArchiveFirstPage,
  getArchiveMonths,
  getArchiveYears,
  getVenue,
} from '@/features/venues/archiveApi'
// Entity-agnostic since PSY-1754 — the artist archive validates its `?year=`
// with the same function, against the same bounds the backend enforces.
import { parseArchiveYear } from '@/features/shows/showArchive'

interface VenueYearArchiveProps {
  params: Promise<{ slug: string; year: string }>
}

/**
 * The year segment as a number, or null if it is not a plausible calendar year.
 *
 * Two checks, not one. `parseArchiveYear` owns the RANGE (and is the same
 * function the archive has always validated `?year=` with), but it takes a
 * number, and `Number('2025abc')` is NaN while `Number(' 2025 ')` is 2025 — so
 * the literal four-digit shape is asserted first. A path segment is not a form
 * field: `/venues/x/shows/%202025%20` must be a 404, not a redirect-free alias
 * of the real archive, or the same rows answer at more than one URL.
 */
function parseYearSegment(segment: string): number | null {
  if (!/^\d{4}$/.test(segment)) return null
  return parseArchiveYear(Number(segment))
}

export async function generateMetadata({
  params,
}: VenueYearArchiveProps): Promise<Metadata> {
  const { slug, year } = await params
  const parsed = parseYearSegment(year)
  if (parsed === null) {
    return { title: 'Shows not found', robots: { index: false, follow: false } }
  }
  return buildVenueYearArchiveMetadata(slug, parsed)
}

/**
 * `/venues/{slug}/shows/{year}` (PSY-1756). See features/venues/yearArchivePage
 * for why the year is a path segment and the page is not.
 *
 * A year the venue has no PAST shows in is a 404 rather than an empty archive,
 * and that is the decision that keeps this route from being an infinite crawl
 * space: every four-digit segment from 1900 up is a URL, and the histogram is
 * the only thing that says which of them are documents. The same histogram
 * feeds the `venue_years` sitemap family and the proxy's existence branch, so
 * the set announced, the set that 200s and the set that renders are derived
 * from one source rather than kept in step by hand.
 *
 * The `notFound()` calls below render the not-found BODY; they do not set the
 * status. Measured on this build: `notFound()` reached after the shell has
 * streamed commits a 404 body at HTTP 200 (the soft-404 the whole of proxy.ts
 * exists to prevent), so the real 404 comes from the venue-year branch there.
 * These stay because a page must never render an archive it does not have, and
 * because the proxy fails OPEN on a backend blip — on that path this is what
 * the reader sees.
 */
export default async function VenueYearArchivePage({
  params,
}: VenueYearArchiveProps) {
  const { slug, year } = await params
  const parsedYear = parseYearSegment(year)
  if (!slug || parsedYear === null) {
    notFound()
  }

  // All three take the slug from `params`, so none of them waits on another —
  // the backend resolves an id or a slug identically, so the rows do not need
  // the venue row first. Serialising them would put a full round trip on the
  // critical path of every cold render, and by the time this route renders the
  // proxy's existence branch has already filtered the years that have no rows.
  const [venueRead, yearsRead, monthsRead, firstPageRead] = await Promise.all([
    getVenue(slug, 'venue-year-archive'),
    getArchiveYears(slug),
    // The pager's range labels (PSY-1769). This route renders the pager into
    // the served HTML, so without this read every page link in that document
    // would be a bare numeral until the client fetched the histogram.
    getArchiveMonths(slug),
    getArchiveFirstPage(slug, parsedYear),
  ])

  // 404 only on a POSITIVE absence. A read that failed is not an answer, and
  // treating it as one is what turns a backend blip into a not-found body for
  // every archive on the site — which the proxy deliberately does not do
  // either (it fails open on anything that is not a backend 404).
  const venue = archiveData(venueRead)
  if (venueRead.status === 'missing') {
    notFound()
  }
  if (yearsRead.status === 'ok' && !archiveYearExists(yearsRead, parsedYear)) {
    notFound()
  }
  // The venue is what every other piece hangs off. Without it there is nothing
  // to render, so this falls through to the not-found body — but by way of the
  // page rather than the head, and `generateMetadata` has already declined to
  // stamp `noindex` on a URL it could not check.
  if (!venue) {
    notFound()
  }

  return (
    <VenueYearArchiveContent
      venue={venue}
      venueSlug={venue.slug || slug}
      year={parsedYear}
      years={archiveData(yearsRead)}
      months={archiveData(monthsRead)}
      firstPage={archiveData(firstPageRead)}
    />
  )
}
