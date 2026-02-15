import { Suspense } from 'react'
import * as Sentry from '@sentry/nextjs'
import { VenueList } from '@/components/venues'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

export const metadata = {
  title: 'Venues',
  description: 'Browse music venues and discover upcoming shows.',
  alternates: {
    canonical: 'https://psychichomily.com/venues',
  },
  openGraph: {
    title: 'Venues | Psychic Homily',
    description: 'Browse music venues and discover upcoming shows.',
    url: '/venues',
    type: 'website',
  },
}

interface VenueListItem {
  slug: string
  name: string
}

interface VenuesApiResponse {
  venues: VenueListItem[]
}

async function getVenues(): Promise<VenueListItem[]> {
  try {
    const res = await fetch(`${API_BASE_URL}/venues?limit=200`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      const data: VenuesApiResponse = await res.json()
      return data.venues ?? []
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Venues listing: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'venues-listing' },
        extra: { status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'venues-listing' },
    })
  }
  return []
}

function VenueListLoading() {
  return (
    <div className="flex justify-center items-center py-12">
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground"></div>
    </div>
  )
}

export default async function VenuesPage() {
  const venues = await getVenues()

  const venuesWithSlugs = venues.filter(
    (v): v is VenueListItem & { slug: string } => !!v.slug
  )

  return (
    <>
      {venuesWithSlugs.length > 0 && (
        <JsonLd data={generateItemListSchema({
          name: 'Venues',
          description: 'Music venues in Phoenix and beyond.',
          listItems: venuesWithSlugs.map(venue => ({
            url: `https://psychichomily.com/venues/${venue.slug}`,
            name: venue.name,
          })),
        })} />
      )}
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Venues', url: 'https://psychichomily.com/venues' },
      ])} />
      <div className="flex min-h-screen items-start justify-center">
        <main className="w-full max-w-4xl px-4 py-8 md:px-8">
          <h1 className="text-3xl font-bold text-center mb-8">Venues</h1>
          <Suspense fallback={<VenueListLoading />}>
            <VenueList />
          </Suspense>
        </main>
      </div>
    </>
  )
}
