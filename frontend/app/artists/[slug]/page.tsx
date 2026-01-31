import { Suspense } from 'react'
import { Metadata } from 'next'
import { Loader2 } from 'lucide-react'
import { ArtistDetail } from '@/components/ArtistDetail'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'https://api.psychichomily.com'

interface ArtistPageProps {
  params: Promise<{ slug: string }>
}

export async function generateMetadata({ params }: ArtistPageProps): Promise<Metadata> {
  const { slug } = await params
  try {
    const res = await fetch(`${API_BASE_URL}/artists/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      const artist = await res.json()
      return {
        title: artist.name,
        description: `${artist.name} - upcoming shows and artist details on Psychic Homily`,
        openGraph: {
          title: artist.name,
          description: `View upcoming shows featuring ${artist.name}`,
          type: 'website',
          url: `/artists/${slug}`,
        },
      }
    }
  } catch {
    // Fall through to default metadata
  }
  return {
    title: 'Artist',
    description: 'View artist details and upcoming shows',
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
  const { slug } = await params

  if (!slug) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Artist</h1>
          <p className="text-muted-foreground">
            The artist could not be found.
          </p>
        </div>
      </div>
    )
  }

  return (
    <Suspense fallback={<ArtistLoadingFallback />}>
      <ArtistDetail artistId={slug} />
    </Suspense>
  )
}
