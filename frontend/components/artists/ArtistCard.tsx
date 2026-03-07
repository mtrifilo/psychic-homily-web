'use client'

import Link from 'next/link'
import { MapPin, Music } from 'lucide-react'
import type { ArtistListItem } from '@/lib/types/artist'
import { getArtistLocation } from '@/lib/types/artist'

interface ArtistCardProps {
  artist: ArtistListItem
}

export function ArtistCard({ artist }: ArtistCardProps) {
  const hasLocation = artist.city || artist.state
  const location = hasLocation ? getArtistLocation(artist) : null

  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <Link
        href={`/artists/${artist.slug}`}
        className="block group"
      >
        <h3 className="font-semibold text-base text-foreground group-hover:text-primary transition-colors truncate">
          {artist.name}
        </h3>
      </Link>

      {/* Space for future tag pills - renders nothing until tags exist */}

      <div className="mt-2 space-y-1">
        <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
          <Music className="h-3.5 w-3.5 shrink-0" />
          <span>
            {artist.upcoming_show_count} upcoming
          </span>
        </div>

        {hasLocation && (
          <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <MapPin className="h-3.5 w-3.5 shrink-0" />
            <span>{location}</span>
          </div>
        )}
      </div>
    </article>
  )
}
