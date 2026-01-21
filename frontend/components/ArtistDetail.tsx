'use client'

import Link from 'next/link'
import { ArrowLeft, Loader2, MapPin } from 'lucide-react'
import { useArtist } from '@/lib/hooks/useArtists'
import { SocialLinks } from '@/components/SocialLinks'
import { ArtistShowsList } from '@/components/ArtistShowsList'
import { Button } from '@/components/ui/button'

interface ArtistDetailProps {
  artistId: number
}

export function ArtistDetail({ artistId }: ArtistDetailProps) {
  const { data: artist, isLoading, error } = useArtist({ artistId })

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load artist'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Artist Not Found' : 'Error Loading Artist'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The artist you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/shows">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Shows
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!artist) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Artist Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The artist you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/shows">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Shows
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const hasLocation = artist.city || artist.state

  return (
    <div className="container max-w-4xl mx-auto px-4 py-6">
      {/* Back Navigation */}
      <div className="mb-6">
        <Link
          href="/shows"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back to Shows
        </Link>
      </div>

      {/* Header */}
      <header className="mb-8">
        <h1 className="text-2xl md:text-3xl font-bold">{artist.name}</h1>
        {hasLocation && (
          <div className="flex items-center gap-1 text-muted-foreground mt-2">
            <MapPin className="h-4 w-4" />
            <span>
              {[artist.city, artist.state].filter(Boolean).join(', ')}
            </span>
          </div>
        )}

        {/* Social Links */}
        {artist.social && <SocialLinks social={artist.social} className="mt-4" />}
      </header>

      {/* Shows List */}
      <ArtistShowsList artistId={artist.id} artistName={artist.name} />
    </div>
  )
}
