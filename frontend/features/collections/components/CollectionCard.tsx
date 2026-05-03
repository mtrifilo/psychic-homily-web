'use client'

import Link from 'next/link'
import {
  Library,
  Users,
  Star,
  Clock,
  Heart,
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
import { CollectionCoverImage } from './CollectionCoverImage'
import { useLikeCollection, useUnlikeCollection } from '../hooks'
import { useAuthContext } from '@/lib/context/AuthContext'

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
  const { isAuthenticated } = useAuthContext()
  const likeMutation = useLikeCollection()
  const unlikeMutation = useUnlikeCollection()

  const handleToggleLike = (e: React.MouseEvent) => {
    // Card body is wrapped in a link; stop propagation so clicking the
    // heart doesn't navigate to the detail page.
    e.preventDefault()
    e.stopPropagation()
    if (collection.user_likes_this) {
      unlikeMutation.mutate({ slug: collection.slug })
    } else {
      likeMutation.mutate({ slug: collection.slug })
    }
  }

  const isLikePending = likeMutation.isPending || unlikeMutation.isPending

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
        {/* Cover image with onError-driven fallback to entity-type mosaic
            (or a single Library icon when no entity types are present).
            PSY-554: a 404 on cover_image_url no longer leaves the tile
            blank — it falls through to the same mosaic the null-URL case
            already used. */}
        <CollectionCoverImage
          url={collection.cover_image_url}
          alt={`${collection.title} cover`}
          className="h-16 w-16 shrink-0 rounded-md bg-muted/50"
          fallback={
            mosaicTypes.length > 0 ? (
              <div
                className={cn(
                  'grid gap-0.5 p-1.5',
                  mosaicTypes.length === 1 ? 'grid-cols-1' : 'grid-cols-2'
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
            )
          }
        />

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

          {/* PSY-354: tag chips. Cap at 5 visible to keep cards readable;
              the detail page shows the full set. Each chip links to the
              tag-filtered collections browse — the ticket explicitly
              chose this over the global /tags/{slug} target so chips on
              cards behave like a "show me other collections like this"
              shortcut rather than a deep-dive into the tag's full corpus. */}
          {collection.tags && collection.tags.length > 0 && (
            <div
              className="mt-1.5 flex flex-wrap gap-1"
              data-testid="collection-card-tags"
            >
              {collection.tags.slice(0, 5).map((tag) => (
                <Link
                  key={tag.id}
                  href={`/collections?tag=${encodeURIComponent(tag.slug)}`}
                  onClick={(e) => e.stopPropagation()}
                  className={cn(
                    'inline-flex items-center rounded-full border px-2 py-0.5',
                    'text-[10px] font-medium transition-colors',
                    'border-border/60 bg-muted/30 text-muted-foreground',
                    'hover:border-primary/40 hover:bg-primary/10 hover:text-primary'
                  )}
                  title={tag.name}
                >
                  {tag.name}
                </Link>
              ))}
              {collection.tags.length > 5 && (
                <span className="text-[10px] text-muted-foreground self-center">
                  +{collection.tags.length - 5}
                </span>
              )}
            </div>
          )}

          <div className="mt-1.5 flex items-center gap-3 text-xs text-muted-foreground flex-wrap">
            <span>
              by{' '}
              {collection.creator_username ? (
                <Link
                  href={`/users/${collection.creator_username}`}
                  className="text-foreground hover:underline"
                >
                  {collection.creator_name}
                </Link>
              ) : (
                collection.creator_name
              )}
            </span>
            <span className="flex items-center gap-1">
              <Library className="h-3 w-3" />
              {collection.item_count === 1
                ? '1 item'
                : `${collection.item_count} items`}
            </span>
            {/* PSY-353: surface community curation when at least 3
                contributors have added items. Threshold matches What.cd's
                min-3-items spirit; below it, attribution is just the
                creator. */}
            {collection.contributor_count >= 3 && (
              <span
                className="flex items-center gap-1"
                data-testid="contributor-badge"
              >
                <Users className="h-3 w-3" />
                Built by {collection.contributor_count} contributors
              </span>
            )}
            {collection.subscriber_count > 0 && (
              <span className="flex items-center gap-1">
                <Users className="h-3 w-3" />
                {collection.subscriber_count === 1
                  ? '1 subscriber'
                  : `${collection.subscriber_count} subscribers`}
              </span>
            )}
            {/* PSY-352: heart + like count. Authenticated users get a
                clickable toggle (filled when liked); anonymous users see
                a static count + outline heart so they know the signal
                exists but can't yet act on it. Hide the row when the count
                is zero AND the viewer can't add — keeps the card quiet on
                fresh collections. */}
            {(collection.like_count > 0 || isAuthenticated) &&
              (isAuthenticated ? (
                <button
                  type="button"
                  onClick={handleToggleLike}
                  disabled={isLikePending}
                  aria-pressed={collection.user_likes_this ?? false}
                  aria-label={
                    collection.user_likes_this
                      ? 'Unlike collection'
                      : 'Like collection'
                  }
                  className={cn(
                    'flex items-center gap-1 transition-colors',
                    'hover:text-primary',
                    collection.user_likes_this && 'text-primary'
                  )}
                  data-testid="collection-like-button"
                >
                  <Heart
                    className={cn(
                      'h-3 w-3',
                      collection.user_likes_this && 'fill-current'
                    )}
                  />
                  <span>{collection.like_count}</span>
                </button>
              ) : (
                <span
                  className="flex items-center gap-1"
                  data-testid="collection-like-count"
                >
                  <Heart className="h-3 w-3" />
                  {collection.like_count}
                </span>
              ))}
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
