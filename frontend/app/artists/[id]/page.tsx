import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import { ArtistDetail } from '@/components/ArtistDetail'

interface ArtistPageProps {
  params: Promise<{ id: string }>
}

export async function generateMetadata({ params }: ArtistPageProps) {
  const { id } = await params
  return {
    title: `Artist | Psychic Homily`,
    description: `View artist details and upcoming shows`,
  }
}

function ArtistLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ArtistPage({ params }: ArtistPageProps) {
  const { id } = await params
  const artistId = parseInt(id, 10)

  if (isNaN(artistId) || artistId <= 0) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Artist ID</h1>
          <p className="text-muted-foreground">
            The artist ID must be a valid number.
          </p>
        </div>
      </div>
    )
  }

  return (
    <Suspense fallback={<ArtistLoadingFallback />}>
      <ArtistDetail artistId={artistId} />
    </Suspense>
  )
}
