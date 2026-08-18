import { Suspense } from 'react'
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import {
  buildVenueYearArchiveMetadata,
  VenueYearArchiveContent,
  type ArchiveSearchParams,
} from '@/features/venues/yearArchivePage'
// Entity-agnostic since PSY-1754 — the artist archive validates its `?year=`
// with the same function, against the same bounds the backend enforces.
import { parseArchiveYear } from '@/features/shows/showArchive'

interface VenueYearArchiveProps {
  params: Promise<{ slug: string; year: string }>
  /**
   * Passed straight through to `VenueYearArchiveContent` and awaited THERE,
   * never here. Awaiting it in this body would make the whole route dynamic and
   * cost it the prerendered shell PSY-1753/1756 measured. `generateMetadata`
   * does not take it at all — that is what keeps every `?page=` of a year on one
   * canonical.
   */
  searchParams: ArchiveSearchParams
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

/**
 * Deliberately the params-only half of {@link VenueYearArchiveProps}. Next would
 * hand `generateMetadata` the search params too; not naming them is what makes
 * "every `?page=` of a year shares one canonical" a property of the signature
 * rather than of remembering.
 */
export async function generateMetadata({
  params,
}: Pick<VenueYearArchiveProps, 'params'>): Promise<Metadata> {
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
 * This body reads NOTHING (PSY-1770). Every fetch, and the `notFound()` rules
 * that depend on one, moved into `VenueYearArchiveContent` under the boundary
 * below — which is what lets the archive read `?page=` at all, since awaiting
 * `searchParams` out here would make the whole route dynamic and cost it the
 * prerendered shell PSY-1753/1756 measured. What is left is the path-segment
 * check, which needs no network and so must not sit behind a boundary: a year
 * segment that cannot be a year is a dead end whatever the database says.
 */
export default async function VenueYearArchivePage({
  params,
  searchParams,
}: VenueYearArchiveProps) {
  const { slug, year } = await params
  const parsedYear = parseYearSegment(year)
  if (!slug || parsedYear === null) {
    notFound()
  }

  return (
    // `fallback={null}` rather than a skeleton. The boundary is here to keep
    // `searchParams` out of this body, not to defer anything a reader waits on:
    // the whole archive resolves on one round of parallel reads, exactly as it
    // did before this ticket, and a crawler receives it either way because the
    // response it gets is shell plus resume.
    <Suspense fallback={null}>
      <VenueYearArchiveContent
        slug={slug}
        year={parsedYear}
        searchParams={searchParams}
      />
    </Suspense>
  )
}
