'use client'

import Link from 'next/link'
import { MapPin } from 'lucide-react'
import type { ArtistListItem } from '@/lib/types/artist'
import { getArtistLocation } from '@/lib/types/artist'

interface ArtistCardProps {
  artist: ArtistListItem
}

export function ArtistCard({ artist }: ArtistCardProps) {
  const hasLocation = artist.city || artist.state
  const location = hasLocation ? getArtistLocation(artist) : null

  return (
    <article className="py-2 px-3 rounded-md hover:bg-muted/50 transition-colors">
      <Link
        href={`/artists/${artist.slug}`}
        className="block hover:text-primary transition-colors"
      >
        <span className="font-medium">{artist.name}</span>
      </Link>
      <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
        <span>{artist.upcoming_show_count} upcoming</span>
        {hasLocation && (
          <>
            <span className="text-border">·</span>
            <span className="flex items-center gap-0.5">
              <MapPin className="h-3 w-3 shrink-0" />
              {location}
            </span>
          </>
        )}
      </div>
    </article>
  )
}
