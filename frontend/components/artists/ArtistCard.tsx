'use client'

import Link from 'next/link'
import { MapPin, Music } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { ArtistListItem } from '@/lib/types/artist'
import { getArtistLocation } from '@/lib/types/artist'

export type ArtistCardDensity = 'compact' | 'comfortable' | 'expanded'

interface ArtistCardProps {
  artist: ArtistListItem
  density?: ArtistCardDensity
}

export function ArtistCard({ artist, density = 'comfortable' }: ArtistCardProps) {
  const hasLocation = artist.city || artist.state
  const location = hasLocation ? getArtistLocation(artist) : null

  return (
    <article
      className={cn(
        'rounded-lg border border-border/50 bg-card transition-shadow hover:shadow-sm',
        density === 'compact' && 'p-3',
        density === 'comfortable' && 'p-4',
        density === 'expanded' && 'p-5'
      )}
    >
      <Link
        href={`/artists/${artist.slug}`}
        className="block group"
      >
        <h3
          className={cn(
            'font-bold text-foreground group-hover:text-primary transition-colors truncate',
            density === 'compact' && 'text-sm',
            density === 'comfortable' && 'text-base',
            density === 'expanded' && 'text-lg'
          )}
        >
          {artist.name}
        </h3>
      </Link>

      {/* Space for future tag pills - renders nothing until tags exist */}

      <div
        className={cn(
          'space-y-1',
          density === 'compact' && 'mt-1',
          density === 'comfortable' && 'mt-2',
          density === 'expanded' && 'mt-3'
        )}
      >
        <div
          className={cn(
            'flex items-center gap-1.5 text-muted-foreground',
            density === 'compact' ? 'text-xs' : 'text-sm'
          )}
        >
          <Music className="h-3.5 w-3.5 shrink-0" />
          <span>
            {artist.upcoming_show_count} upcoming
          </span>
        </div>

        {hasLocation && (
          <div
            className={cn(
              'flex items-center gap-1.5 text-muted-foreground',
              density === 'compact' ? 'text-xs' : 'text-sm'
            )}
          >
            <MapPin className="h-3.5 w-3.5 shrink-0" />
            <span>{location}</span>
          </div>
        )}
      </div>
    </article>
  )
}
