'use client'

import Link from 'next/link'
import { Tag, MapPin, Users, Disc3 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import {
  getLabelStatusLabel,
  getLabelStatusVariant,
  formatLabelLocation,
} from '@/lib/types/label'
import type { LabelListItem } from '@/lib/types/label'

export type LabelCardDensity = 'compact' | 'comfortable' | 'expanded'

interface LabelCardProps {
  label: LabelListItem
  density?: LabelCardDensity
}

export function LabelCard({ label, density = 'comfortable' }: LabelCardProps) {
  const location = formatLabelLocation(label)

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
        {/* Icon placeholder */}
        <div
          className={cn(
            'shrink-0 rounded-md bg-muted/50 flex items-center justify-center',
            density === 'compact' && 'h-12 w-12',
            density === 'comfortable' && 'h-16 w-16',
            density === 'expanded' && 'h-20 w-20'
          )}
        >
          <Tag
            className={cn(
              'text-muted-foreground/40',
              density === 'compact' && 'h-6 w-6',
              density === 'comfortable' && 'h-8 w-8',
              density === 'expanded' && 'h-10 w-10'
            )}
          />
        </div>

        {/* Text Content */}
        <div className="flex-1 min-w-0">
          <Link href={`/labels/${label.slug}`} className="block group">
            <h3
              className={cn(
                'font-bold text-foreground group-hover:text-primary transition-colors truncate',
                density === 'compact' && 'text-sm',
                density === 'comfortable' && 'text-base',
                density === 'expanded' && 'text-lg'
              )}
            >
              {label.name}
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
            <Badge
              variant={getLabelStatusVariant(label.status)}
              className="text-[10px] px-1.5 py-0"
            >
              {getLabelStatusLabel(label.status)}
            </Badge>
            {location && (
              <span
                className={cn(
                  'flex items-center gap-1 text-muted-foreground',
                  density === 'compact' ? 'text-xs' : 'text-sm'
                )}
              >
                <MapPin className="h-3 w-3" />
                {location}
              </span>
            )}
          </div>

          {density !== 'compact' && (
            <div className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
              <span className="flex items-center gap-1">
                <Users className="h-3.5 w-3.5" />
                {label.artist_count === 1
                  ? '1 artist'
                  : `${label.artist_count} artists`}
              </span>
              <span className="flex items-center gap-1">
                <Disc3 className="h-3.5 w-3.5" />
                {label.release_count === 1
                  ? '1 release'
                  : `${label.release_count} releases`}
              </span>
            </div>
          )}
        </div>
      </div>
    </article>
  )
}
