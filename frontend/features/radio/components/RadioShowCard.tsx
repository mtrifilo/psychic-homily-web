'use client'

import Image from 'next/image'
import { Mic2, ListMusic } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { EntityCardTitle } from '@/components/shared'
import type { RadioShowListItem } from '../types'

interface RadioShowCardProps {
  show: RadioShowListItem
  stationSlug: string
}

export function RadioShowCard({ show, stationSlug }: RadioShowCardProps) {
  const showUrl = `/radio/${stationSlug}/${show.slug}`
  const genreTags = show.genre_tags ?? []

  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex gap-3">
        {/* Show Image / Placeholder */}
        <div className="shrink-0 rounded-md bg-muted/50 flex items-center justify-center overflow-hidden h-14 w-14">
          {show.image_url ? (
            <Image
              src={show.image_url}
              alt={show.name}
              width={56}
              height={56}
              sizes="56px"
              className="h-full w-full object-cover"
            />
          ) : (
            <Mic2 className="h-7 w-7 text-muted-foreground/40" />
          )}
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          <EntityCardTitle name={show.name} href={showUrl} />

          {show.host_name && (
            <p className="text-sm text-muted-foreground mt-0.5">
              Hosted by {show.host_name}
            </p>
          )}

          <div className="flex items-center gap-2 flex-wrap mt-1.5">
            {genreTags.slice(0, 3).map(tag => (
              <Badge key={tag} variant="secondary" className="text-[10px] px-1.5 py-0">
                {tag}
              </Badge>
            ))}
            {show.episode_count > 0 && (
              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                <ListMusic className="h-3 w-3" />
                {show.episode_count} {show.episode_count === 1 ? 'episode' : 'episodes'}
              </span>
            )}
          </div>
        </div>
      </div>
    </article>
  )
}
