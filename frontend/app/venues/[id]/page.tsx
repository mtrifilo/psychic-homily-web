import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import { VenueDetail } from '@/components/VenueDetail'

interface VenuePageProps {
  params: Promise<{ id: string }>
}

export async function generateMetadata({ params }: VenuePageProps) {
  const { id } = await params
  return {
    title: `Venue | Psychic Homily`,
    description: `View venue details and upcoming shows`,
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
  const { id } = await params
  const venueId = parseInt(id, 10)

  if (isNaN(venueId) || venueId <= 0) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Venue ID</h1>
          <p className="text-muted-foreground">
            The venue ID must be a valid number.
          </p>
        </div>
      </div>
    )
  }

  return (
    <Suspense fallback={<VenueLoadingFallback />}>
      <VenueDetail venueId={venueId} />
    </Suspense>
  )
}
