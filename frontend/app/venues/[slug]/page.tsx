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
import { getArchiveYears, getVenue } from '@/features/venues/archiveApi'

interface VenuePageProps {
  params: Promise<{ slug: string }>
}

export async function generateMetadata({ params }: VenuePageProps): Promise<Metadata> {
  const { slug } = await params
  const venue = await getVenue(slug)

  if (venue) {
    return {
      title: venue.name,
      description: `${venue.name} in ${venue.city}, ${venue.state} - upcoming shows and venue details`,
      alternates: {
        canonical: `https://psychichomily.com/venues/${slug}`,
      },
      openGraph: {
        title: venue.name,
        description: `View upcoming shows at ${venue.name}`,
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
  const [venueData, pastYears] = await Promise.all([
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
  ])

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
