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
 * The three answers a server read can give, kept apart on purpose.
 *
 * "The backend said this does not exist" and "the backend did not answer" are
 * different facts, and collapsing them into `null` is what turns a transient
 * outage into an SEO incident: the page 404s a real archive and the metadata
 * stamps `noindex` on it, at HTTP 200, with no retry signal — an active
 * de-index instruction for a URL that was fine ten seconds ago. Only `missing`
 * may produce either.
 *
 * Nothing here throws, because the page component is the only place that may
 * call `notFound()`; from a helper it renders the not-found body and leaves the
 * status at 200 (measured on the scene pages, PSY-906). On these routes the 404
 * STATUS comes from one layer earlier still — the existence branch in proxy.ts —
 * because a `notFound()` reached after the shell has streamed cannot set it.
 */
export type ArchiveRead<T> =
  | { status: 'ok'; data: T }
  /** The backend answered 404: this thing genuinely is not there. */
  | { status: 'missing' }
  /** 5xx, a timeout, a network error, an unparseable body: we cannot tell. */
  | { status: 'unavailable' }

/** Narrowing helper for the common "did we get it" question. */
export function archiveData<T>(read: ArchiveRead<T>): T | null {
  return read.status === 'ok' ? read.data : null
}

/**
 * Read one JSON document for the archive.
 *
 * `timeoutMs` is per caller because the callers disagree about what a failed
 * read costs. A read whose failure degrades gracefully wants a tight budget so
 * a slow backend cannot hold the visitor's render open; a read whose failure
 * takes the whole page down wants the backend's own budget, because turning a
 * slow response into a 404 is worse than waiting. Omit it for no timeout.
 */
async function readArchiveJson<T>(
  url: string,
  service: string,
  context: Record<string, unknown>,
  timeoutMs?: number
): Promise<ArchiveRead<T>> {
  try {
    const res = await fetch(url, {
      next: { revalidate: ARCHIVE_REVALIDATE_SECONDS },
      ...(timeoutMs === undefined
        ? {}
        : { signal: createBuildTimeApiSignal(timeoutMs) }),
    })
    if (res.ok) return { status: 'ok', data: (await res.json()) as T }
    // 404s are expected for a slug that does not exist; only a real fault is
    // worth a Sentry event.
    if (res.status === 404) return { status: 'missing' }
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
  return { status: 'unavailable' }
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
 *
 * NO timeout, deliberately. Both callers treat a failed venue read as "this
 * page does not exist", so a budget here would convert a slow backend into a
 * 404 for a venue that is perfectly real — a failure mode the venue page did
 * not have before this module existed. `surface` separates the two callers in
 * Sentry, so a regression specific to the archive route is visible.
 */
export const getVenue = cache(
  (idOrSlug: string, surface: 'venue-page' | 'venue-year-archive' = 'venue-page') =>
    readArchiveJson<Venue>(venueEndpoints.GET(encodeURIComponent(idOrSlug)), surface, {
      idOrSlug,
      surface,
    })
)

/**
 * The venue's PAST year histogram — the authority on which years exist.
 *
 * `time_filter=past` is what makes a year archive an archive, and it is the
 * same filter the year strip, the proxy's existence branch and the
 * `venue_years` sitemap family are built from. All four must agree, or the site
 * advertises a year it 404s.
 *
 * Timed out at the first-screen budget: both callers degrade to the pre-ticket
 * behaviour on a failed read (the strip appears after the client's own fetch
 * instead of in the HTML), so a slow backend must not hold the render open.
 */
export const getArchiveYears = cache((slug: string) =>
  readArchiveJson<VenueShowYearsResponse>(
    `${venueEndpoints.SHOW_YEARS(encodeURIComponent(slug))}?time_filter=past`,
    'venue-year-archive-years',
    { slug },
    FIRST_SCREEN_FETCH_TIMEOUT_MS
  )
)

/**
 * The first page of one year's rows — what the archive route renders.
 *
 * Nothing per-viewer is sent, so a SERVER read is a legitimate stand-in for the
 * one the client hook would make: the upcoming/past split is made in each show's
 * own venue-local day, and the year filter is venue-local too, so one answer is
 * the right answer for every reader.
 *
 * It is a DIFFERENT URL from the hook's — this one addresses the venue by SLUG
 * and orders its parameters differently, while the hook addresses it by numeric
 * id — and that is harmless. These rows reach the client through the
 * `initialData` prop, not by the hook finding this fetch's entry, and no
 * rewriting of this URL could change that: the hook's request is a browser
 * fetch, which can never read a server-side Data Cache entry.
 *
 * Timed out for the same reason as the histogram: an unseeded archive still
 * renders, it just fetches its rows on the client.
 */
export const getArchiveFirstPage = cache((slug: string, year: number) =>
  readArchiveJson<VenueShowsResponse>(
    `${venueEndpoints.SHOWS(encodeURIComponent(slug))}` +
      `?time_filter=past&year=${year}&limit=${VENUE_PAST_SHOWS_PAGE_LIMIT}`,
    'venue-year-archive-page',
    { slug, year },
    FIRST_SCREEN_FETCH_TIMEOUT_MS
  )
)

/**
 * Whether the histogram POSITIVELY says this venue has past shows in `year`.
 *
 * Only ever true on a successful read. A caller that needs to distinguish "no"
 * from "could not tell" must check the read's status itself — this returning
 * false is not evidence the year is absent.
 */
export function archiveYearExists(
  years: ArchiveRead<VenueShowYearsResponse>,
  year: number
): boolean {
  if (years.status !== 'ok') return false
  return years.data.years.some(entry => entry.year === year && entry.count > 0)
}
