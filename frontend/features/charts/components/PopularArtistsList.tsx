'use client'

import Link from 'next/link'
import { UserCheck, Calendar } from 'lucide-react'
import type { PopularArtist } from '../types'

interface PopularArtistsListProps {
  artists: PopularArtist[]
  compact?: boolean
}

export function PopularArtistsList({ artists, compact = false }: PopularArtistsListProps) {
  if (artists.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        No popular artists right now.
      </p>
    )
  }

  return (
    <ol className="space-y-1">
      {artists.map((artist, index) => (
        <li key={artist.artist_id}>
          <Link
            href={`/artists/${artist.slug}`}
            className="group flex items-center gap-3 rounded-lg px-3 py-2.5 transition-colors hover:bg-muted/50"
          >
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
              {index + 1}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium group-hover:text-primary truncate">
                {artist.name}
              </p>
            </div>
            {!compact && (
              <div className="flex shrink-0 items-center gap-3 text-xs text-muted-foreground">
                <span className="flex items-center gap-1" title="Followers">
                  <UserCheck className="h-3 w-3" />
                  {artist.follow_count}
                </span>
                <span className="flex items-center gap-1" title="Upcoming shows">
                  <Calendar className="h-3 w-3" />
                  {artist.upcoming_show_count}
                </span>
              </div>
            )}
          </Link>
        </li>
      ))}
    </ol>
  )
}
