import { Suspense } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary } from '@tanstack/react-query'
import { VenueDetail } from '@/features/venues'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicVenueSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'
import { archiveData, getArchiveYears, getVenue } from '@/features/venues/archiveApi'
import type { Venue, VenueShowsResponse } from '@/features/venues/types'
// The policy module, not the shows barrel: `showRails.ts` renders nothing.
import { venueRailShowsUrl } from '@/features/shows/showRails'
import { CURRENT_PERIOD_REVALIDATE } from '@/features/scenes/scenePeriodApi'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { venueNextShowFrom, venueSnippet } from '@/lib/seo/entitySnippets'

interface VenuePageProps {
  params: Promise<{ slug: string }>
}

/**
 * The venue's soonest upcoming shows, for the `Next` clause of the meta
 * description.
 *
 * The same URL and cache window as the show page's venue rail. The window is
 * the short one because the next show changes as each night passes. Null on
 * any failure, which only drops the clause.
 */
function getVenueUpcomingShows(venue: Venue): Promise<VenueShowsResponse | null> {
  // Checked rather than trusted: the id arrives from `res.json()` through a
  // type assertion and is interpolated into a server-side URL path.
  if (!Number.isSafeInteger(venue.id)) return Promise.resolve(null)
  return fetchListPayload<VenueShowsResponse>({
    url: venueRailShowsUrl(venue.id),
    collection: 'shows',
    service: 'venue-page-next-show',
    revalidateSeconds: CURRENT_PERIOD_REVALIDATE,
  })
}

export async function generateMetadata({ params }: VenuePageProps): Promise<Metadata> {
  const { slug } = await params
  const venue = archiveData(await getVenue(slug))

  if (venue) {
    const upcoming = await getVenueUpcomingShows(venue)
    const { title, description } = venueSnippet({
      name: venue.name,
      city: venue.city,
      state: venue.state,
      nextShow: upcoming
        ? venueNextShowFrom(upcoming.shows, {
            state: venue.state,
            timezone: venue.timezone,
          })
        : null,
    })
    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/venues/${slug}`,
      },
      openGraph: {
        title,
        description,
        type: 'website',
        url: `/venues/${slug}`,
      },
    }
  }

  return {
    title: 'Venue',
    description: 'View venue details and upcoming shows',
  }
}

function VenueLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function VenuePage({ params }: VenuePageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  // Both reads take the slug from `params`, so neither waits on the other. The
  // histogram is thrown away for a venue that turns out not to exist, which the
  // proxy's existence check already filters before this route renders.
  const [venueRead, pastYearsRead] = await Promise.all([
    getVenue(slug),
    // The past-shows year strip, server-side (PSY-1756).
    //
    // Without it the archive section renders nothing until its first client
    // fetch, so the links to a venue's year archives were in no served HTML and
    // a crawler could only reach them through the sitemap. One row per year, so
    // it is the cheapest thing that turns the venue page into the hub of its
    // own archive.
    //
    // Null on a failed read. The strip then behaves exactly as it did before
    // this ticket — it appears after the client fetch — instead of taking the
    // venue page down with it.
    getArchiveYears(slug),
    // NO month-histogram read here, deliberately (PSY-1769). This route does not
    // seed `initialShows`, so the archive renders its spinner server-side and no
    // pager reaches this document — a seeded histogram would buy nothing for the
    // HTML and cost a full-history aggregate on every render, for every venue,
    // including the majority whose archive fits on one page and shows no pager
    // at all. The client fetches it, gated on there being a pager to label. The
    // year-archive route DOES seed it, because there the pager is in the HTML.
  ])

  // Unchanged from before this ticket: ANY failed venue read is a not-found
  // here, indeterminate or not. Narrowing that to a positive 404 would be a
  // change to a shipped route's failure semantics, which is not this ticket's
  // to make.
  const venueData = archiveData(venueRead)
  const pastYears = archiveData(pastYearsRead)

  if (!venueData) {
    notFound()
  }

  const dehydratedState = await prefetchEntity(
    queryKeys.venues.detail(slug),
    venueData,
  )

  return (
    <>
      <JsonLd data={generateMusicVenueSchema({
        name: venueData.name,
        address: venueData.address ?? undefined,
        city: venueData.city,
        state: venueData.state,
        slug: venueData.slug || slug,
      })} />
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Venues', url: 'https://psychichomily.com/venues' },
        { name: venueData.name, url: `https://psychichomily.com/venues/${venueData.slug || slug}` },
      ])} />
      <HydrationBoundary state={dehydratedState}>
        <Suspense fallback={<VenueLoadingFallback />}>
          <VenueDetail venueId={slug} initialPastYears={pastYears ?? undefined} />
        </Suspense>
      </HydrationBoundary>
    </>
  )
}
