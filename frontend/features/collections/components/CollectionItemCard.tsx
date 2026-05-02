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
      {/* Single navigation target — image area and title live inside one
          <a> so two links to the same href don't fight strict-mode
          getByRole resolutions (Playwright). Caption + attribution stay
          outside so inline links in markdown notes remain independent. */}
      <Link
        href={entityUrl}
        className="group flex flex-col gap-2 rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={`${item.entity_name} (${typeLabel})`}
      >
        <div
          className={cn(
            'relative block aspect-square overflow-hidden rounded-lg',
            'border border-border/50 bg-muted/40',
            'transition-shadow group-hover:shadow-sm'
          )}
        >
          {hasImage ? (
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
        </div>

        <p
          className={cn(
            'font-medium text-foreground group-hover:text-primary transition-colors',
            'line-clamp-2',
            TITLE_SIZE_CLASSES[density]
          )}
          title={item.entity_name}
          data-testid="collection-item-card-title"
        >
          {item.entity_name}
        </p>
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
