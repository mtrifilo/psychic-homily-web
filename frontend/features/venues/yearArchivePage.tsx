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
import { Suspense } from 'react'
import type { Metadata } from 'next'
import { JsonLd } from '@/components/seo/JsonLd'
import { Breadcrumb } from '@/components/shared'
import { SITE_URL } from '@/lib/seo/siteMetadata'
import { generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { VenuePastShows } from './components/VenuePastShows'
import {
  archiveData,
  archiveYearExists,
  getArchiveFirstPage,
  getArchiveYears,
  getVenue,
} from './archiveApi'
import { venueArchiveHref, venueArchivePageParam } from './showArchive'
import type { Venue, VenueShowYearsResponse } from './types'

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

/**
 * The archive's rows, and the one place `?page=` is read on the server.
 *
 * `VenuePastShows` is the SAME component the venue page renders, scoped by
 * `activeYear` and handed the rows and histogram this route already fetched. It
 * is a client component, which is not in tension with any of this: client
 * components render on the server too, and with its data seeded it has nothing
 * to wait for, so the table, the year strip and the pager are all in the served
 * HTML rather than appearing after a client fetch.
 *
 * SEEDED ONLY ON PAGE 1, which is what this component exists for (PSY-1770).
 * `VenuePastShows` already refuses page 1's rows when the URL asks for another
 * page — `initialData` attaches to whatever key is current, so seeding page 2
 * with page 1's slice would look like a cache hit and never correct itself — so
 * before this the deep-page render fetched fifty rows, dehydrated them into the
 * flight payload, and threw them away. The client then fetched the page it
 * actually wanted. One wasted 50-row read per deep-page visit, on a URL space
 * of one page per fifty shows.
 *
 * ITS OWN ASYNC COMPONENT, under Suspense, because reading `searchParams` in
 * the page body would make the whole route dynamic and cost it its prerendered
 * shell — the same shape `/artists` uses for the same reason. Only this block
 * resumes per request; a crawler still receives it, because the response it
 * gets is shell plus resume.
 *
 * The page number is derived through `venueArchivePageParam`, which is the
 * client's own parser and bound. A URL the two read differently would be the
 * one real hazard here: the server would seed rows the hook does not ask for
 * (harmless — ignored) or withhold rows it does (a canonical URL rendering
 * unseeded, which is the thing PSY-1756 bought).
 */
export async function VenueYearArchiveShows({
  slug,
  venue,
  venueSlug,
  year,
  years,
  searchParams,
}: {
  /** The slug as the request spelled it — what the archive reads are keyed on. */
  slug: string
  venue: Venue
  venueSlug: string
  year: number
  years: VenueShowYearsResponse | null
  searchParams: Promise<Record<string, string | string[] | undefined>>
}) {
  const page = venueArchivePageParam(await searchParams)
  const firstPageRead = page === 1 ? await getArchiveFirstPage(slug, year) : null

  return (
    <VenuePastShows
      venueId={venue.id}
      venueSlug={venueSlug}
      venueName={venue.name}
      venueState={venue.state}
      venueTimezone={venue.timezone}
      activeYear={year}
      initialYears={years ?? undefined}
      // Undefined on a failed read AND on every page but the first. The section
      // then fetches for itself and owns its error state, which is strictly
      // better than throwing away a page whose navigation is intact.
      initialShows={
        firstPageRead ? (archiveData(firstPageRead) ?? undefined) : undefined
      }
    />
  )
}

/**
 * The archive body.
 */
export function VenueYearArchiveContent({
  slug,
  venue,
  venueSlug,
  year,
  years,
  searchParams,
}: {
  slug: string
  venue: Venue
  /** The venue's canonical slug, resolved once by the route. */
  venueSlug: string
  year: number
  /**
   * Null when the histogram read failed. The strip then appears after the
   * client's own fetch rather than in the HTML — degraded, not broken, and
   * strictly better than 404ing an archive over a transient blip.
   */
  years: VenueShowYearsResponse | null
  searchParams: Promise<Record<string, string | string[] | undefined>>
}) {
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
      {/* `fallback={null}` rather than a skeleton: the boundary exists to keep
          `searchParams` out of the page body, not to defer anything a reader
          waits on. On page 1 it resolves on one already-budgeted fetch — the
          same one the page used to block its entire HTML on — and on every
          other page there is no fetch at all, so there is nothing for a
          placeholder to stand in for. The block appends BELOW the heading, so
          arriving late costs no layout shift. */}
      <Suspense fallback={null}>
        <VenueYearArchiveShows
          slug={slug}
          venue={venue}
          venueSlug={venueSlug}
          year={year}
          years={years}
          searchParams={searchParams}
        />
      </Suspense>
    </div>
  )
}
