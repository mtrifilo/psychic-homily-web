/**
 * `/venues/{slug}/shows/{year}` — one venue's shows in one venue-local calendar
 * year, served as a crawlable document (PSY-1756).
 *
 * WHY A PATH SEGMENT rather than `?year=` on the venue route. Measured on this
 * branch, both ways, against `next build` + `next start` + curl:
 *
 *   - Route mode is NOT the differentiator. Reading `searchParams` inside a
 *     Suspense boundary left `/venues/[slug]` at `◐ Partial Prerender`, same as
 *     main. Neither candidate regresses it.
 *   - CONTENT is. The venue page's body is `VenueDetail`, a client component
 *     that fetches everything it renders, so a `?year=` variant of it carries
 *     the venue's whole page — the MusicVenue JSON-LD, the description, the
 *     upcoming list, the bill network — around one differing section. N years
 *     would be N near-duplicate documents of the same venue. A year archive is
 *     its own document, so it gets its own URL.
 *   - The year strip is the crawl path, and before this ticket it was in no
 *     served HTML at all (measured: zero `year-strip` occurrences on the venue
 *     page). Both surfaces now seed the histogram server-side, so every year of
 *     a venue's history is a real `<a href>` on the venue page AND in the HTML
 *     of every other year.
 *
 * `?page=` inside a year stays a QUERY and stays client-side, and
 * `generateMetadata` still does not read it — which is what makes every page of
 * a year canonicalize to the year root for free, structurally rather than by
 * remembering to. Since PSY-1770 the BODY reads it, for one decision and no
 * other: whether seeding page 1's rows is worth a request. See
 * `VenueYearArchiveShows`.
 */
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { JsonLd } from '@/components/seo/JsonLd'
import { Breadcrumb } from '@/components/shared'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { archiveIsFirstPage } from '@/features/shows/showArchive'
import { VenuePastShows } from './components/VenuePastShows'
import {
  archiveData,
  archiveYearExists,
  getArchiveFirstPage,
  getArchiveYears,
  getVenue,
} from './archiveApi'
import { venueArchiveHref } from './showArchive'
import type { VenueShowsResponse } from './types'

/** The archive's own URL, absolute. Its canonical, and the crumb it links. */
function archiveUrl(slug: string, year: number): string {
  return `${SITE_URL}${venueArchiveHref(slug, year, 1)}`
}

/**
 * Metadata for one year archive.
 *
 * The canonical is SELF-referencing and always the year root. A `?page=2` of
 * this year therefore canonicalizes here rather than declaring itself — the
 * locked decision for intra-year pages, and it holds structurally because
 * nothing in this function can see the page number.
 *
 * `noindex` is emitted ONLY on a positive absence: the backend said the venue
 * is not there, or it answered with a histogram that does not carry the year.
 * A read that FAILED gets a plain title and no robots directive, because
 * "noindex" is an instruction, not a shrug — stamped on a live archive during a
 * backend blip it de-indexes a working URL at HTTP 200, with no retry signal
 * and no way to notice. The proxy fails open on exactly those responses; a
 * fail-closed head here would cancel that out.
 */
export async function buildVenueYearArchiveMetadata(
  slug: string,
  year: number
): Promise<Metadata> {
  const [venueRead, yearsRead] = await Promise.all([
    getVenue(slug, 'venue-year-archive'),
    getArchiveYears(slug),
  ])
  const venue = archiveData(venueRead)

  if (venueRead.status === 'missing' || (venue && yearsRead.status === 'ok' && !archiveYearExists(yearsRead, year))) {
    return { title: 'Shows not found', robots: { index: false, follow: false } }
  }

  // Could not tell. Say as little as possible and instruct nothing: the year is
  // still the address, so the canonical stays honest even without the venue.
  if (!venue) {
    return {
      title: `Shows in ${year}`,
      alternates: { canonical: archiveUrl(slug, year) },
    }
  }

  const count =
    yearsRead.status === 'ok'
      ? (yearsRead.data.years.find(entry => entry.year === year)?.count ?? 0)
      : null
  const title = `${venue.name} shows in ${year}`
  const where = [venue.city, venue.state].filter(Boolean).join(', ')
  const scope = `at ${venue.name}${where ? ` in ${where}` : ''} during ${year}`
  const description =
    count === null
      ? `Every show we have on record ${scope}.`
      : `Every show we have on record ${scope} — ${count} ${count === 1 ? 'show' : 'shows'}.`
  const canonical = archiveUrl(venue.slug || slug, year)

  return {
    title,
    description,
    alternates: { canonical },
    openGraph: { title, description, url: canonical, type: 'website' },
  }
}

/** The route's search params, un-awaited. Awaited in exactly one place below. */
export type ArchiveSearchParams = Promise<
  Record<string, string | string[] | undefined>
>

/**
 * Page 1's rows, or null when this URL is not asking for page 1 (PSY-1770).
 *
 * `VenuePastShows` refuses page 1's rows when the URL asks for another page —
 * `initialData` attaches to whatever key is current, so seeding page 2 with page
 * 1's slice would look like a cache hit and never correct itself — so before
 * this the deep-page render fetched fifty rows, dehydrated them into the flight
 * payload, and threw them away. The client then fetched the page it actually
 * wanted. One wasted 50-row read per deep-page visit, on a URL space of one page
 * per fifty shows.
 *
 * An async function rather than an inline branch so the caller can hand it
 * straight to `Promise.all`: awaiting `searchParams` first and only then
 * deciding would put the row read BEHIND the venue and histogram reads, turning
 * one round trip into two on the cold canonical URL. Search params resolve
 * without a network hop, so as a promise this starts effectively at once.
 *
 * `archiveIsFirstPage` is the shared derivation, built on the same nuqs parser
 * `VenuePastShows` reads the URL with. A URL the two read differently is the one
 * real hazard here: the server would seed rows the hook does not ask for
 * (harmless — ignored) or withhold rows it does ask for (a canonical URL
 * rendering unseeded, which is the thing PSY-1756 bought).
 */
async function readSeedablePage(
  slug: string,
  year: number,
  searchParams: ArchiveSearchParams
): Promise<VenueShowsResponse | null> {
  if (!archiveIsFirstPage(await searchParams)) return null
  return archiveData(await getArchiveFirstPage(slug, year))
}

/**
 * The archive body, and the only component on this route that reads anything.
 *
 * ONE async component under ONE Suspense boundary, rather than the page body
 * doing the reads. That is what lets `?page=` be read at all: `searchParams` in
 * the page body would make the whole route dynamic and cost it its prerendered
 * shell (`/artists` uses the same shape for the same reason, PSY-1774). The
 * route file keeps only the path-segment validation, which needs no network.
 *
 * All three reads start TOGETHER. Two of them take the slug from `params` and
 * the third takes it plus the year, so none waits on another — and serialising
 * any of them would put a full round trip on the critical path of every cold
 * render, which is the opposite of what PSY-1770 is for.
 *
 * `VenuePastShows` is the SAME component the venue page renders, scoped by
 * `activeYear` and handed the rows and histogram fetched here. It is a client
 * component, which is not in tension with any of this: client components render
 * on the server too, and with its data seeded it has nothing to wait for, so the
 * table, the year strip and the pager are all in the served HTML rather than
 * appearing after a client fetch.
 *
 * A `notFound()` here renders the not-found BODY; it does not set the status.
 * Measured on this build: `notFound()` reached after the shell has streamed
 * commits a 404 body at HTTP 200 (the soft-404 the whole of proxy.ts exists to
 * prevent), so the real 404 comes from the venue-year branch there. These stay
 * because a page must never render an archive it does not have, and because the
 * proxy fails OPEN on a backend blip — on that path this is what the reader
 * sees.
 */
export async function VenueYearArchiveContent({
  slug,
  year,
  searchParams,
}: {
  /** The slug as the request spelled it — what the archive reads are keyed on. */
  slug: string
  year: number
  searchParams: ArchiveSearchParams
}) {
  const [venueRead, yearsRead, firstPage] = await Promise.all([
    getVenue(slug, 'venue-year-archive'),
    getArchiveYears(slug),
    readSeedablePage(slug, year, searchParams),
  ])

  // 404 only on a POSITIVE absence. A read that failed is not an answer, and
  // treating it as one is what turns a backend blip into a not-found body for
  // every archive on the site — which the proxy deliberately does not do either
  // (it fails open on anything that is not a backend 404).
  const venue = archiveData(venueRead)
  if (venueRead.status === 'missing') {
    notFound()
  }
  if (yearsRead.status === 'ok' && !archiveYearExists(yearsRead, year)) {
    notFound()
  }
  // The venue is what every other piece hangs off. Without it there is nothing
  // to render, so this falls through to the not-found body — but by way of the
  // page rather than the head, and `generateMetadata` has already declined to
  // stamp `noindex` on a URL it could not check.
  if (!venue) {
    notFound()
  }

  /**
   * Null when the histogram read failed. The strip then appears after the
   * client's own fetch rather than in the HTML — degraded, not broken, and
   * strictly better than 404ing an archive over a transient blip.
   */
  const years = archiveData(yearsRead)
  const venueSlug = venue.slug || slug
  const venueHref = `/venues/${venueSlug}`

  return (
    <div className="container max-w-6xl mx-auto px-4 py-6">
      <JsonLd
        data={generateBreadcrumbSchema([
          { name: 'Home', url: SITE_URL },
          { name: 'Venues', url: `${SITE_URL}/venues` },
          { name: venue.name, url: `${SITE_URL}${venueHref}` },
          { name: `Shows in ${year}`, url: archiveUrl(venueSlug, year) },
        ])}
      />
      {/* The way back OUT of the archive, and the only one guaranteed to be
          there: the year strip below renders nothing for a venue with a single
          year, so a crawler that arrived here from the sitemap would otherwise
          have no link to the venue itself. */}
      <Breadcrumb
        fallback={{ href: '/venues', label: 'Venues' }}
        intermediate={[{ href: venueHref, label: venue.name }]}
        currentPage={`Shows in ${year}`}
      />
      <h1 className="mb-4 text-2xl font-semibold tracking-tight">
        {venue.name} shows in {year}
      </h1>
      <VenuePastShows
        venueId={venue.id}
        venueSlug={venueSlug}
        venueName={venue.name}
        venueState={venue.state}
        venueTimezone={venue.timezone}
        activeYear={year}
        initialYears={years ?? undefined}
        // Undefined on a failed read AND on every page but the first. The
        // section then fetches for itself and owns its error state, which is
        // strictly better than throwing away a page whose navigation is intact.
        initialShows={firstPage ?? undefined}
      />
    </div>
  )
}
