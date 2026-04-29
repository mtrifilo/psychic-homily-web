'use client'

import Link from 'next/link'
import {
  Library,
  Users,
  Star,
  Clock,
  Mic2,
  MapPin,
  Calendar,
  Disc3,
  Tag,
  Tent,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { getEntityTypeLabel, type Collection } from '../types'
import { MarkdownContent } from './MarkdownEditor'

const ENTITY_ICONS: Record<string, LucideIcon> = {
  artist: Mic2,
  venue: MapPin,
  show: Calendar,
  release: Disc3,
  label: Tag,
  festival: Tent,
}

interface CollectionCardProps {
  collection: Collection
}

export function CollectionCard({ collection }: CollectionCardProps) {
  const topEntityTypes = Object.entries(collection.entity_type_counts ?? {})
    .sort((a, b) => b[1] - a[1])
    .slice(0, 2)

  // Get up to 4 entity type icons for the mosaic placeholder
  const mosaicTypes = Object.entries(collection.entity_type_counts ?? {})
    .sort((a, b) => b[1] - a[1])
    .slice(0, 4)
    .map(([type]) => type)

  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex gap-3">
        {/* Icon / cover image / entity-type mosaic */}
        <div className="h-16 w-16 shrink-0 rounded-md bg-muted/50 flex items-center justify-center overflow-hidden">
          {collection.cover_image_url ? (
            <img
              src={collection.cover_image_url}
              alt={`${collection.title} cover`}
              className="h-full w-full object-cover"
            />
          ) : mosaicTypes.length > 0 ? (
            <div
              className={cn(
                'grid gap-0.5 p-1.5',
                mosaicTypes.length === 1
                  ? 'grid-cols-1'
                  : 'grid-cols-2'
              )}
            >
              {mosaicTypes.map((type) => {
                const Icon = ENTITY_ICONS[type] ?? Library
                return (
                  <div
                    key={type}
                    className="flex items-center justify-center"
                  >
                    <Icon
                      className={cn(
                        'text-muted-foreground/50',
                        mosaicTypes.length === 1 ? 'h-7 w-7' : 'h-5 w-5'
                      )}
                    />
                  </div>
                )
              })}
            </div>
          ) : (
            <Library className="h-8 w-8 text-muted-foreground/40" />
          )}
        </div>

        {/* Text content */}
        <div className="flex-1 min-w-0">
          <Link href={`/collections/${collection.slug}`} className="block group">
            <h3
              className="font-bold text-foreground group-hover:text-primary transition-colors line-clamp-1"
              title={collection.title}
            >
              {collection.title}
            </h3>
          </Link>

          <div className="flex items-center gap-1 flex-wrap mt-0.5">
            {/*
              PSY-350: "N new" badge for subscribed collections in the library
              tab. Backend only populates new_since_last_visit when the viewer
              is subscribed; for public list cards, this prop is undefined or
              zero and the badge is hidden.
            */}
            {collection.new_since_last_visit !== undefined &&
              collection.new_since_last_visit > 0 && (
                <Badge
                  variant="default"
                  className="text-[10px] px-1.5 py-0"
                  aria-label={`${collection.new_since_last_visit} new since your last visit`}
                >
                  {collection.new_since_last_visit} new
                </Badge>
              )}
            {collection.is_featured && (
              <Badge variant="default" className="text-[10px] px-1.5 py-0">
                <Star className="h-2.5 w-2.5 mr-0.5" />
                Featured
              </Badge>
            )}
            {collection.collaborative && (
              <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                Collaborative
              </Badge>
            )}
            {topEntityTypes.map(([type, count]) => (
              <Badge
                key={type}
                variant="outline"
                className="text-[10px] px-1.5 py-0 font-normal"
              >
                {count} {getEntityTypeLabel(type).toLowerCase()}
                {count === 1 ? '' : 's'}
              </Badge>
            ))}
          </div>

          {/* PSY-349: server-rendered description_html (sanitized markdown).
              line-clamp keeps the card height stable; the prose styles
              come from MarkdownContent. Falls back to nothing rather than
              rendering raw markdown source as HTML. */}
          {collection.description_html && (
            <MarkdownContent
              html={collection.description_html}
              className={cn(
                'text-sm text-muted-foreground mt-1 line-clamp-3'
              )}
            />
          )}

          <div className="mt-1.5 flex items-center gap-3 text-xs text-muted-foreground flex-wrap">
            <span>by {collection.creator_name}</span>
            <span className="flex items-center gap-1">
              <Library className="h-3 w-3" />
              {collection.item_count === 1
                ? '1 item'
                : `${collection.item_count} items`}
            </span>
            {collection.subscriber_count > 0 && (
              <span className="flex items-center gap-1">
                <Users className="h-3 w-3" />
                {collection.subscriber_count === 1
                  ? '1 subscriber'
                  : `${collection.subscriber_count} subscribers`}
              </span>
            )}
            <span className="flex items-center gap-1">
              <Clock className="h-3 w-3" />
              {formatRelativeTime(collection.updated_at)}
            </span>
          </div>
        </div>
      </div>
    </article>
  )
}
