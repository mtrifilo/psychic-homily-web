/**
 * Server-side reads behind the venue show archive (PSY-1756).
 *
 * A leaf module by design: `app/venues/[slug]/page.tsx` needs the year
 * histogram so the year strip reaches its HTML, and reaching it through
 * `yearArchivePage` would drag that route's view, its breadcrumb and its
 * JSON-LD into the venue page's graph for one fetch.
 */
import { cache } from 'react'
import * as Sentry from '@sentry/nextjs'
import { createBuildTimeApiSignal } from '@/lib/build-time-api'
import { FIRST_SCREEN_FETCH_TIMEOUT_MS } from '@/lib/ssr/fetchListPayload'
import { venueEndpoints, VENUE_PAST_SHOWS_PAGE_LIMIT } from './api'
import type { Venue, VenueShowsResponse, VenueShowYearsResponse } from './types'

/**
 * How long the archive's reads stay warm.
 *
 * The same hour the venue page uses for the venue itself. It is the only thing
 * that refreshes a year page as new shows land: these routes have no
 * `generateStaticParams`, so their entity data is in the postponed dynamic
 * resume and is bounded by THIS window rather than by anything in the prerender
 * manifest (see the ISR notes in next.config.ts). For a closed year that means
 * an hour of nothing changing; for the CURRENT year it means a show that
 * graduated from upcoming to past appears within the hour.
 */
export const ARCHIVE_REVALIDATE_SECONDS = 3600

/**
 * Read one JSON document for the archive, or null.
 *
 * Null rather than throw, uniformly: a missing venue and an unreachable backend
 * both have to reach the page component, which is the only place that may call
 * `notFound()`. Calling it from a helper module renders the not-found body but
 * leaves the status at HTTP 200 (measured on the scene pages, PSY-906) — and on
 * these routes the 404 STATUS is produced one layer earlier still, by the
 * existence branch in proxy.ts, because a `notFound()` reached after the shell
 * has streamed cannot change the status at all.
 */
async function readArchiveJson<T>(
  url: string,
  service: string,
  context: Record<string, unknown>
): Promise<T | null> {
  try {
    const res = await fetch(url, {
      next: { revalidate: ARCHIVE_REVALIDATE_SECONDS },
      // These reads happen at REQUEST time (no generateStaticParams on either
      // route), so an unresponsive backend would hold the visitor's render open
      // rather than degrading. The same budget the other request-time server
      // reads use, for the same reason — and hitting it lands on the null
      // branch below, which is the pre-ticket behaviour: the section fetches
      // for itself on the client.
      signal: createBuildTimeApiSignal(FIRST_SCREEN_FETCH_TIMEOUT_MS),
    })
    if (res.ok) return (await res.json()) as T
    // 404s are expected for a slug that does not exist; only a real fault is
    // worth a Sentry event.
    if (res.status >= 500) {
      Sentry.captureMessage(`Venue show archive: API returned ${res.status}`, {
        level: 'error',
        tags: { service },
        extra: { ...context, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service },
      extra: context,
    })
  }
  return null
}

/**
 * One venue, by id or slug.
 *
 * Wrapped in `React.cache` so `generateMetadata` and the page body share ONE
 * trip per request instead of two — the pattern the scene pages already use.
 *
 * Shared by BOTH venue routes: the detail page and the year archive read a
 * venue the same way, with the same window and the same failure handling, and
 * two copies of that had already drifted on whether the slug is URL-encoded.
 */
export const getVenue = cache((idOrSlug: string) =>
  readArchiveJson<Venue>(
    venueEndpoints.GET(encodeURIComponent(idOrSlug)),
    'venue-page',
    { idOrSlug }
  )
)

/**
 * The venue's PAST year histogram — the authority on which years exist.
 *
 * `time_filter=past` is what makes a year archive an archive, and it is the
 * same filter the year strip, the proxy's existence branch and the
 * `venue_years` sitemap family are built from. All four must agree, or the site
 * advertises a year it 404s.
 */
export const getArchiveYears = cache((slug: string) =>
  readArchiveJson<VenueShowYearsResponse>(
    `${venueEndpoints.SHOW_YEARS(encodeURIComponent(slug))}?time_filter=past`,
    'venue-year-archive-years',
    { slug }
  )
)

/**
 * The first page of one year's rows — what the archive route renders.
 *
 * `timezone` is deliberately NOT sent, so this URL differs from the one the
 * client hook builds (`venuePastShowsPageParams`, which always sends the
 * viewer's zone). The two still answer identically: the backend documents that
 * parameter as "deprecated and ignored — the upcoming/past split is made in each
 * show's own venue-local timezone", and the year filter is venue-local too. It
 * is omitted rather than filled in because the only zone this module could send
 * is the SERVER's, which is a fact about the machine and not about the reader.
 */
export const getArchiveFirstPage = cache((slug: string, year: number) =>
  readArchiveJson<VenueShowsResponse>(
    `${venueEndpoints.SHOWS(encodeURIComponent(slug))}` +
      `?time_filter=past&year=${year}&limit=${VENUE_PAST_SHOWS_PAGE_LIMIT}`,
    'venue-year-archive-page',
    { slug, year }
  )
)

/** Whether this venue has any past show in `year`, per the histogram. */
export function archiveYearExists(
  years: VenueShowYearsResponse | null,
  year: number
): boolean {
  return (years?.years ?? []).some(
    entry => entry.year === year && entry.count > 0
  )
}
