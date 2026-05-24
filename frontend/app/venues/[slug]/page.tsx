import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import * as Sentry from '@sentry/nextjs'
import { HydrationBoundary } from '@tanstack/react-query'
import { VenueDetail } from '@/features/venues'
import type { Venue } from '@/features/venues/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicVenueSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'

interface VenuePageProps {
  params: Promise<{ slug: string }>
}

/**
 * Wrapped with `React.cache()` so `generateMetadata` and the page body
 * share ONE backend fetch per request instead of two. The result also
 * seeds the TanStack Query cache via `prefetchEntity` below, eliminating
 * the client-side refetch on first paint.
 */
const getVenue = cache(async (slug: string): Promise<Venue | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/venues/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    // Don't report 404s - they're expected for invalid slugs
    if (res.status >= 500) {
      Sentry.captureMessage(`Venue page: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'venue-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'venue-page' },
      extra: { slug },
    })
  }
  return null
})

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

  const venueData = await getVenue(slug)

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
          <VenueDetail venueId={slug} />
        </Suspense>
      </HydrationBoundary>
    </>
  )
}
