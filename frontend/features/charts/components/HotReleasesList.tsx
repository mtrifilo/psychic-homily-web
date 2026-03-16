'use client'

import Link from 'next/link'
import { Bookmark, Calendar } from 'lucide-react'
import type { HotRelease } from '../types'

interface HotReleasesListProps {
  releases: HotRelease[]
  compact?: boolean
}

export function HotReleasesList({ releases, compact = false }: HotReleasesListProps) {
  if (releases.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        No hot releases right now.
      </p>
    )
  }

  return (
    <ol className="space-y-1">
      {releases.map((release, index) => (
        <li key={release.release_id}>
          <Link
            href={`/releases/${release.slug}`}
            className="group flex items-center gap-3 rounded-lg px-3 py-2.5 transition-colors hover:bg-muted/50"
          >
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
              {index + 1}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium group-hover:text-primary truncate">
                {release.title}
              </p>
              {!compact && (
                <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
                  {release.artist_names && release.artist_names.length > 0 && (
                    <span className="truncate">
                      {release.artist_names.join(', ')}
                    </span>
                  )}
                  {release.release_date && (
                    <span className="flex items-center gap-1">
                      <Calendar className="h-3 w-3" />
                      {new Date(release.release_date).toLocaleDateString('en-US', {
                        month: 'short',
                        day: 'numeric',
                        year: 'numeric',
                      })}
                    </span>
                  )}
                </div>
              )}
            </div>
            <div className="flex shrink-0 items-center gap-1 text-xs text-muted-foreground">
              <Bookmark className="h-3 w-3" />
              {release.bookmark_count}
            </div>
          </Link>
        </li>
      ))}
    </ol>
  )
}
