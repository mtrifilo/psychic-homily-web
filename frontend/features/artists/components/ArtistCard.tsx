'use client'

import Link from 'next/link'
import { MapPin, Music } from 'lucide-react'
import type { ArtistListItem } from '../types'
import { getArtistLocation } from '../types'

export type ArtistCardDensity = 'compact' | 'comfortable' | 'expanded'

interface ArtistCardProps {
  artist: ArtistListItem
  density?: ArtistCardDensity
}

export function ArtistCard({ artist, density = 'comfortable' }: ArtistCardProps) {
  const hasLocation = artist.city || artist.state
  const location = hasLocation ? getArtistLocation(artist) : null

  if (density === 'compact') {
    return (
      <article className="flex items-center gap-3 px-3 py-1.5 hover:bg-muted/50 rounded-md transition-colors">
        <Link
          href={`/artists/${artist.slug}`}
          className="font-medium text-sm truncate min-w-0 flex-1 hover:text-primary transition-colors"
        >
          {artist.name}
        </Link>
        {hasLocation && (
          <span className="text-xs text-muted-foreground shrink-0">{location}</span>
        )}
        <span className="text-xs text-muted-foreground shrink-0 tabular-nums">
          {artist.upcoming_show_count} upcoming
        </span>
      </article>
    )
  }

  if (density === 'expanded') {
    return (
      <article className="rounded-lg border border-border/50 bg-card p-6 hover:shadow-md transition-shadow">
        <Link href={`/artists/${artist.slug}`} className="block group">
          <h3 className="font-bold text-xl text-foreground group-hover:text-primary transition-colors">
            {artist.name}
          </h3>
        </Link>
        <div className="mt-3 flex items-center gap-4 text-sm text-muted-foreground">
          {hasLocation && (
            <span className="flex items-center gap-1.5">
              <MapPin className="h-3.5 w-3.5 shrink-0" />
              {location}
            </span>
          )}
          <span className="flex items-center gap-1.5">
            <Music className="h-3.5 w-3.5 shrink-0" />
            {artist.upcoming_show_count} upcoming shows
          </span>
        </div>
      </article>
    )
  }

  // Comfortable (default)
  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <Link href={`/artists/${artist.slug}`} className="block group">
        <h3 className="font-bold text-base text-foreground group-hover:text-primary transition-colors truncate">
          {artist.name}
        </h3>
      </Link>

      <div className="mt-2 space-y-1">
        <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
          <Music className="h-3.5 w-3.5 shrink-0" />
          <span>{artist.upcoming_show_count} upcoming</span>
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
