'use client'

import Link from 'next/link'
import { Tag, MapPin, Users, Disc3 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import {
  getLabelStatusLabel,
  getLabelStatusVariant,
  formatLabelLocation,
} from '../types'
import type { LabelListItem } from '../types'

export type LabelCardDensity = 'compact' | 'comfortable' | 'expanded'

interface LabelCardProps {
  label: LabelListItem
  density?: LabelCardDensity
}

export function LabelCard({ label, density = 'comfortable' }: LabelCardProps) {
  const location = formatLabelLocation(label)
  const labelUrl = `/labels/${label.slug}`
  const statusLabel = getLabelStatusLabel(label.status)
  const statusVariant = getLabelStatusVariant(label.status)

  const artistText =
    label.artist_count === 1 ? '1 artist' : `${label.artist_count} artists`
  const releaseText =
    label.release_count === 1 ? '1 release' : `${label.release_count} releases`

  if (density === 'compact') {
    return (
      <article className="flex items-center gap-3 px-3 py-1.5 hover:bg-muted/50 rounded-md transition-colors">
        <Link
          href={labelUrl}
          className="font-medium text-sm truncate flex-1 hover:text-primary"
        >
          {label.name}
        </Link>
        <Badge variant={statusVariant} className="text-[10px] shrink-0">
          {statusLabel}
        </Badge>
        {location && (
          <span className="text-xs text-muted-foreground shrink-0">
            {location}
          </span>
        )}
        <span className="text-xs text-muted-foreground shrink-0 tabular-nums">
          {artistText}
        </span>
      </article>
    )
  }

  if (density === 'expanded') {
    return (
      <article className="rounded-lg border border-border/50 bg-card p-6 transition-shadow hover:shadow-sm">
        <div className="flex gap-4">
          <div className="shrink-0 rounded-md bg-muted/50 flex items-center justify-center h-20 w-20">
            <Tag className="h-10 w-10 text-muted-foreground/40" />
          </div>

          <div className="flex-1 min-w-0">
            <Link href={labelUrl} className="block group">
              <h3 className="font-bold text-xl text-foreground group-hover:text-primary transition-colors truncate">
                {label.name}
              </h3>
            </Link>

            <div className="flex items-center gap-3 flex-wrap mt-2">
              <Badge
                variant={statusVariant}
                className="text-xs px-2 py-0.5"
              >
                {statusLabel}
              </Badge>
              {location && (
                <span className="flex items-center gap-1 text-sm text-muted-foreground">
                  <MapPin className="h-3.5 w-3.5" />
                  {location}
                </span>
              )}
            </div>

            <div className="mt-3 flex items-center gap-4 text-sm text-muted-foreground">
              <span className="flex items-center gap-1.5">
                <Users className="h-4 w-4" />
                {artistText}
              </span>
              <span className="flex items-center gap-1.5">
                <Disc3 className="h-4 w-4" />
                {releaseText}
              </span>
            </div>
          </div>
        </div>
      </article>
    )
  }

  // Comfortable (default)
  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex gap-3">
        <div className="shrink-0 rounded-md bg-muted/50 flex items-center justify-center h-16 w-16">
          <Tag className="h-8 w-8 text-muted-foreground/40" />
        </div>

        <div className="flex-1 min-w-0">
          <Link href={labelUrl} className="block group">
            <h3 className="font-bold text-base text-foreground group-hover:text-primary transition-colors truncate">
              {label.name}
            </h3>
          </Link>

          <div className="flex items-center gap-2 flex-wrap mt-1">
            <Badge
              variant={statusVariant}
              className="text-[10px] px-1.5 py-0"
            >
              {statusLabel}
            </Badge>
            {location && (
              <span className="flex items-center gap-1 text-sm text-muted-foreground">
                <MapPin className="h-3 w-3" />
                {location}
              </span>
            )}
          </div>

          <div className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
            <span className="flex items-center gap-1">
              <Users className="h-3.5 w-3.5" />
              {artistText}
            </span>
            <span className="flex items-center gap-1">
              <Disc3 className="h-3.5 w-3.5" />
              {releaseText}
            </span>
          </div>
        </div>
      </div>
    </article>
  )
}
