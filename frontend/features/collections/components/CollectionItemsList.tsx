'use client'

/**
 * CollectionItemsList — the collection's items grid/list, including the
 * drag-to-reorder machinery (`@dnd-kit/*`, ~87 KB raw).
 *
 * Extracted out of `CollectionDetail.tsx` in PSY-951 so it can be
 * `dynamic()`-imported by the detail component. `@dnd-kit/*` was riding in
 * Turbopack's global shared client chunk (loaded on every route, incl.
 * /explore, which uses no drag-reorder) because `CollectionDetail` is
 * multi-route-reachable via the feature barrel — see the PSY-944 spike. Moving
 * the only `@dnd-kit` consumers (this component + `CollectionItemCard`) behind
 * a lazy boundary, and de-barreling `CollectionDetail`, evicts the lib from the
 * shared chunk into a per-route async chunk.
 *
 * Loaded with `dynamic(ssr:true)` at the call site so the items (the page's
 * primary content) still server-render for SEO/LCP — `@dnd-kit`'s `useSortable`
 * runs fine in disabled mode during SSR (it returns no-op refs). The drag
 * sensors / DndContext only mount client-side when `canReorder`, exactly as
 * before this move. Behavior is unchanged; this is purely a module relocation.
 */

import { useState, useCallback, useMemo } from 'react'
import Link from 'next/link'
import {
  Library,
  GripVertical,
  ChevronUp,
  ChevronDown,
  Pencil,
  Check,
  X,
  Loader2,
  LayoutGrid,
  List,
} from 'lucide-react'
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
  rectSortingStrategy,
  arrayMove,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  useReorderCollectionItems,
  useRemoveCollectionItem,
  useUpdateCollectionItem,
} from '../hooks'
import { cn } from '@/lib/utils'
import {
  getEntityUrl,
  getEntityTypeLabel,
  MAX_COLLECTION_MARKDOWN_LENGTH,
} from '../types'
import type { CollectionDisplayMode, CollectionItem } from '../types'
import { MarkdownContent } from './MarkdownContent'
// Lazily-loaded write-mode editor (keeps marked/dompurify out of the shared
// chunk). See MarkdownEditorLazy / PSY-951.
import { MarkdownEditor } from './MarkdownEditorLazy'
import { CollectionItemCard } from './CollectionItemCard'
import { useDensity, type Density } from '@/lib/hooks/common/useDensity'
import { useLocalStorageEnum } from '@/lib/hooks/common/useLocalStorageEnum'
import { DensityToggle } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  ENTITY_ICONS,
  describeCollectionMutationError,
  MutationFeedback,
  useAutoDismissError,
} from './collectionDetailShared'

// ──────────────────────────────────────────────
// Items List (with reorder support + grid/list view toggle, PSY-360)
// ──────────────────────────────────────────────

/**
 * View-mode for the items list (PSY-360).
 *
 * `grid` — visual entity-imagery cards (CollectionItemCard) in a
 * density-aware responsive grid. Default for new visitors.
 *
 * `list` — the original CollectionItemRow layout: a horizontal row per
 * item with text-first metadata, drag handles for ranked mode, and
 * inline notes editor. Preserved as the alternate so curators who
 * prefer dense scan-and-edit can keep their existing UX.
 */
const VIEW_MODES = ['grid', 'list'] as const
type CollectionItemsViewMode = (typeof VIEW_MODES)[number]

const VIEW_MODE_STORAGE_KEY = 'ph-collection-items-view-mode'

/**
 * Density-driven column counts for the grid view (PSY-360). Mirrors the
 * compact/comfortable/expanded scale used by other browse pages
 * (ArtistList, ShowList, ReleaseList) but tightened up: collection items
 * are smaller than full browse cards because the user is in
 * collection-context (already drilled in) and wants to see more at a
 * glance.
 */
const GRID_COLUMN_CLASSES: Record<Density, string> = {
  compact: 'grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 gap-3',
  comfortable: 'grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4',
  expanded: 'grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-5',
}

export function CollectionItemsList({
  items,
  slug,
  isCreator,
  displayMode,
}: {
  items: CollectionItem[]
  slug: string
  isCreator: boolean
  displayMode: CollectionDisplayMode
}) {
  const reorderMutation = useReorderCollectionItems()
  const isRanked = displayMode === 'ranked'
  // Reordering only makes sense in ranked mode and only for creators.
  const canReorder = isCreator && isRanked

  // PSY-609: drag-drop and arrow-key reorder were silent on failure — the
  // mutation has no optimistic update so a 4xx left the items in their
  // original order with no explanation. Auto-dismiss after ~3s so the
  // banner doesn't sit around once the user has registered the failure.
  const formatReorderError = useCallback(
    (err: unknown) =>
      describeCollectionMutationError(err, 'Failed to save the new order.'),
    []
  )
  const reorderError = useAutoDismissError(
    reorderMutation.error,
    reorderMutation.isError,
    formatReorderError
  )

  // Density preference for the grid view. List view ignores density (its
  // layout is intentionally fixed). Storage key matches the hook's prefix
  // convention (ph-density-collections).
  const { density, setDensity } = useDensity('collections')

  // View-mode preference (grid vs list). Default `grid` so first-time viewers
  // see the visual layout. Persists per-browser. `useLocalStorageEnum` returns
  // the default on the server + first hydration so the public SSR boundary
  // never trips a React mismatch when the stored preference is `list`.
  const [viewMode, setViewMode] = useLocalStorageEnum<CollectionItemsViewMode>(
    VIEW_MODE_STORAGE_KEY,
    'grid',
    VIEW_MODES
  )

  const persistOrder = useCallback(
    (orderedItems: CollectionItem[]) => {
      const reorderPayload = orderedItems.map((item, i) => ({
        item_id: item.id,
        position: i,
      }))
      reorderMutation.mutate({ slug, items: reorderPayload })
    },
    [slug, reorderMutation]
  )

  const handleMoveUp = useCallback(
    (index: number) => {
      if (index <= 0) return
      const newItems = [...items]
      ;[newItems[index - 1], newItems[index]] = [newItems[index], newItems[index - 1]]
      persistOrder(newItems)
    },
    [items, persistOrder]
  )

  const handleMoveDown = useCallback(
    (index: number) => {
      if (index >= items.length - 1) return
      const newItems = [...items]
      ;[newItems[index], newItems[index + 1]] = [newItems[index + 1], newItems[index]]
      persistOrder(newItems)
    },
    [items, persistOrder]
  )

  // dnd-kit sensors:
  // - PointerSensor with a small distance prevents drag from triggering on click.
  // - TouchSensor with delay ⇒ long-press initiates drag on mobile (PSY-348).
  // - KeyboardSensor pairs with sortableKeyboardCoordinates so focusable drag
  //   handles support arrow-key reordering as a fallback.
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 200, tolerance: 8 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  )

  const itemIds = useMemo(() => items.map((item) => item.id), [items])

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event
      if (!over || active.id === over.id) return
      const oldIndex = items.findIndex((item) => item.id === active.id)
      const newIndex = items.findIndex((item) => item.id === over.id)
      if (oldIndex === -1 || newIndex === -1) return
      const reordered = arrayMove(items, oldIndex, newIndex)
      persistOrder(reordered)
    },
    [items, persistOrder]
  )

  if (items.length === 0) {
    return (
      <div>
        <h2 className="text-lg font-semibold mb-4">Items</h2>
        <div className="text-center py-12 text-muted-foreground">
          <Library className="h-12 w-12 mx-auto mb-3 text-muted-foreground/30" />
          <p>
            {isCreator
              ? 'Add your first item using the search above.'
              : 'This collection is empty.'}
          </p>
        </div>
      </div>
    )
  }

  // Container layout depends on view + display mode:
  // - grid view  → density-driven responsive grid of CollectionItemCard
  // - list view + ranked → vertical stack (numbering reads top-to-bottom)
  // - list view + unranked → 2-up text-row grid (legacy compact layout)
  const isGridView = viewMode === 'grid'
  const containerClasses = isGridView
    ? GRID_COLUMN_CLASSES[density]
    : isRanked
      ? 'space-y-2'
      : 'grid grid-cols-1 sm:grid-cols-2 gap-2'

  // Drag-drop strategy: rect for grid (2-D adjacency), vertical for list.
  // Ranked + grid is uncommon but legal; this avoids the foot-gun of using
  // the vertical strategy in a 2-D layout (drop hit-testing breaks down).
  const sortStrategy = isGridView
    ? rectSortingStrategy
    : verticalListSortingStrategy

  const renderListRows = () =>
    items.map((item, index) => (
      <CollectionItemRow
        key={item.id}
        item={item}
        position={index + 1}
        index={index}
        totalItems={items.length}
        slug={slug}
        isCreator={isCreator}
        isRanked={isRanked}
        canReorder={canReorder}
        onMoveUp={handleMoveUp}
        onMoveDown={handleMoveDown}
        isReordering={reorderMutation.isPending}
      />
    ))

  const renderGridCards = () =>
    items.map((item, index) => (
      <CollectionItemCard
        key={item.id}
        item={item}
        position={isRanked ? index + 1 : undefined}
        density={density}
        isCreator={isCreator}
        slug={slug}
        reorder={
          canReorder
            ? {
                index,
                totalItems: items.length,
                onMoveUp: handleMoveUp,
                onMoveDown: handleMoveDown,
                isPending: reorderMutation.isPending,
              }
            : undefined
        }
      />
    ))

  const renderItems = isGridView ? renderGridCards : renderListRows

  // Header row: section title on the left, view + density toggles on the
  // right. Density toggle stays mounted in list view so the toolbar
  // doesn't shift between modes (PSY-556); it's disabled there with a
  // tooltip explaining the constraint. The persisted selection is
  // preserved so toggling back to grid restores the user's choice.
  const header = (
    <div className="mb-4 flex items-center justify-between gap-3 flex-wrap">
      <h2 className="text-lg font-semibold">Items</h2>
      <div className="flex items-center gap-2">
        <DensityToggle
          density={density}
          onDensityChange={setDensity}
          disabled={!isGridView}
          disabledTooltip="Density only applies to grid view"
        />
        <div
          className="inline-flex items-center rounded-lg border border-border/50 bg-muted/30 p-0.5"
          role="radiogroup"
          aria-label="Items view"
          data-testid="collection-items-view-toggle"
        >
          <button
            type="button"
            role="radio"
            aria-checked={viewMode === 'grid'}
            aria-label="Grid view"
            onClick={() => setViewMode('grid')}
            className={cn(
              'flex items-center justify-center h-7 w-7 rounded-md transition-colors',
              viewMode === 'grid'
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
            data-testid="view-mode-grid"
          >
            <LayoutGrid className="h-4 w-4" />
          </button>
          <button
            type="button"
            role="radio"
            aria-checked={viewMode === 'list'}
            aria-label="List view"
            onClick={() => setViewMode('list')}
            className={cn(
              'flex items-center justify-center h-7 w-7 rounded-md transition-colors',
              viewMode === 'list'
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
            data-testid="view-mode-list"
          >
            <List className="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  )

  return (
    <div>
      {header}
      {/*
        PSY-609: surface drag-drop / arrow-key reorder failures. The
        useReorderCollectionItems mutation has no optimistic update, so a
        rejected request leaves the items in their original order with no
        feedback. Auto-dismiss the banner after ~3s.
      */}
      {reorderError && (
        <MutationFeedback
          variant="error"
          testId="reorder-error"
          message={reorderError}
        />
      )}
      {canReorder ? (
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragEnd={handleDragEnd}
        >
          <SortableContext items={itemIds} strategy={sortStrategy}>
            <div
              className={containerClasses}
              data-testid="collection-items"
              data-view-mode={viewMode}
            >
              {renderItems()}
            </div>
          </SortableContext>
        </DndContext>
      ) : (
        <div
          className={containerClasses}
          data-testid="collection-items"
          data-view-mode={viewMode}
        >
          {renderItems()}
        </div>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Item Row
// ──────────────────────────────────────────────

function CollectionItemRow({
  item,
  position,
  index,
  totalItems,
  slug,
  isCreator,
  isRanked,
  canReorder,
  onMoveUp,
  onMoveDown,
  isReordering,
}: {
  item: CollectionItem
  position: number
  index: number
  totalItems: number
  slug: string
  isCreator: boolean
  isRanked: boolean
  canReorder: boolean
  onMoveUp: (index: number) => void
  onMoveDown: (index: number) => void
  isReordering: boolean
}) {
  const removeMutation = useRemoveCollectionItem()
  const updateMutation = useUpdateCollectionItem()
  const [isEditingNotes, setIsEditingNotes] = useState(false)
  const [notesValue, setNotesValue] = useState(item.notes ?? '')
  const [showRemoveConfirm, setShowRemoveConfirm] = useState(false)
  const Icon = ENTITY_ICONS[item.entity_type] ?? Library

  // useSortable returns no-op refs/listeners when not registered with a
  // DndContext (e.g. unranked mode). Always calling it keeps hook order stable.
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

  const handleRemove = () => {
    removeMutation.mutate(
      { slug, itemId: item.id },
      { onSuccess: () => setShowRemoveConfirm(false) }
    )
  }

  const handleSaveNotes = () => {
    const trimmed = notesValue.trim()
    updateMutation.mutate(
      { slug, itemId: item.id, notes: trimmed || null },
      {
        onSuccess: () => {
          setIsEditingNotes(false)
        },
      }
    )
  }

  const handleCancelNotes = () => {
    setNotesValue(item.notes ?? '')
    setIsEditingNotes(false)
  }

  return (
    <div
      ref={canReorder ? setNodeRef : undefined}
      style={sortableStyle}
      className="rounded-lg border border-border/50 bg-card p-3"
    >
      <div className="flex items-center gap-3">
        {/* Drag handle + keyboard reorder fallback (ranked mode, creator only) */}
        {canReorder && (
          <div className="flex items-center gap-1 shrink-0">
            <button
              type="button"
              {...attributes}
              {...listeners}
              className="touch-none cursor-grab active:cursor-grabbing h-7 w-5 flex items-center justify-center text-muted-foreground hover:text-foreground rounded focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label={`Drag to reorder ${item.entity_name}. Use space to lift, arrow keys to move.`}
              data-testid="drag-handle"
            >
              <GripVertical className="h-4 w-4" />
            </button>
            <div className="flex flex-col">
              <Button
                variant="ghost"
                size="sm"
                className="h-5 w-5 p-0 text-muted-foreground hover:text-foreground"
                onClick={() => onMoveUp(index)}
                disabled={index === 0 || isReordering}
                aria-label={`Move ${item.entity_name} up`}
              >
                <ChevronUp className="h-3.5 w-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-5 w-5 p-0 text-muted-foreground hover:text-foreground"
                onClick={() => onMoveDown(index)}
                disabled={index === totalItems - 1 || isReordering}
                aria-label={`Move ${item.entity_name} down`}
              >
                <ChevronDown className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        )}

        {/* Position number — only meaningful in ranked mode */}
        {isRanked && (
          <span className="text-sm font-medium text-muted-foreground/60 w-6 text-right shrink-0">
            {position}
          </span>
        )}

        {/* Entity type icon */}
        <div className="h-8 w-8 shrink-0 rounded-md bg-muted/50 flex items-center justify-center">
          <Icon className="h-4 w-4 text-muted-foreground/60" />
        </div>

        {/* Entity info */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <Link
              href={getEntityUrl(item.entity_type, item.entity_slug)}
              className="font-medium text-foreground hover:text-primary transition-colors truncate"
            >
              {item.entity_name}
            </Link>
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
              {getEntityTypeLabel(item.entity_type)}
            </Badge>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
            <span>added by {item.added_by_name}</span>
          </div>
          {/* PSY-349: render notes as sanitized markdown HTML (server-rendered).
              Display only when not editing; legacy plain-text notes still
              render correctly because plain text is valid markdown. */}
          {!isEditingNotes && item.notes_html && (
            <MarkdownContent
              html={item.notes_html}
              className="mt-1 text-xs text-muted-foreground"
              testId={`collection-item-notes-${item.id}`}
            />
          )}
        </div>

        {/* Action buttons (creator only) */}
        {isCreator && (
          <div className="flex items-center gap-1 shrink-0">
            {/* Edit notes button */}
            {!isEditingNotes && (
              <Button
                variant="ghost"
                size="sm"
                className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
                onClick={() => {
                  setNotesValue(item.notes ?? '')
                  setIsEditingNotes(true)
                }}
                aria-label={`Edit notes for ${item.entity_name}`}
              >
                <Pencil className="h-3.5 w-3.5" />
              </Button>
            )}

            {/* Remove button */}
            {!showRemoveConfirm ? (
              <Button
                variant="ghost"
                size="sm"
                className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                onClick={() => setShowRemoveConfirm(true)}
                disabled={removeMutation.isPending}
                aria-label={`Remove ${item.entity_name} from collection`}
              >
                <X className="h-4 w-4" />
              </Button>
            ) : (
              <div className="flex items-center gap-1">
                <Button
                  variant="destructive"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={handleRemove}
                  disabled={removeMutation.isPending}
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
                  onClick={() => setShowRemoveConfirm(false)}
                  disabled={removeMutation.isPending}
                >
                  Cancel
                </Button>
              </div>
            )}
          </div>
        )}
      </div>

      {/*
        PSY-609: surface remove failures inline so the user knows their
        click didn't take effect. Sticky (no auto-dismiss) until the
        confirmation flow is dismissed — once the user clicks Cancel or
        Remove again, a fresh attempt clears the error via the mutation's
        own state transition.
      */}
      {removeMutation.isError && (
        <MutationFeedback
          variant="error"
          testId={`remove-error-${item.id}`}
          message={describeCollectionMutationError(
            removeMutation.error,
            'Failed to remove this item.'
          )}
        />
      )}

      {/* Inline notes editor (PSY-349: markdown w/ preview toggle) */}
      {isEditingNotes && isCreator && (
        <div className="mt-2 ml-[4.25rem] space-y-2">
          <MarkdownEditor
            value={notesValue}
            onChange={setNotesValue}
            placeholder="Add a note about this item... (markdown supported)"
            rows={2}
            maxLength={MAX_COLLECTION_MARKDOWN_LENGTH}
            ariaLabel="Notes for this collection item"
            autoFocus
            testId={`collection-item-notes-editor-${item.id}`}
          />
          {updateMutation.isError && (
            <p className="text-xs text-destructive">
              {updateMutation.error instanceof Error
                ? updateMutation.error.message
                : 'Failed to update notes'}
            </p>
          )}
          <div className="flex gap-2">
            <Button
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={handleSaveNotes}
              disabled={updateMutation.isPending}
            >
              {updateMutation.isPending ? (
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
              ) : (
                <Check className="h-3 w-3 mr-1" />
              )}
              Save
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="h-7 px-2 text-xs"
              onClick={handleCancelNotes}
              disabled={updateMutation.isPending}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
