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
 *
 * PSY-526: creators get a Remove control overlaid on the image area.
 * Hybrid pattern — hover-revealed corner X on desktop (uses Tailwind v4's
 * media-gated `group-hover:`), always-visible kebab on touch devices
 * (uses the `touch-only:` custom variant defined in globals.css). Both
 * paths drive the same two-step confirmation flow that mirrors
 * CollectionItemRow's existing `useRemoveCollectionItem` UX.
 */

import { useState, useRef, useEffect } from 'react'
import Link from 'next/link'
import {
  Library,
  Mic2,
  MapPin,
  Calendar,
  Disc3,
  Tag as TagIcon,
  Tent,
  X,
  MoreVertical,
  Loader2,
  GripVertical,
  ChevronUp,
  ChevronDown,
  AlertCircle,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { cn } from '@/lib/utils'
import { getEntityUrl, getEntityTypeLabel, type CollectionItem } from '../types'
import { MarkdownContent } from './MarkdownEditor'
import { Button } from '@/components/ui/button'
import { useRemoveCollectionItem } from '../hooks'

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
  /** 1-indexed display position. Renders the ranked position badge when set. */
  position?: number
  density: CollectionItemCardDensity
  isCreator?: boolean
  /** Required when isCreator — used by the Remove mutation. */
  slug?: string
  /**
   * Drag + keyboard reorder wiring. When set, the card registers with the
   * parent SortableContext and renders the reorder cluster. When omitted,
   * useSortable still runs (in disabled mode) to keep React hook order
   * stable across reorder-eligibility transitions.
   */
  reorder?: {
    index: number
    totalItems: number
    onMoveUp: (index: number) => void
    onMoveDown: (index: number) => void
    isPending?: boolean
  }
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
  isCreator = false,
  slug,
  reorder,
}: CollectionItemCardProps) {
  const Icon = ENTITY_ICONS[item.entity_type] ?? Library
  const entityUrl = getEntityUrl(item.entity_type, item.entity_slug)
  const typeLabel = getEntityTypeLabel(item.entity_type)
  const hasImage = Boolean(item.image_url)
  const canReorder = Boolean(reorder)

  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: item.id, disabled: !canReorder })

  const sortableStyle: React.CSSProperties = canReorder
    ? {
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.6 : undefined,
      }
    : {}

  return (
    <article
      ref={canReorder ? setNodeRef : undefined}
      style={sortableStyle}
      className="relative flex flex-col gap-2"
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
                density === 'compact' ? 'text-xs' : 'text-sm',
                // PSY-526: when the creator hovers (and the corner X
                // appears in the same corner), fade the position badge
                // out so the X has a clean spot. `group-hover:` only
                // fires on hover-capable pointers (Tailwind v4 default),
                // so touch-only users continue to see the badge — they
                // see the kebab in the same corner but with a slight
                // visual stack (kebab ~24px, badge ~30px wide).
                // Non-creators don't get the corner X, so we gate this
                // on isCreator to avoid an inexplicable fade-on-hover.
                isCreator && 'transition-opacity group-hover:opacity-0'
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

      {/* PSY-526: creator-only Remove control. Lives outside the wrapping
          <Link> (rendered as a sibling, absolutely positioned over the
          image's top-right corner) so a) it isn't part of the link's
          accessible name, and b) the <button> isn't nested inside <a>
          (invalid HTML). The control occupies the same corner as the
          ranked-position badge; on hover-capable devices the badge
          fades via group-hover and the X takes its spot. On touch
          devices the kebab and badge stack lightly (kebab ~24px,
          badge ~30px) — readable, with the kebab visually on top. */}
      {isCreator && slug && (
        <CollectionItemCardRemoveControl
          slug={slug}
          itemId={item.id}
          entityName={item.entity_name}
        />
      )}

      {/* Sibling of <Link> (PSY-526 pattern) to avoid <button> in <a>. In
          flow rather than overlaid because the image-area bottom can't be
          reached from outside the Link without JS measurement. */}
      {reorder && (
        <div
          className="self-start flex items-center gap-0.5 rounded-md border border-border/50 bg-background/80 p-0.5"
          data-testid="collection-item-card-reorder"
        >
          <button
            type="button"
            {...attributes}
            {...listeners}
            className="touch-none cursor-grab active:cursor-grabbing h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            aria-label={`Drag to reorder ${item.entity_name}. Use space to lift, arrow keys to move.`}
            title="Drag to reorder"
            data-testid="collection-item-card-drag-handle"
          >
            <GripVertical className="h-3.5 w-3.5" />
          </button>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
            onClick={() => reorder.onMoveUp(reorder.index)}
            disabled={reorder.index === 0 || reorder.isPending}
            title="Move up"
            aria-label="Move up"
          >
            <ChevronUp className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground"
            onClick={() => reorder.onMoveDown(reorder.index)}
            disabled={reorder.index === reorder.totalItems - 1 || reorder.isPending}
            title="Move down"
            aria-label="Move down"
          >
            <ChevronDown className="h-3.5 w-3.5" />
          </Button>
        </div>
      )}

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

/**
 * Internal Remove control for grid-view cards. Renders three states:
 *
 *   1. Idle  — desktop X (hover-revealed) + touch kebab (always visible).
 *   2. Touch popover open — small inline popover anchored to the kebab,
 *      with a single "Remove" action inside. Tapping anywhere else closes
 *      it (handled by document-level pointerdown listener).
 *   3. Confirm — destructive "Remove" + ghost "Cancel" overlaid on the
 *      image's top-right corner; same UX as CollectionItemRow.
 *
 * The two-step pattern (idle → confirm → mutation) is intentional. We
 * mirror the list-view row exactly so muscle memory and Linear screenshots
 * stay consistent.
 */
function CollectionItemCardRemoveControl({
  slug,
  itemId,
  entityName,
}: {
  slug: string
  itemId: number
  entityName: string
}) {
  const removeMutation = useRemoveCollectionItem()
  const [showRemoveConfirm, setShowRemoveConfirm] = useState(false)
  const [showTouchMenu, setShowTouchMenu] = useState(false)
  const containerRef = useRef<HTMLDivElement | null>(null)

  // Close the touch popover when the user taps outside it. Pointerdown
  // (rather than click) so the popover dismisses before any nested
  // <Link> click handler can fire — keeps the dismiss-by-tap-elsewhere
  // gesture from accidentally navigating to the entity page.
  useEffect(() => {
    if (!showTouchMenu) return
    const onPointerDown = (e: PointerEvent) => {
      if (!containerRef.current) return
      if (containerRef.current.contains(e.target as Node)) return
      setShowTouchMenu(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    return () => document.removeEventListener('pointerdown', onPointerDown)
  }, [showTouchMenu])

  const handleRemove = () => {
    removeMutation.mutate(
      { slug, itemId },
      {
        onSuccess: () => {
          setShowRemoveConfirm(false)
          setShowTouchMenu(false)
        },
      }
    )
  }

  // All click handlers stop propagation + prevent default so a tap on the
  // control never bubbles into the wrapping <Link>'s navigation. This
  // matters because the Link covers most of the card; a stray click from
  // the popover overlay would otherwise navigate to the entity page.
  const stop = (e: React.MouseEvent | React.PointerEvent) => {
    e.preventDefault()
    e.stopPropagation()
  }

  return (
    <div
      ref={containerRef}
      // Absolutely positioned over the image's top-right. The article is
      // `flex flex-col` with the image as the first child, so top:0 of the
      // article aligns with top:0 of the image. We inset by 1.5 (matching
      // the position badge / type label) so the control sits inside the
      // rounded-lg corner.
      className="absolute right-1.5 top-1.5 z-10"
      onPointerDown={stop}
    >
      {showRemoveConfirm ? (
        <div className="flex flex-col items-end gap-1">
          <div className="flex items-center gap-1 rounded-md bg-background/95 p-1 shadow-md ring-1 ring-border">
            <Button
              variant="destructive"
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={(e) => {
                stop(e)
                handleRemove()
              }}
              disabled={removeMutation.isPending}
              data-testid="collection-item-card-remove-confirm"
            >
              {removeMutation.isPending ? (
                <Loader2 className="h-3 w-3 animate-spin" />
              ) : (
                'Remove'
              )}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={(e) => {
                stop(e)
                setShowRemoveConfirm(false)
              }}
              disabled={removeMutation.isPending}
            >
              Cancel
            </Button>
          </div>
          {/*
            PSY-609: surface the remove failure inline so the user knows
            the click didn't take effect. Sticky while the confirm UI is
            open; clears as soon as the user clicks Remove again or
            Cancels (mutation state transitions to pending/idle).
          */}
          {removeMutation.isError && (
            <div
              role="status"
              data-testid={`collection-item-card-remove-error-${itemId}`}
              className={cn(
                'flex max-w-[14rem] items-start gap-1 rounded-md',
                'bg-background/95 px-2 py-1 text-[11px] text-destructive',
                'shadow-md ring-1 ring-destructive/40'
              )}
            >
              <AlertCircle
                className="h-3 w-3 mt-0.5 shrink-0"
                aria-hidden="true"
              />
              <span className="flex-1">
                {removeMutation.error instanceof Error &&
                removeMutation.error.message
                  ? removeMutation.error.message
                  : 'Failed to remove this item.'}
              </span>
            </div>
          )}
        </div>
      ) : (
        <>
          {/* Desktop trigger — hover-revealed X. `group-hover:` only fires
              on hover-capable pointers (Tailwind v4 default), and we
              explicitly hide it on touch-only devices via the
              `touch-only:` custom variant so a sleeping touch device
              doesn't briefly flash the X on render. */}
          <button
            type="button"
            onClick={(e) => {
              stop(e)
              setShowRemoveConfirm(true)
            }}
            disabled={removeMutation.isPending}
            title="Remove from collection"
            aria-label={`Remove ${entityName} from collection`}
            data-testid="collection-item-card-remove"
            className={cn(
              'flex h-6 w-6 items-center justify-center rounded-md',
              'bg-background/90 text-muted-foreground shadow-sm ring-1 ring-border',
              'transition-opacity hover:text-destructive focus:text-destructive',
              'opacity-0 group-hover:opacity-100 focus-visible:opacity-100',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              'touch-only:hidden'
            )}
          >
            <X className="h-3.5 w-3.5" />
          </button>

          {/* Touch trigger — always-visible kebab. Hidden on hover-capable
              devices so desktop users don't see two redundant controls.
              Opens an inline popover; the popover's only action is
              Remove, which transitions to the same confirm UI desktop
              uses. We could expand this later (Edit notes, Move, etc.)
              without changing the API surface — the entry point is
              already a menu. */}
          <button
            type="button"
            onClick={(e) => {
              stop(e)
              setShowTouchMenu((prev) => !prev)
            }}
            disabled={removeMutation.isPending}
            aria-label={`Item actions for ${entityName}`}
            aria-expanded={showTouchMenu}
            aria-haspopup="menu"
            data-testid="collection-item-card-actions"
            className={cn(
              'hidden h-6 w-6 items-center justify-center rounded-md',
              'bg-background/90 text-muted-foreground shadow-sm ring-1 ring-border',
              'transition-colors hover:text-foreground',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              'touch-only:flex'
            )}
          >
            <MoreVertical className="h-3.5 w-3.5" />
          </button>

          {showTouchMenu && (
            <div
              role="menu"
              aria-label="Item actions"
              className={cn(
                'absolute right-0 top-7 z-20 min-w-[10rem] rounded-md',
                'bg-background shadow-md ring-1 ring-border',
                'p-1'
              )}
            >
              <button
                type="button"
                role="menuitem"
                onClick={(e) => {
                  stop(e)
                  setShowTouchMenu(false)
                  setShowRemoveConfirm(true)
                }}
                // No `title` here on purpose — the visible text is the
                // accessible name. Sharing `title="Remove from collection"`
                // with the desktop X would duplicate the smoke-test
                // selector once the kebab path is added to the spec.
                data-testid="collection-item-card-remove-menu-item"
                className={cn(
                  'flex w-full items-center gap-2 rounded-sm px-2 py-1.5',
                  'text-left text-sm text-foreground',
                  'hover:bg-muted focus:bg-muted',
                  'focus:outline-none'
                )}
              >
                <X className="h-3.5 w-3.5 text-destructive" />
                <span>Remove from collection</span>
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
