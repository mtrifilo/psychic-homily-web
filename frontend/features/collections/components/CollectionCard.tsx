'use client'

import Link from 'next/link'
import { Library, Users, Star, Clock } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { getEntityTypeLabel, type Collection } from '../types'

interface CollectionCardProps {
  collection: Collection
}

export function CollectionCard({ collection }: CollectionCardProps) {
  const topEntityTypes = Object.entries(collection.entity_type_counts ?? {})
    .sort((a, b) => b[1] - a[1])
    .slice(0, 2)

  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex gap-3">
        {/* Icon / cover image placeholder */}
        <div className="h-16 w-16 shrink-0 rounded-md bg-muted/50 flex items-center justify-center overflow-hidden">
          {collection.cover_image_url ? (
            <img
              src={collection.cover_image_url}
              alt={`${collection.title} cover`}
              className="h-full w-full object-cover"
            />
          ) : (
            <Library className="h-8 w-8 text-muted-foreground/40" />
          )}
        </div>

        {/* Text content */}
        <div className="flex-1 min-w-0">
          <Link href={`/collections/${collection.slug}`} className="block group">
            <h3 className="font-bold text-foreground group-hover:text-primary transition-colors line-clamp-2">
              {collection.title}
            </h3>
          </Link>

          <div className="flex items-center gap-1 flex-wrap mt-0.5">
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

          {collection.description && (
            <p
              className={cn(
                'text-sm text-muted-foreground mt-1 line-clamp-3'
              )}
            >
              {collection.description}
            </p>
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
