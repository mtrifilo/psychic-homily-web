import { Suspense } from 'react'
import { Metadata } from 'next'
import { Loader2 } from 'lucide-react'
import { VenueDetail } from '@/components/venues'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicVenueSchema } from '@/lib/seo/jsonld'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'https://api.psychichomily.com'

interface VenuePageProps {
  params: Promise<{ slug: string }>
}

interface VenueData {
  name: string
  slug?: string
  address?: string
  city?: string
  state?: string
  zip_code?: string
}

async function getVenue(slug: string): Promise<VenueData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/venues/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
  } catch {
    // Fall through to null
  }
  return null
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
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Venue</h1>
          <p className="text-muted-foreground">
            The venue could not be found.
          </p>
        </div>
      </div>
    )
  }

  const venueData = await getVenue(slug)

  return (
    <>
      {venueData && (
        <JsonLd data={generateMusicVenueSchema({
          name: venueData.name,
          address: venueData.address,
          city: venueData.city,
          state: venueData.state,
          zip_code: venueData.zip_code,
          slug: venueData.slug || slug,
        })} />
      )}
      <Suspense fallback={<VenueLoadingFallback />}>
        <VenueDetail venueId={slug} />
      </Suspense>
    </>
  )
}
