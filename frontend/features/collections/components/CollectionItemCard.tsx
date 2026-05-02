'use client'

/**
 * CollectionItemCard (PSY-360)
 *
 * Visual entity-imagery card used by the grid view of a collection's items
 * list. Square-cropped image (release.cover_art_url, festival.flyer_url) when
 * the backend surfaces one; otherwise a typed Lucide icon centered on a
 * subtle muted background. Caption (notes_html) renders below the card
 * always, so curator commentary is visible at a glance — variable card
 * height is intentional.
 *
 * The list-view alternate continues to use the existing CollectionItemRow
 * inline in CollectionDetail; this component is grid-only.
 */

import Link from 'next/link'
import {
  Library,
  Mic2,
  MapPin,
  Calendar,
  Disc3,
  Tag as TagIcon,
  Tent,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { getEntityUrl, getEntityTypeLabel, type CollectionItem } from '../types'
import { MarkdownContent } from './MarkdownEditor'

/**
 * Lucide icon table per entity type — used as the image fallback when the
 * backend returns no image_url for the entity. Mirrors the table on
 * CollectionDetail and CollectionCard so the same iconography stays
 * recognizable across surfaces.
 */
const ENTITY_ICONS: Record<string, LucideIcon> = {
  artist: Mic2,
  venue: MapPin,
  show: Calendar,
  release: Disc3,
  label: TagIcon,
  festival: Tent,
}

export type CollectionItemCardDensity = 'compact' | 'comfortable' | 'expanded'

interface CollectionItemCardProps {
  item: CollectionItem
  /**
   * Display position number (1-indexed). Only rendered when set; this is
   * how the parent decides whether to show the ranked position badge.
   */
  position?: number
  density: CollectionItemCardDensity
}

/**
 * Density-driven icon sizing. The fallback icon should fill ~half the
 * square's area regardless of column-grid density so it remains visually
 * dominant — keeps the typed-icon fallback feeling intentional rather than
 * placeholder-y.
 */
const ICON_SIZE_CLASSES: Record<CollectionItemCardDensity, string> = {
  compact: 'h-8 w-8',
  comfortable: 'h-12 w-12',
  expanded: 'h-16 w-16',
}

const TITLE_SIZE_CLASSES: Record<CollectionItemCardDensity, string> = {
  compact: 'text-xs',
  comfortable: 'text-sm',
  expanded: 'text-base',
}

export function CollectionItemCard({
  item,
  position,
  density,
}: CollectionItemCardProps) {
  const Icon = ENTITY_ICONS[item.entity_type] ?? Library
  const entityUrl = getEntityUrl(item.entity_type, item.entity_slug)
  const typeLabel = getEntityTypeLabel(item.entity_type)
  const hasImage = Boolean(item.image_url)

  return (
    <article
      className="flex flex-col gap-2"
      data-testid="collection-item-card"
      data-entity-type={item.entity_type}
    >
      {/* Square image / fallback area. The whole tile is a link to the
          entity detail page so the click target is generous on touch. */}
      <Link
        href={entityUrl}
        className={cn(
          'group relative block aspect-square overflow-hidden rounded-lg',
          'border border-border/50 bg-muted/40',
          'transition-shadow hover:shadow-sm',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
        )}
        aria-label={`${item.entity_name} (${typeLabel})`}
      >
        {hasImage ? (
          // Plain <img> matches the CollectionCard / ReleaseCard /
          // existing CollectionDetail patterns. We can't use next/image
          // for arbitrary external URLs (Bandcamp art, festival flyers)
          // without an admin-curated `images.remotePatterns`, which is
          // out of scope for PSY-360.
          /* eslint-disable-next-line @next/next/no-img-element */
          <img
            src={item.image_url ?? ''}
            alt=""
            className="h-full w-full object-cover transition-transform group-hover:scale-[1.02]"
            data-testid="collection-item-card-image"
          />
        ) : (
          <div
            className="flex h-full w-full items-center justify-center"
            data-testid="collection-item-card-fallback"
          >
            <Icon
              className={cn(
                'text-muted-foreground/50',
                ICON_SIZE_CLASSES[density]
              )}
              aria-hidden="true"
            />
          </div>
        )}

        {/* Position badge (top-right). Only rendered for ranked mode where
            the parent passes a 1-indexed position. Semi-transparent dark
            background so it remains legible over both image and icon
            backgrounds. */}
        {position !== undefined && (
          <span
            className={cn(
              'absolute right-1.5 top-1.5 rounded px-1.5 py-0.5',
              'bg-black/60 text-white font-semibold tabular-nums',
              density === 'compact' ? 'text-xs' : 'text-sm'
            )}
            data-testid="collection-item-card-position"
            aria-label={`Position ${position}`}
          >
            {position}
          </span>
        )}

        {/* Entity-type chip (top-left) — small, subtle, lets the visual
            still dominate. Useful when many entity types share a grid. */}
        <span
          className={cn(
            'absolute left-1.5 top-1.5 rounded px-1.5 py-0.5',
            'bg-black/60 text-white font-medium uppercase tracking-wide',
            density === 'compact' ? 'text-[9px]' : 'text-[10px]'
          )}
          aria-hidden="true"
        >
          {typeLabel}
        </span>
      </Link>

      {/* Title — always present below the tile, never inside it. Keeping
          it outside means the image stays a true square regardless of
          how long the entity name is. */}
      <Link
        href={entityUrl}
        className={cn(
          'block font-medium text-foreground hover:text-primary transition-colors',
          'line-clamp-2',
          TITLE_SIZE_CLASSES[density]
        )}
        title={item.entity_name}
      >
        {item.entity_name}
      </Link>

      {/* Caption — server-rendered markdown notes. Always visible (never
          truncated to a single line) so curator commentary is the point of
          the grid view. Falls back to nothing when there are no notes. */}
      {item.notes_html && (
        <MarkdownContent
          html={item.notes_html}
          className="text-xs text-muted-foreground"
          testId={`collection-item-card-notes-${item.id}`}
        />
      )}

      {/* Attribution line — single source of truth for "added by". The
          list-view shows this on its own row; in grid we keep it inline
          but small so it doesn't compete with the image. */}
      <p className="text-[10px] text-muted-foreground/80">
        added by {item.added_by_name}
      </p>
    </article>
  )
}
