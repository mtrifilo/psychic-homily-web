'use client'

import Link from 'next/link'
import { Disc3 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { getReleaseTypeLabel } from '@/lib/types/release'
import type { ReleaseListItem } from '@/lib/types/release'

export type ReleaseCardDensity = 'compact' | 'comfortable' | 'expanded'

interface ReleaseCardProps {
  release: ReleaseListItem
  density?: ReleaseCardDensity
}

export function ReleaseCard({
  release,
  density = 'comfortable',
}: ReleaseCardProps) {
  return (
    <article
      className={cn(
        'rounded-lg border border-border/50 bg-card transition-shadow hover:shadow-sm',
        density === 'compact' && 'p-3',
        density === 'comfortable' && 'p-4',
        density === 'expanded' && 'p-5'
      )}
    >
      <div className="flex gap-3">
        {/* Cover Art / Placeholder */}
        <div
          className={cn(
            'shrink-0 rounded-md bg-muted/50 flex items-center justify-center overflow-hidden',
            density === 'compact' && 'h-12 w-12',
            density === 'comfortable' && 'h-16 w-16',
            density === 'expanded' && 'h-20 w-20'
          )}
        >
          {release.cover_art_url ? (
            <img
              src={release.cover_art_url}
              alt={`${release.title} cover art`}
              className="h-full w-full object-cover"
            />
          ) : (
            <Disc3
              className={cn(
                'text-muted-foreground/40',
                density === 'compact' && 'h-6 w-6',
                density === 'comfortable' && 'h-8 w-8',
                density === 'expanded' && 'h-10 w-10'
              )}
            />
          )}
        </div>

        {/* Text Content */}
        <div className="flex-1 min-w-0">
          <Link
            href={`/releases/${release.slug}`}
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
              {release.title}
            </h3>
          </Link>

          <div
            className={cn(
              'flex items-center gap-2 flex-wrap',
              density === 'compact' && 'mt-0.5',
              density === 'comfortable' && 'mt-1',
              density === 'expanded' && 'mt-1.5'
            )}
          >
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
              {getReleaseTypeLabel(release.release_type)}
            </Badge>
            {release.release_year && (
              <span
                className={cn(
                  'text-muted-foreground',
                  density === 'compact' ? 'text-xs' : 'text-sm'
                )}
              >
                {release.release_year}
              </span>
            )}
          </div>

          {density !== 'compact' && release.artist_count > 0 && (
            <div className="mt-1 text-sm text-muted-foreground">
              {release.artist_count === 1
                ? '1 artist'
                : `${release.artist_count} artists`}
            </div>
          )}
        </div>
      </div>
    </article>
  )
}
