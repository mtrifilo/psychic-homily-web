'use client'

import { useState, useCallback, useMemo } from 'react'
import Link from 'next/link'
import {
  Loader2,
  Library,
  Users,
  Star,
  Bell,
  BellOff,
  Pencil,
  Check,
  X,
  Trash2,
  Mic2,
  MapPin,
  Calendar,
  Disc3,
  Tag,
  Tent,
  Plus,
  Search,
  ChevronUp,
  ChevronDown,
  Share2,
  GitFork,
  GripVertical,
  Heart,
  ListOrdered,
  LayoutGrid,
  List,
  Network,
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  useCollection,
  useUpdateCollection,
  useAddCollectionItem,
  useRemoveCollectionItem,
  useReorderCollectionItems,
  useUpdateCollectionItem,
  useSubscribeCollection,
  useUnsubscribeCollection,
  useDeleteCollection,
  useCloneCollection,
  useLikeCollection,
  useUnlikeCollection,
} from '../hooks'
import { cn } from '@/lib/utils'
import {
  getEntityUrl,
  getEntityTypeLabel,
  MAX_COLLECTION_MARKDOWN_LENGTH,
  MAX_COVER_IMAGE_URL_LENGTH,
  MIN_PUBLIC_COLLECTION_ITEMS,
  MIN_PUBLIC_COLLECTION_DESCRIPTION_CHARS,
  validateCoverImageUrl,
} from '../types'
import type { CollectionDisplayMode, CollectionItem, CollectionDetail as CollectionDetailType } from '../types'
import { MarkdownEditor, MarkdownContent } from './MarkdownEditor'
import { CollectionGraph } from './CollectionGraph'
import { CollectionItemCard } from './CollectionItemCard'
import { CollectionCoverImage } from './CollectionCoverImage'
import { useDensity, type Density } from '@/lib/hooks/common/useDensity'
import { GRAPH_HASH, useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { DensityToggle } from '@/components/shared'
import { useEntitySearch } from '@/lib/hooks/common/useEntitySearch'
import type { EntitySearchResult } from '@/lib/hooks/common/useEntitySearch'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Breadcrumb } from '@/components/shared'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useRouter } from 'next/navigation'
import type { ApiError } from '@/lib/api'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { CommentThread } from '@/features/comments'
import { EntityTagList } from '@/features/tags'

interface CollectionDetailProps {
  slug: string
}

const ENTITY_ICONS: Record<string, React.ElementType> = {
  artist: Mic2,
  venue: MapPin,
  show: Calendar,
  release: Disc3,
  label: Tag,
  festival: Tent,
}

/**
 * PSY-356: curator-only banner shown on a collection's detail page when it
 * fails the public-visibility gate (>= 3 items AND >= 50-char description).
 * Copy enumerates only the missing pieces and changes wording based on
 * whether the collection is currently public (and thus dropped from browse)
 * or still private (and warned about pre-publish).
 *
 * Hidden when:
 *   - Caller is not the creator (other viewers should not see this).
 *   - The gate passes.
 */
function PublishGateBanner({ collection }: { collection: CollectionDetailType }) {
  const itemsBelow = collection.item_count < MIN_PUBLIC_COLLECTION_ITEMS
  const descriptionLength = collection.description?.length ?? 0
  const descBelow = descriptionLength < MIN_PUBLIC_COLLECTION_DESCRIPTION_CHARS
  if (!itemsBelow && !descBelow) {
    return null
  }

  const itemsNeeded = Math.max(0, MIN_PUBLIC_COLLECTION_ITEMS - collection.item_count)
  const itemsClause =
    itemsNeeded === 1
      ? '1 more item'
      : `${itemsNeeded} more items`
  const descriptionClause =
    descriptionLength === 0
      ? `a description of at least ${MIN_PUBLIC_COLLECTION_DESCRIPTION_CHARS} characters`
      : `a longer description (${MIN_PUBLIC_COLLECTION_DESCRIPTION_CHARS}+ characters)`

  let needsCopy: string
  if (itemsBelow && descBelow) {
    needsCopy = `${itemsClause} and ${descriptionClause}`
  } else if (itemsBelow) {
    needsCopy = itemsClause
  } else {
    needsCopy = descriptionClause
  }

  const message = collection.is_public
    ? `Below current quality standards — your collection isn't appearing in the public browse. Add ${needsCopy} to fix this.`
    : `Before publishing, this collection needs ${needsCopy}. Public collections must meet these standards.`

  return (
    <Alert className="mb-4" data-testid="publish-gate-banner">
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  )
}

export function CollectionDetail({ slug }: CollectionDetailProps) {
  const router = useRouter()
  const { user, isAuthenticated } = useAuthContext()
  const { data: collection, isLoading, error } = useCollection(slug)
  const subscribeMutation = useSubscribeCollection()
  const unsubscribeMutation = useUnsubscribeCollection()
  const deleteMutation = useDeleteCollection()
  // PSY-351: clone an existing collection. On success, navigate to the
  // new collection's detail page using the slug returned by the server.
  const cloneMutation = useCloneCollection()
  // PSY-352: like/unlike toggle.
  const likeMutation = useLikeCollection()
  const unlikeMutation = useUnlikeCollection()

  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [showCopied, setShowCopied] = useState(false)
  // null = not interacted; URL hash drives the default. User toggle sticks once set.
  const [showGraphOverride, setShowGraphOverride] = useState<boolean | null>(null)
  const hash = useUrlHash()

  const handleShare = useCallback(() => {
    navigator.clipboard.writeText(window.location.href).then(() => {
      setShowCopied(true)
      setTimeout(() => setShowCopied(false), 2000)
    })
  }, [])

  const items = collection?.items ?? []
  // PSY-555 (was PSY-366): surface the graph toggle whenever the collection
  // has any items. Pre-PSY-555 the toggle was gated on artist items only
  // because non-artist items couldn't be rendered; with the multi-type
  // graph every entity type becomes a node, so the gate moves to "is the
  // collection non-empty".
  const hasItems = items.length > 0

  // Gate auto-open on artist items so `#graph` on a non-artist collection no-ops.
  const autoOpenFromHash = hash === GRAPH_HASH && artistItemCount > 0
  const showGraph = showGraphOverride ?? autoOpenFromHash

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load collection'
    const is404 = (error as ApiError).status === 404

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Collection Not Found' : 'Error Loading Collection'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The collection you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/collections">Back to Collections</Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!collection) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Collection Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The collection you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/collections">Back to Collections</Link>
          </Button>
        </div>
      </div>
    )
  }

  const currentUserId = user?.id ? Number(user.id) : undefined
  const isCreator = currentUserId === collection.creator_id
  const canSubscribe = isAuthenticated && !isCreator
  // PSY-351: per ticket, the clone button is hidden on the user's own
  // collections (you wouldn't fork yourself). Anyone else who is
  // authenticated may clone any public collection.
  const canClone = isAuthenticated && !isCreator && collection.is_public

  // PSY-351 attribution state.
  // - forkedFromInfo set + collection.forked_from_collection_id set →
  //   render "Forked from <link> by <curator>".
  // - collection.forked_from_collection_id falsy → not a fork, render nothing.
  // - collection.forked_from_collection_id set, info absent → source was
  //   deleted; render fallback copy.
  const isFork = Boolean(collection.forked_from_collection_id)
  const forkedFromInfo = collection.forked_from ?? null

  const handleSubscribe = () => {
    if (collection.is_subscribed) {
      unsubscribeMutation.mutate({ slug })
    } else {
      subscribeMutation.mutate({ slug })
    }
  }

  const handleDelete = () => {
    deleteMutation.mutate(
      { slug },
      {
        onSuccess: () => {
          setIsDeleteDialogOpen(false)
          router.push('/collections')
        },
      }
    )
  }

  const handleClone = () => {
    cloneMutation.mutate(
      { slug },
      {
        onSuccess: (newCollection) => {
          router.push(`/collections/${newCollection.slug}`)
        },
      }
    )
  }

  const handleToggleLike = () => {
    if (collection.user_likes_this) {
      unlikeMutation.mutate({ slug })
    } else {
      likeMutation.mutate({ slug })
    }
  }
  const isLikePending = likeMutation.isPending || unlikeMutation.isPending

  return (
    <div className="container max-w-6xl mx-auto px-4 py-6">
      {/* Breadcrumb Navigation */}
      <Breadcrumb
        fallback={{ href: '/collections', label: 'Collections' }}
        currentPage={collection.title}
      />

      {/* Header */}
      <header className="mb-8">
        {isEditing ? (
          <InlineEditForm
            slug={slug}
            title={collection.title}
            description={collection.description}
            isPublic={collection.is_public}
            collaborative={collection.collaborative}
            displayMode={collection.display_mode}
            coverImageUrl={collection.cover_image_url ?? ''}
            onDone={() => setIsEditing(false)}
          />
        ) : (
          <div>
            <div className="flex items-start justify-between gap-4">
              <div className="flex items-start gap-4 min-w-0">
                {/* PSY-554: cover always renders (typed Library icon when
                    URL is null/empty or `<img>` 404s). Same h-24 footprint
                    in either state so the header layout doesn't shift. */}
                <CollectionCoverImage
                  url={collection.cover_image_url}
                  alt={`${collection.title} cover`}
                  className="h-24 w-24 shrink-0 rounded-lg border border-border/50 bg-muted/50"
                  fallback={
                    <Library
                      className="h-10 w-10 text-muted-foreground/50"
                      aria-hidden="true"
                    />
                  }
                />
                <div className="min-w-0">
                <div className="flex items-center gap-3 mb-1 flex-wrap">
                  <h1 className="text-3xl font-bold tracking-tight">
                    {collection.title}
                  </h1>
                  {collection.is_featured && (
                    <Badge variant="default" className="text-xs">
                      <Star className="h-3 w-3 mr-0.5" />
                      Featured
                    </Badge>
                  )}
                  {collection.collaborative && (
                    <Badge variant="secondary" className="text-xs">
                      Collaborative
                    </Badge>
                  )}
                </div>

                <p className="text-sm text-muted-foreground">
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
                </p>

                {/* PSY-353: contributor badge surfaces community curation
                    when at least 3 distinct users have added items.
                    Threshold matches What.cd's min-3-items spirit; below
                    it, the creator-only line above is sufficient. */}
                {collection.contributor_count >= 3 && (
                  <p
                    className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground"
                    data-testid="contributor-badge"
                  >
                    <Users className="h-3 w-3" aria-hidden="true" />
                    Built by {collection.contributor_count} contributors
                  </p>
                )}

                {/* PSY-351: inline fork attribution. Renders below the
                    creator line when this collection was cloned. Falls back
                    to literal copy when the source was deleted (FK is set
                    but the snapshot is null). */}
                {isFork && (
                  <p
                    className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground"
                    data-testid="forked-from-attribution"
                  >
                    <GitFork className="h-3 w-3" aria-hidden="true" />
                    {forkedFromInfo ? (
                      <span>
                        Forked from{' '}
                        <Link
                          href={`/collections/${forkedFromInfo.slug}`}
                          className="underline-offset-2 hover:underline text-foreground/80"
                        >
                          {forkedFromInfo.title}
                        </Link>{' '}
                        by {forkedFromInfo.creator_name}
                      </span>
                    ) : (
                      <span>Forked from a deleted collection</span>
                    )}
                  </p>
                )}

                {/* PSY-349: description rendered as markdown by the backend
                    (goldmark + bluemonday). Always trust description_html when
                    present; fall back to nothing rather than rendering raw
                    description (which is markdown source, not HTML). */}
                {collection.description_html && (
                  <MarkdownContent
                    html={collection.description_html}
                    className="mt-3 text-muted-foreground"
                    testId="collection-description"
                  />
                )}

                {/* Stats */}
                <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-muted-foreground">
                  <span className="flex items-center gap-1">
                    <Library className="h-4 w-4" />
                    {collection.item_count === 1
                      ? '1 item'
                      : `${collection.item_count} items`}
                  </span>
                  <span className="flex items-center gap-1">
                    <Users className="h-4 w-4" />
                    {collection.subscriber_count === 1
                      ? '1 subscriber'
                      : `${collection.subscriber_count} subscribers`}
                  </span>
                  {/* PSY-351: public fork count — visible to all viewers
                      so original collections advertise their pull. Only
                      rendered when forks exist to avoid noise on every
                      collection page. */}
                  {collection.forks_count > 0 && (
                    <span
                      className="flex items-center gap-1"
                      data-testid="forks-count"
                    >
                      <GitFork className="h-4 w-4" />
                      {collection.forks_count === 1
                        ? '1 fork'
                        : `${collection.forks_count} forks`}
                    </span>
                  )}
                  <span title={new Date(collection.created_at).toLocaleString()}>
                    Created {formatRelativeTime(collection.created_at)}
                  </span>
                  {collection.updated_at !== collection.created_at && (
                    <span title={new Date(collection.updated_at).toLocaleString()}>
                      Updated {formatRelativeTime(collection.updated_at)}
                    </span>
                  )}
                </div>

                {/* Entity type breakdown */}
                {items.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {Object.entries(
                      items.reduce<Record<string, number>>((acc, item) => {
                        acc[item.entity_type] = (acc[item.entity_type] || 0) + 1
                        return acc
                      }, {})
                    ).map(([type, count]) => (
                      <Badge key={type} variant="secondary" className="text-xs font-normal">
                        {count} {count === 1 ? getEntityTypeLabel(type).toLowerCase() : `${getEntityTypeLabel(type).toLowerCase()}s`}
                      </Badge>
                    ))}
                  </div>
                )}
                </div>
              </div>

              {/* Action buttons */}
              <div className="flex items-center gap-2 shrink-0">
                {/* PSY-352: Like toggle. Authenticated viewers can click;
                    anonymous viewers see a read-only heart + count so they
                    know the signal exists. Aggregate count only — privacy
                    decision: no list of likers exposed. */}
                {isAuthenticated ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleToggleLike}
                    disabled={isLikePending}
                    aria-pressed={collection.user_likes_this ?? false}
                    aria-label={
                      collection.user_likes_this
                        ? 'Unlike collection'
                        : 'Like collection'
                    }
                    className={cn(
                      collection.user_likes_this && 'text-primary'
                    )}
                    data-testid="collection-like-button"
                  >
                    <Heart
                      className={cn(
                        'h-4 w-4 mr-1.5',
                        collection.user_likes_this && 'fill-current'
                      )}
                    />
                    {collection.like_count}
                  </Button>
                ) : (
                  <span
                    className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border/60 px-3 text-sm text-muted-foreground"
                    data-testid="collection-like-count"
                  >
                    <Heart className="h-4 w-4" />
                    {collection.like_count}
                  </span>
                )}

                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleShare}
                >
                  <Share2 className="h-4 w-4 mr-1.5" />
                  {showCopied ? 'Copied!' : 'Share'}
                </Button>

                {/* PSY-555 (was PSY-366): Explore graph toggle. Visible
                    whenever the collection has at least one item — every
                    entity type renders as a node in the multi-type graph. */}
                {hasItems && (
                  <Button
                    variant={showGraph ? 'default' : 'outline'}
                    size="sm"
                    onClick={() => setShowGraphOverride(!showGraph)}
                    aria-pressed={showGraph}
                    aria-label={showGraph ? 'Hide collection graph' : 'Explore collection graph'}
                  >
                    <Network className="h-4 w-4 mr-1.5" />
                    {showGraph ? 'Hide graph' : 'Explore graph'}
                  </Button>
                )}

                {canSubscribe && (
                  <Button
                    variant={collection.is_subscribed ? 'secondary' : 'default'}
                    size="sm"
                    onClick={handleSubscribe}
                    disabled={
                      subscribeMutation.isPending ||
                      unsubscribeMutation.isPending
                    }
                  >
                    {collection.is_subscribed ? (
                      <>
                        <BellOff className="h-4 w-4 mr-1.5" />
                        Unsubscribe
                      </>
                    ) : (
                      <>
                        <Bell className="h-4 w-4 mr-1.5" />
                        Subscribe
                      </>
                    )}
                  </Button>
                )}

                {/* PSY-351: Clone/fork. Visible only when caller is
                    authenticated AND not the owner AND the source is
                    public. */}
                {canClone && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleClone}
                    disabled={cloneMutation.isPending}
                    aria-label="Fork collection"
                  >
                    {cloneMutation.isPending ? (
                      <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
                    ) : (
                      <GitFork className="h-4 w-4 mr-1.5" />
                    )}
                    {cloneMutation.isPending ? 'Forking...' : 'Fork'}
                  </Button>
                )}

                {isCreator && (
                  <>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setIsEditing(true)}
                    >
                      <Pencil className="h-4 w-4 mr-1.5" />
                      Edit
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setIsDeleteDialogOpen(true)}
                      disabled={deleteMutation.isPending}
                      className="text-destructive hover:text-destructive"
                      aria-label="Delete collection"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </>
                )}
              </div>
            </div>
          </div>
        )}
      </header>

      {/* PSY-354: tag chips + picker. Reuses the same EntityTagList that
          renders on artist/release/etc detail pages — chips link to
          /tags/{slug} for the deep-dive (the collection-card override
          links to /collections?tag=<slug> instead because cards prefer
          the lateral "show me other collections like this" path). The
          per-collection 10-tag cap is enforced server-side in
          catalog.TagService.AddTagToEntity, so this picker honors the
          limit regardless of the picker's UI cap awareness. */}
      <div className="mb-4">
        <EntityTagList
          entityType="collection"
          entityId={collection.id}
          isAuthenticated={isAuthenticated}
        />
      </div>

      {/* PSY-356: publish-gate banner (creator-only) */}
      {isCreator && <PublishGateBanner collection={collection} />}

      {/* PSY-555 (was PSY-366): collection graph (toggleable). Renders
          only when the user clicks "Explore graph" in the actions row.
          The wrapper has `id="graph"` so Cmd+K deep-links resolve. */}
      {showGraph && hasItems && (
        <CollectionGraph slug={slug} collectionTitle={collection.title} />
      )}

      {/* Add Items (creator only) */}
      {isCreator && (
        <AddItemsSection slug={slug} existingItems={items} />
      )}

      {/* Items list */}
      <CollectionItemsList
        items={items}
        slug={slug}
        isCreator={isCreator}
        displayMode={collection.display_mode}
      />

      {/* Discussion */}
      <CommentThread entityType="collection" entityId={collection.id} />

      {/* Delete Confirmation Dialog */}
      <Dialog open={isDeleteDialogOpen} onOpenChange={setIsDeleteDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Trash2 className="h-5 w-5 text-destructive" />
              Delete Collection
            </DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{collection.title}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>

          {deleteMutation.isError && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {deleteMutation.error?.message ||
                'Failed to delete collection. Please try again.'}
            </div>
          )}

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setIsDeleteDialogOpen(false)}
              disabled={deleteMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDelete}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Deleting...
                </>
              ) : (
                <>
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete Collection
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

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
type CollectionItemsViewMode = 'grid' | 'list'

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

function readStoredViewMode(): CollectionItemsViewMode {
  if (typeof window === 'undefined') return 'grid'
  try {
    const stored = window.localStorage.getItem(VIEW_MODE_STORAGE_KEY)
    if (stored === 'grid' || stored === 'list') return stored
  } catch {
    // localStorage unavailable (private mode, etc.) — fall through.
  }
  return 'grid'
}

function CollectionItemsList({
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

  // PSY-360: density preference for the grid view. List view ignores
  // density (its layout is intentionally fixed). Storage key matches the
  // hook's prefix convention (ph-density-collections).
  const { density, setDensity } = useDensity('collections')

  // PSY-360: view-mode preference (grid vs list). Default `grid` so
  // first-time viewers see the visual layout. Persists per-browser.
  // Read once on mount via lazy initializer; `useEffect` could fight
  // with SSR hydration so we delay the read by mirroring the
  // FavoriteVenuesTab pattern that already ships.
  const [viewMode, setViewModeState] = useState<CollectionItemsViewMode>(() =>
    readStoredViewMode()
  )

  const setViewMode = useCallback((mode: CollectionItemsViewMode) => {
    setViewModeState(mode)
    try {
      window.localStorage.setItem(VIEW_MODE_STORAGE_KEY, mode)
    } catch {
      // localStorage unavailable — preference falls back to default
      // next mount. Acceptable.
    }
  }, [])

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
              title="Drag to reorder"
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
                title="Move up"
                aria-label="Move up"
              >
                <ChevronUp className="h-3.5 w-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-5 w-5 p-0 text-muted-foreground hover:text-foreground"
                onClick={() => onMoveDown(index)}
                disabled={index === totalItems - 1 || isReordering}
                title="Move down"
                aria-label="Move down"
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
                title="Edit notes"
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
                title="Remove from collection"
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

// ──────────────────────────────────────────────
// Add Items Search Panel
// ──────────────────────────────────────────────

function AddItemsSection({
  slug,
  existingItems,
}: {
  slug: string
  existingItems: CollectionItem[]
}) {
  const [isOpen, setIsOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [addedMessage, setAddedMessage] = useState<string | null>(null)
  const addMutation = useAddCollectionItem()

  const { data: searchResults, isSearching } = useEntitySearch({
    query: searchQuery,
    enabled: isOpen,
  })

  // Flatten results into a single list for display.
  // PSY-372: shows surface alongside the other entity types now that the
  // /shows/search endpoint exists (PSY-520).
  const allResults: EntitySearchResult[] = searchResults
    ? [
        ...searchResults.artists,
        ...searchResults.shows,
        ...searchResults.venues,
        ...searchResults.releases,
        ...searchResults.labels,
        ...searchResults.festivals,
      ]
    : []

  // Check if an entity is already in the collection
  const isAlreadyAdded = (entityType: string, entityId: number): boolean => {
    return existingItems.some(
      item => item.entity_type === entityType && item.entity_id === entityId
    )
  }

  const handleAdd = (result: EntitySearchResult) => {
    addMutation.mutate(
      {
        slug,
        entityType: result.entityType,
        entityId: result.id,
      },
      {
        onSuccess: () => {
          setAddedMessage(`Added "${result.name}" to collection`)
          setTimeout(() => setAddedMessage(null), 3000)
        },
      }
    )
  }

  const SearchIcon = ENTITY_ICONS

  return (
    <div className="mb-6">
      {!isOpen ? (
        <Button
          variant="outline"
          size="sm"
          onClick={() => setIsOpen(true)}
        >
          <Plus className="h-4 w-4 mr-1.5" />
          Add Items
        </Button>
      ) : (
        <div className="rounded-lg border border-border/50 bg-card p-4">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-sm font-semibold">Add Items</h3>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 w-7 p-0"
              onClick={() => {
                setIsOpen(false)
                setSearchQuery('')
              }}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>

          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search artists, shows, venues, releases, labels, festivals..."
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              className="pl-9"
              autoFocus
            />
          </div>

          {/* Success feedback */}
          {addedMessage && (
            <div className="mt-2 text-sm text-green-600 dark:text-green-400 flex items-center gap-1.5">
              <Check className="h-3.5 w-3.5" />
              {addedMessage}
            </div>
          )}

          {/* Search results */}
          {searchQuery.trim().length >= 2 && (
            <div className="mt-3 max-h-64 overflow-y-auto">
              {isSearching ? (
                <div className="flex items-center justify-center py-4">
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                </div>
              ) : allResults.length === 0 ? (
                <p className="text-sm text-muted-foreground py-3 text-center">
                  No results found for &quot;{searchQuery}&quot;
                </p>
              ) : (
                <div className="space-y-1">
                  {allResults.map(result => {
                    const alreadyAdded = isAlreadyAdded(result.entityType, result.id)
                    const Icon = SearchIcon[result.entityType] ?? Library

                    return (
                      <div
                        key={`${result.entityType}-${result.id}`}
                        className="flex items-center gap-3 rounded-md p-2 hover:bg-muted/50"
                      >
                        <div className="h-7 w-7 shrink-0 rounded bg-muted/50 flex items-center justify-center">
                          <Icon className="h-3.5 w-3.5 text-muted-foreground/60" />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium truncate">
                              {result.name}
                            </span>
                            <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
                              {getEntityTypeLabel(result.entityType)}
                            </Badge>
                          </div>
                          {result.subtitle && (
                            <p className="text-xs text-muted-foreground truncate">
                              {result.subtitle}
                            </p>
                          )}
                        </div>
                        {alreadyAdded ? (
                          <Badge variant="secondary" className="text-xs shrink-0">
                            <Check className="h-3 w-3 mr-1" />
                            Added
                          </Badge>
                        ) : (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 px-2 shrink-0"
                            onClick={() => handleAdd(result)}
                            disabled={addMutation.isPending}
                          >
                            <Plus className="h-3.5 w-3.5 mr-1" />
                            Add
                          </Button>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )}

          {/* Error feedback */}
          {addMutation.isError && (
            <p className="mt-2 text-sm text-destructive">
              {addMutation.error instanceof Error
                ? addMutation.error.message
                : 'Failed to add item'}
            </p>
          )}
        </div>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Inline Edit Form
// ──────────────────────────────────────────────

function InlineEditForm({
  slug,
  title: initialTitle,
  description: initialDescription,
  isPublic: initialPublic,
  collaborative: initialCollaborative,
  displayMode: initialDisplayMode,
  coverImageUrl: initialCoverImageUrl,
  onDone,
}: {
  slug: string
  title: string
  description: string
  isPublic: boolean
  collaborative: boolean
  displayMode: CollectionDisplayMode
  coverImageUrl: string
  onDone: () => void
}) {
  const updateMutation = useUpdateCollection()
  const [title, setTitle] = useState(initialTitle)
  const [description, setDescription] = useState(initialDescription)
  const [isPublic, setIsPublic] = useState(initialPublic)
  const [collaborative, setCollaborative] = useState(initialCollaborative)
  const [displayMode, setDisplayMode] =
    useState<CollectionDisplayMode>(initialDisplayMode)
  // PSY-371: cover image URL input. Empty string means "no cover" (also the
  // affordance for clearing a previously-set one — the Save payload sends
  // `null` so the backend nulls the column).
  const [coverImageUrl, setCoverImageUrl] = useState(initialCoverImageUrl)
  // Track whether the user has interacted with the field so we can defer
  // the inline error until they've had a chance to finish typing.
  const [coverImageUrlTouched, setCoverImageUrlTouched] = useState(false)

  const trimmedCoverUrl = coverImageUrl.trim()
  const coverImageUrlError = validateCoverImageUrl(coverImageUrl)
  const showCoverImageUrlError =
    coverImageUrlTouched && coverImageUrlError !== null
  // Only render the preview for valid, non-empty URLs.
  const showCoverImagePreview =
    trimmedCoverUrl.length > 0 && coverImageUrlError === null

  const handleSave = () => {
    if (coverImageUrlError) return
    updateMutation.mutate(
      {
        slug,
        title: title.trim(),
        description: description.trim(),
        is_public: isPublic,
        collaborative,
        display_mode: displayMode,
        // Empty string clears the cover — send explicit null so the backend
        // distinguishes "set to null" from "no change".
        cover_image_url: trimmedCoverUrl.length === 0 ? null : trimmedCoverUrl,
      },
      { onSuccess: () => onDone() }
    )
  }

  return (
    <div className="space-y-4 rounded-lg border border-border/50 bg-card p-4">
      <div>
        <label htmlFor="edit-title" className="text-sm font-medium mb-1.5 block">
          Title
        </label>
        <Input
          id="edit-title"
          value={title}
          onChange={e => setTitle(e.target.value)}
          autoFocus
        />
      </div>

      <div>
        <label
          htmlFor="edit-description"
          className="text-sm font-medium mb-1.5 block"
        >
          Description
        </label>
        <MarkdownEditor
          id="edit-description"
          value={description}
          onChange={setDescription}
          rows={4}
          maxLength={MAX_COLLECTION_MARKDOWN_LENGTH}
          placeholder="Markdown supported: **bold**, *italic*, [link](url), > quote, - list"
          testId="collection-description-editor"
        />
      </div>

      {/* Cover image URL (PSY-371). Optional; clearing the field nulls the
          cover via an explicit `null` payload at save time. */}
      <div>
        <label
          htmlFor="edit-cover-image-url"
          className="text-sm font-medium mb-1.5 block"
        >
          Cover image URL{' '}
          <span className="text-xs font-normal text-muted-foreground">
            (optional)
          </span>
        </label>
        <div className="flex gap-2">
          <Input
            id="edit-cover-image-url"
            type="url"
            inputMode="url"
            value={coverImageUrl}
            onChange={e => {
              setCoverImageUrl(e.target.value)
              setCoverImageUrlTouched(true)
            }}
            onBlur={() => setCoverImageUrlTouched(true)}
            placeholder="https://example.com/cover.jpg"
            maxLength={MAX_COVER_IMAGE_URL_LENGTH}
            aria-invalid={showCoverImageUrlError ? true : undefined}
            aria-describedby={
              showCoverImageUrlError
                ? 'edit-cover-image-url-error'
                : 'edit-cover-image-url-help'
            }
            data-testid="edit-cover-image-url-input"
          />
          {trimmedCoverUrl.length > 0 && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setCoverImageUrl('')
                setCoverImageUrlTouched(true)
              }}
              data-testid="edit-cover-image-url-clear"
            >
              <X className="h-4 w-4 mr-1" />
              Clear
            </Button>
          )}
        </div>
        {showCoverImageUrlError ? (
          <p
            id="edit-cover-image-url-error"
            className="text-xs text-destructive mt-1.5"
            role="alert"
          >
            {coverImageUrlError}
          </p>
        ) : (
          <p
            id="edit-cover-image-url-help"
            className="text-xs text-muted-foreground mt-1.5"
          >
            Paste a direct image URL (e.g. Bandcamp art). Leave empty to
            remove the current cover.
          </p>
        )}
        {showCoverImagePreview && (
          <div className="mt-2 h-24 w-24 rounded-lg overflow-hidden border border-border/50 bg-muted/50">
            <img
              src={trimmedCoverUrl}
              alt="Cover image preview"
              className="h-full w-full object-cover"
              data-testid="edit-cover-image-url-preview"
            />
          </div>
        )}
      </div>

      {/* Display mode — radio group so the choice and its consequence are
          always visible together. Numbered/ranked is opt-in. */}
      <fieldset>
        <legend className="text-sm font-medium mb-1.5">Display mode</legend>
        <div className="flex flex-wrap items-stretch gap-2">
          <label
            className={`flex flex-1 min-w-[10rem] items-start gap-2 rounded-md border p-2.5 cursor-pointer text-sm ${
              displayMode === 'unranked'
                ? 'border-primary bg-primary/5'
                : 'border-border hover:border-border/80'
            }`}
          >
            <input
              type="radio"
              name="display-mode"
              value="unranked"
              checked={displayMode === 'unranked'}
              onChange={() => setDisplayMode('unranked')}
              className="mt-0.5"
            />
            <span className="flex-1">
              <span className="flex items-center gap-1.5 font-medium">
                <LayoutGrid className="h-3.5 w-3.5" />
                Unranked
              </span>
              <span className="block text-xs text-muted-foreground mt-0.5">
                Flat list. No numbering.
              </span>
            </span>
          </label>

          <label
            className={`flex flex-1 min-w-[10rem] items-start gap-2 rounded-md border p-2.5 cursor-pointer text-sm ${
              displayMode === 'ranked'
                ? 'border-primary bg-primary/5'
                : 'border-border hover:border-border/80'
            }`}
          >
            <input
              type="radio"
              name="display-mode"
              value="ranked"
              checked={displayMode === 'ranked'}
              onChange={() => setDisplayMode('ranked')}
              className="mt-0.5"
            />
            <span className="flex-1">
              <span className="flex items-center gap-1.5 font-medium">
                <ListOrdered className="h-3.5 w-3.5" />
                Ranked
              </span>
              <span className="block text-xs text-muted-foreground mt-0.5">
                Numbered, drag-to-reorder.
              </span>
            </span>
          </label>
        </div>
      </fieldset>

      <div className="flex items-center gap-6">
        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={isPublic}
            onChange={e => setIsPublic(e.target.checked)}
            className="rounded border-border"
          />
          Public
        </label>

        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={collaborative}
            onChange={e => setCollaborative(e.target.checked)}
            className="rounded border-border"
          />
          Collaborative
        </label>
      </div>

      {updateMutation.error && (
        <p className="text-sm text-destructive">
          {updateMutation.error instanceof Error
            ? updateMutation.error.message
            : 'Failed to update collection'}
        </p>
      )}

      <div className="flex gap-2">
        <Button
          size="sm"
          onClick={handleSave}
          disabled={
            !title.trim() ||
            coverImageUrlError !== null ||
            updateMutation.isPending
          }
        >
          <Check className="h-4 w-4 mr-1" />
          {updateMutation.isPending ? 'Saving...' : 'Save'}
        </Button>
        <Button size="sm" variant="outline" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </div>
  )
}
