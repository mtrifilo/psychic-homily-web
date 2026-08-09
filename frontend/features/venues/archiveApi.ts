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
import { API_BASE_URL } from '@/lib/api-base'
import { VENUE_PAST_SHOWS_PAGE_LIMIT } from './api'
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
 * The venue behind an archive URL.
 *
 * Wrapped in `React.cache` so `generateMetadata` and the page body share ONE
 * trip per request instead of two — the pattern the venue page and the scene
 * pages already use. By SLUG deliberately: the backend resolves either, and the
 * slug means the archive never has to load the venue before asking for shows.
 */
export const getArchiveVenue = cache((slug: string) =>
  readArchiveJson<Venue>(
    `${API_BASE_URL}/venues/${encodeURIComponent(slug)}`,
    'venue-year-archive',
    { slug }
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
    `${API_BASE_URL}/venues/${encodeURIComponent(slug)}/shows/years?time_filter=past`,
    'venue-year-archive-years',
    { slug }
  )
)

/** The first page of one year's rows — what the archive route renders. */
export const getArchiveFirstPage = cache((slug: string, year: number) =>
  readArchiveJson<VenueShowsResponse>(
    `${API_BASE_URL}/venues/${encodeURIComponent(slug)}/shows` +
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
