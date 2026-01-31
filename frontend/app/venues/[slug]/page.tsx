import { Suspense } from 'react'
import { Metadata } from 'next'
import { Loader2 } from 'lucide-react'
import { VenueDetail } from '@/components/VenueDetail'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'https://api.psychichomily.com'

interface VenuePageProps {
  params: Promise<{ slug: string }>
}

export async function generateMetadata({ params }: VenuePageProps): Promise<Metadata> {
  const { slug } = await params
  try {
    const res = await fetch(`${API_BASE_URL}/venues/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      const venue = await res.json()
      return {
        title: venue.name,
        description: `${venue.name} in ${venue.city}, ${venue.state} - upcoming shows and venue details`,
        openGraph: {
          title: venue.name,
          description: `View upcoming shows at ${venue.name}`,
          type: 'website',
          url: `/venues/${slug}`,
        },
      }
    }
  } catch {
    // Fall through to default metadata
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

  return (
    <Suspense fallback={<VenueLoadingFallback />}>
      <VenueDetail venueId={slug} />
    </Suspense>
  )
}
