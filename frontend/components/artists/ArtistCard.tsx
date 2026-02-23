'use client'

import Link from 'next/link'
import { MapPin } from 'lucide-react'
import type { Artist } from '@/lib/types/artist'
import { getArtistLocation } from '@/lib/types/artist'
import { SocialLinks } from '@/components/shared/SocialLinks'

interface ArtistCardProps {
  artist: Artist
}

export function ArtistCard({ artist }: ArtistCardProps) {
  const location = getArtistLocation(artist)
  const hasLocation = artist.city || artist.state

  return (
    <article className="border border-border/50 rounded-lg mb-4 overflow-hidden bg-card">
      <div className="px-4 py-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex-1 min-w-0">
            <h2 className="text-lg font-semibold truncate">
              <Link
                href={`/artists/${artist.slug}`}
                className="hover:text-primary transition-colors"
              >
                {artist.name}
              </Link>
            </h2>
            {hasLocation && (
              <div className="flex items-center gap-1 text-sm text-muted-foreground mt-1">
                <MapPin className="h-3.5 w-3.5 shrink-0" />
                <span>{location}</span>
              </div>
            )}
          </div>
          <SocialLinks social={artist.social} />
        </div>
      </div>
    </article>
  )
}
