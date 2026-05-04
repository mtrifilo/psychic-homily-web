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

// Format an ISO timestamp as "Mon YYYY" for the "last show" hint
// (e.g. "Mar 2024"). Returns null on unparseable input so the card can
// gracefully fall back to just "No upcoming shows".
function formatLastShowMonth(iso: string | null | undefined): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return d.toLocaleString('en-US', { month: 'short', year: 'numeric' })
}

// Build the "upcoming shows" affordance string. When the artist has upcoming
// shows, we show the count (current behavior). When the artist has none
// (PSY-495 Bandcamp model — dormant artists surfaced via tag filter), we
// show "No upcoming shows" and, if known, the last past-show month so the
// visitor sees the artist is real, just inactive, not broken.
function upcomingLabel(artist: ArtistListItem, short: boolean): string {
  if (artist.upcoming_show_count > 0) {
    return short
      ? `${artist.upcoming_show_count} upcoming`
      : `${artist.upcoming_show_count} upcoming shows`
  }
  const lastShow = formatLastShowMonth(artist.last_show_date)
  if (lastShow) {
    return `No upcoming shows · last show ${lastShow}`
  }
  return 'No upcoming shows'
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
          {upcomingLabel(artist, true)}
        </span>
      </article>
    )
  }

  if (density === 'expanded') {
    return (
      <article className="rounded-lg border border-border/50 bg-card p-6 hover:shadow-md transition-shadow">
        <Link href={`/artists/${artist.slug}`} className="block group">
          <h3
            className="font-bold text-xl text-foreground group-hover:text-primary transition-colors line-clamp-2"
            title={artist.name}
          >
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
            {upcomingLabel(artist, false)}
          </span>
        </div>
      </article>
    )
  }

  // Comfortable (default)
  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <Link href={`/artists/${artist.slug}`} className="block group">
        <h3
          className="font-bold text-base text-foreground group-hover:text-primary transition-colors line-clamp-2"
          title={artist.name}
        >
          {artist.name}
        </h3>
      </Link>

      <div className="mt-2 space-y-1">
        <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
          <Music className="h-3.5 w-3.5 shrink-0" />
          <span>{upcomingLabel(artist, true)}</span>
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
