'use client'

import { useState, useCallback } from 'react'
import dynamic from 'next/dynamic'
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
  Plus,
  Share2,
  GitFork,
  Heart,
  ListOrdered,
  LayoutGrid,
  Network,
  Flag,
} from 'lucide-react'
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
  useBulkAddCollectionItems,
  useSubscribeCollection,
  useUnsubscribeCollection,
  useDeleteCollection,
  useCloneCollection,
  useLikeCollection,
  useUnlikeCollection,
} from '../hooks'
import {
  AddItemsPicker,
  type StagedCollectionItem,
} from './AddItemsPicker'
import { cn } from '@/lib/utils'
import {
  getEntityTypeLabel,
  MAX_COLLECTION_MARKDOWN_LENGTH,
  MAX_COVER_IMAGE_URL_LENGTH,
  validateCoverImageUrl,
} from '../types'
import type { CollectionDisplayMode, CollectionItem } from '../types'
import { MarkdownContent } from './MarkdownContent'
// Lazily-loaded write-mode editor (keeps marked/dompurify out of the shared
// chunk). See MarkdownEditorLazy / PSY-951.
import { MarkdownEditor } from './MarkdownEditorLazy'
// The items list carries the `@dnd-kit/*` drag-reorder machinery (~87 KB raw).
// It is `dynamic()`-imported (ssr:true) so that lib lands in a per-route async
// chunk instead of Turbopack's global shared client chunk (loaded on /explore,
// which has no drag-reorder). ssr:true keeps the items — the page's primary
// content — server-rendered for SEO/LCP. See PSY-951 / the PSY-944 spike.
const CollectionItemsList = dynamic(
  () =>
    import('./CollectionItemsList').then(m => ({
      default: m.CollectionItemsList,
    })),
  { ssr: true }
)
import {
  describeCollectionMutationError,
  MutationFeedback,
  useAutoDismissError,
} from './collectionDetailShared'
import { CollectionGraph } from './CollectionGraph'
import { CollectionCoverImage } from './CollectionCoverImage'
import { GRAPH_HASH, useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { Breadcrumb, UserAttribution } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useRouter } from 'next/navigation'
import type { ApiError } from '@/lib/api'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { CommentThread } from '@/features/comments'
import { EntityTagList } from '@/features/tags'
import { ReportEntityDialog } from '@/features/contributions'

interface CollectionDetailProps {
  slug: string
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
  // PSY-357: report dialog open state. The trigger is only rendered for
  // authenticated, non-creator viewers (private collections aren't visible
  // to non-creators, so no extra public-state gate is needed).
  const [isReportOpen, setIsReportOpen] = useState(false)
  // null = not interacted; URL hash drives the default. User toggle sticks once set.
  const [showGraphOverride, setShowGraphOverride] = useState<boolean | null>(null)
  const hash = useUrlHash()

  // PSY-609: like/unlike use optimistic-rollback — when the server rejects
  // the action, the optimistic state snaps back but until now the user got
  // no explanation. Auto-dismiss the banner after ~3s so it doesn't
  // accumulate after the user moves on. The 403 case (private target)
  // gets dedicated copy via describeCollectionMutationError.
  const formatLikeError = useCallback(
    (err: unknown) =>
      describeCollectionMutationError(err, 'Failed to like collection.'),
    []
  )
  const formatUnlikeError = useCallback(
    (err: unknown) =>
      describeCollectionMutationError(err, 'Failed to unlike collection.', {
        unlikePrivate: true,
      }),
    []
  )
  const likeError = useAutoDismissError(
    likeMutation.error,
    likeMutation.isError,
    formatLikeError
  )
  const unlikeError = useAutoDismissError(
    unlikeMutation.error,
    unlikeMutation.isError,
    formatUnlikeError
  )

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

  // Gate auto-open so `#graph` on an empty collection no-ops; multi-type graph (PSY-555) can render any item.
  const autoOpenFromHash = hash === GRAPH_HASH && hasItems
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
  const isAdmin = user?.is_admin === true
  const canSubscribe = isAuthenticated && !isCreator
  // PSY-351: per ticket, the clone button is hidden on the user's own
  // collections (you wouldn't fork yourself). Anyone else who is
  // authenticated may clone any public collection.
  const canClone = isAuthenticated && !isCreator && collection.is_public
  // PSY-578: admins moderate via the queue, so they don't need (or get)
  // the Report trigger here. Creators are excluded for the obvious reason.
  const canReport = isAuthenticated && !isCreator && !isAdmin

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
                  <UserAttribution
                    name={collection.creator_name}
                    username={collection.creator_username}
                    className="text-foreground hover:underline"
                  />
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

                {/* PSY-578: report a collection. Mirrors the Report
                    button on artist/venue/festival/show detail pages. */}
                {canReport && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setIsReportOpen(true)}
                    aria-label="Report collection"
                    data-testid="collection-report-button"
                  >
                    <Flag className="h-4 w-4 mr-1.5" />
                    Report
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

            {/*
              PSY-609: surface failures from the header-row action buttons
              so the user isn't left guessing why nothing happened.
              - Subscribe / unsubscribe: sticky inline banner on 4xx.
              - Clone (Fork): sticky inline banner on 4xx (no navigation
                happened, so the user needs to know).
              - Like / unlike (PSY-352): optimistic-rollback hooks; the
                snap-back of the heart is the visual signal, the banner
                just explains the *why* and auto-dismisses after ~3s so
                it doesn't accrue after the user moves on. 403 (private
                target) renders dedicated copy via describeCollectionMutationError.
            */}
            {subscribeMutation.isError && (
              <MutationFeedback
                variant="error"
                testId="subscribe-error"
                message={describeCollectionMutationError(
                  subscribeMutation.error,
                  'Failed to subscribe to this collection.'
                )}
              />
            )}
            {unsubscribeMutation.isError && (
              <MutationFeedback
                variant="error"
                testId="unsubscribe-error"
                message={describeCollectionMutationError(
                  unsubscribeMutation.error,
                  'Failed to unsubscribe from this collection.'
                )}
              />
            )}
            {cloneMutation.isError && (
              <MutationFeedback
                variant="error"
                testId="clone-error"
                message={describeCollectionMutationError(
                  cloneMutation.error,
                  'Failed to fork this collection.'
                )}
              />
            )}
            {likeError && (
              <MutationFeedback
                variant="error"
                testId="like-error"
                message={likeError}
              />
            )}
            {unlikeError && (
              <MutationFeedback
                variant="error"
                testId="unlike-error"
                message={unlikeError}
              />
            )}
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

      {/* PSY-555 (was PSY-366): collection graph (toggleable). Renders
          only when the user clicks "Explore graph" in the actions row.
          The wrapper has `id="graph"` so Cmd+K deep-links resolve. */}
      {showGraph && hasItems && (
        <CollectionGraph slug={slug} collectionTitle={collection.title} />
      )}

      {/* Add Items (creator only). PSY-581: empty collections render the
          search open so the "Add your first item using the search above"
          copy is honest. */}
      {isCreator && (
        <AddItemsSection
          slug={slug}
          existingItems={items}
          defaultOpen={items.length === 0}
        />
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

      {/* Mounted only when `canReport` so we don't ship the dialog tree
          to viewers who can't open it. `entityTypeLabel="collection"`
          makes the modal copy explicit ("Report Issue with collection
          'X'") rather than the bare entity-name fallback. */}
      {canReport && (
        <ReportEntityDialog
          open={isReportOpen}
          onOpenChange={setIsReportOpen}
          entityType="collection"
          entityId={collection.id}
          entityName={collection.title}
          entityTypeLabel="collection"
        />
      )}

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
// Add Items Search Panel
// ──────────────────────────────────────────────

function AddItemsSection({
  slug,
  existingItems,
  defaultOpen = false,
}: {
  slug: string
  existingItems: CollectionItem[]
  /**
   * PSY-581: when true, the picker renders open on first paint. Parent
   * passes `items.length === 0` so the empty-state copy stays honest.
   * Sets only the initial state — the X toggle still collapses/reopens.
   */
  defaultOpen?: boolean
}) {
  const [isOpen, setIsOpen] = useState(defaultOpen)
  // PSY-823: items staged in the picker, submitted in one bulk-add request.
  const [stagedItems, setStagedItems] = useState<StagedCollectionItem[]>([])
  const [feedback, setFeedback] = useState<
    | { variant: 'success'; message: string }
    | { variant: 'error'; message: string }
    | null
  >(null)
  const bulkAddMutation = useBulkAddCollectionItems()

  const handleSubmit = async () => {
    if (stagedItems.length === 0) return
    try {
      const resp = await bulkAddMutation.mutateAsync({
        slug,
        items: stagedItems.map((s) => ({
          entity_type: s.entityType,
          entity_id: s.entityId,
        })),
      })
      const addedCount = resp.added.length
      const rejectedCount = resp.errors.length
      if (rejectedCount === 0) {
        setFeedback({
          variant: 'success',
          message: `Added ${addedCount} ${addedCount === 1 ? 'item' : 'items'} to collection`,
        })
      } else if (addedCount === 0) {
        setFeedback({
          variant: 'error',
          message: `Couldn't add any items (${rejectedCount} ${rejectedCount === 1 ? 'error' : 'errors'}). Adjust the picker and try again.`,
        })
      } else {
        setFeedback({
          variant: 'success',
          message: `Added ${addedCount} ${addedCount === 1 ? 'item' : 'items'}; ${rejectedCount} couldn't be added.`,
        })
      }
      // Clear staged list only if at least one row committed. When EVERY
      // row failed, leave the picker as-is so the user can edit/retry
      // without re-staging from scratch.
      if (addedCount > 0) {
        setStagedItems([])
      }
      setTimeout(() => setFeedback(null), 4000)
    } catch (err) {
      setFeedback({
        variant: 'error',
        message: describeCollectionMutationError(err, 'Failed to add items.'),
      })
    }
  }

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
            <span className="text-sm font-semibold sr-only">Add items</span>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 w-7 p-0 ml-auto"
              onClick={() => {
                setIsOpen(false)
                setStagedItems([])
                setFeedback(null)
              }}
              aria-label="Close add-items picker"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>

          <AddItemsPicker
            existingItems={existingItems.map((i) => ({
              entity_type: i.entity_type,
              entity_id: i.entity_id,
            }))}
            stagedItems={stagedItems}
            onStagedItemsChange={setStagedItems}
          />

          {feedback && (
            <MutationFeedback
              variant={feedback.variant}
              message={feedback.message}
              testId={feedback.variant === 'success' ? 'add-item-success' : 'add-item-error'}
            />
          )}

          <div className="flex justify-end mt-4">
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={stagedItems.length === 0 || bulkAddMutation.isPending}
              data-testid="add-items-picker-submit"
            >
              {bulkAddMutation.isPending
                ? 'Adding...'
                : `Add ${stagedItems.length || ''} item${stagedItems.length === 1 ? '' : 's'}`.trim()}
            </Button>
          </div>
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
