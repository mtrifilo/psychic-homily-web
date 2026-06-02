'use client'

import { useState, useCallback, useEffect } from 'react'
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
  Share2,
  GitFork,
  Heart,
  ListOrdered,
  LayoutGrid,
  MoreHorizontal,
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  useCollection,
  useUpdateCollection,
  useSubscribeCollection,
  useUnsubscribeCollection,
  useDeleteCollection,
  useCloneCollection,
  useLikeCollection,
  useUnlikeCollection,
} from '../hooks'
import { cn } from '@/lib/utils'
import {
  getEntityTypeLabel,
  MAX_COLLECTION_MARKDOWN_LENGTH,
  MAX_COVER_IMAGE_URL_LENGTH,
  validateCoverImageUrl,
} from '../types'
import type { CollectionDisplayMode } from '../types'
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
import {
  CollectionAnchorNav,
  ANCHOR_SECTION_SCROLL_MT,
  type AnchorSection,
} from './CollectionAnchorNav'
import { GRAPH_HASH, useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { Breadcrumb, UserAttribution } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useRouter, usePathname } from 'next/navigation'
import type { ApiError } from '@/lib/api'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import { CommentThread } from '@/features/comments'
import { EntityTagList } from '@/features/tags'
import { ReportEntityDialog } from '@/features/contributions'

interface CollectionDetailProps {
  slug: string
}

// PSY-892 D1/D6: sticky anchor nav sections, in locked page order
// (Items → Tags → Discussion). Module-level constant so the nav's
// IntersectionObserver effect keys on a stable reference.
const ANCHOR_SECTIONS: AnchorSection[] = [
  { id: 'items', label: 'Items' },
  { id: 'tags', label: 'Tags' },
  { id: 'discussion', label: 'Discussion' },
]

export function CollectionDetail({ slug }: CollectionDetailProps) {
  const router = useRouter()
  const pathname = usePathname()
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
  // PSY-894 D4: green "Collection updated" banner after a successful edit
  // save. The form closing + header re-render is the inherent confirmation;
  // the banner makes it explicit and auto-dismisses after ~3s. No toast —
  // the project has no toast library (PSY-608/609 convention).
  const [showUpdateSuccess, setShowUpdateSuccess] = useState(false)
  useEffect(() => {
    if (!showUpdateSuccess) return
    const timer = setTimeout(() => setShowUpdateSuccess(false), 3000)
    return () => clearTimeout(timer)
  }, [showUpdateSuccess])
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
  // The "this viewer has liked it" visual state — only meaningful for
  // authenticated viewers (anonymous viewers see the count but never a
  // filled heart).
  const showLiked = isAuthenticated && (collection.user_likes_this ?? false)

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
            onSaved={() => {
              setIsEditing(false)
              // PSY-894 D4: only a successful save shows the green banner.
              setShowUpdateSuccess(true)
            }}
            onCancel={() => setIsEditing(false)}
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

              {/* Action buttons — consolidated per PSY-892 D4. Each viewer
                  state shows 2-3 primary actions; secondary actions live in
                  the ⋯ overflow menu:
                  - Owner:            Like · Edit · Delete  |  ⋯ Share · Explore graph
                  - Non-owner (auth): Like · Subscribe      |  ⋯ Share · Explore graph · Fork · Report
                  - Logged out:       Like (sign-in prompt) |  ⋯ Share · Explore graph */}
              <div className="flex items-center gap-2 shrink-0">
                {/* PSY-352: Like toggle. Primary in every viewer state.
                    Authenticated viewers toggle; anonymous viewers are routed
                    to sign-in on click (D4 — same returnTo redirect as
                    FollowButton / AttendanceButton so they land back here
                    after signing in). Aggregate count only — privacy
                    decision: no list of likers exposed. */}
                <Button
                  variant="outline"
                  size="sm"
                  onClick={
                    isAuthenticated
                      ? handleToggleLike
                      : () =>
                          router.push(
                            `/auth?returnTo=${encodeURIComponent(pathname)}`
                          )
                  }
                  disabled={isAuthenticated && isLikePending}
                  aria-pressed={isAuthenticated ? showLiked : undefined}
                  aria-label={
                    !isAuthenticated
                      ? 'Sign in to like collection'
                      : showLiked
                        ? 'Unlike collection'
                        : 'Like collection'
                  }
                  className={cn(showLiked && 'text-primary')}
                  data-testid="collection-like-button"
                >
                  <Heart
                    className={cn(
                      'h-4 w-4 mr-1.5',
                      showLiked && 'fill-current'
                    )}
                  />
                  {collection.like_count}
                </Button>

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

                {/* Edit + Delete: the curator's daily actions stay primary
                    (D4); Edit's click behavior is the inline form (D5,
                    locked by PSY-894). */}
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

                {/* ⋯ overflow menu (D4): Share + Explore graph for everyone;
                    Fork + Report join for authenticated non-owners. */}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label="More actions"
                      data-testid="collection-overflow-trigger"
                    >
                      <MoreHorizontal className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onClick={handleShare}
                      data-testid="overflow-share"
                    >
                      <Share2 className="h-4 w-4" />
                      Share
                    </DropdownMenuItem>
                    {/* PSY-555 (was PSY-366): Explore graph toggle. Visible
                        whenever the collection has at least one item — every
                        entity type renders as a node in the multi-type
                        graph. */}
                    {hasItems && (
                      <DropdownMenuItem
                        onClick={() => setShowGraphOverride(!showGraph)}
                        data-testid="overflow-explore-graph"
                      >
                        <Network className="h-4 w-4" />
                        {showGraph ? 'Hide graph' : 'Explore graph'}
                      </DropdownMenuItem>
                    )}
                    {/* PSY-351: Clone/fork. Visible only when caller is
                        authenticated AND not the owner AND the source is
                        public. */}
                    {canClone && (
                      <DropdownMenuItem
                        onClick={handleClone}
                        disabled={cloneMutation.isPending}
                        aria-label="Fork collection"
                        data-testid="overflow-fork"
                      >
                        <GitFork className="h-4 w-4" />
                        {cloneMutation.isPending ? 'Forking...' : 'Fork'}
                      </DropdownMenuItem>
                    )}
                    {/* PSY-578: report a collection. Mirrors the Report
                        button on artist/venue/festival/show detail pages. */}
                    {canReport && (
                      <DropdownMenuItem
                        onClick={() => setIsReportOpen(true)}
                        aria-label="Report collection"
                        data-testid="overflow-report"
                      >
                        <Flag className="h-4 w-4" />
                        Report
                      </DropdownMenuItem>
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
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
            {/* PSY-894 D4: edit-save success confirmation — the form has
                already closed and the header re-rendered with new values;
                this banner makes the save explicit. Auto-dismisses ~3s. */}
            {showUpdateSuccess && (
              <MutationFeedback
                variant="success"
                testId="collection-update-success"
                message="Collection updated"
              />
            )}
            {/* PSY-892 D4: Share lives in the overflow menu now, so its
                "Copied!" feedback can't render on the trigger — surface it
                as an inline success banner instead. */}
            {showCopied && (
              <MutationFeedback
                variant="success"
                testId="share-copied"
                message="Link copied to clipboard"
              />
            )}
            {/* Fork-in-flight feedback: the overflow menu closes on click,
                taking its "Forking..." pending label with it — surface the
                in-flight state inline so a slow clone isn't dead air. */}
            {cloneMutation.isPending && (
              <p
                className="mt-2 flex items-center gap-1.5 text-sm text-muted-foreground"
                role="status"
                data-testid="fork-pending"
              >
                <Loader2
                  className="h-3.5 w-3.5 animate-spin"
                  aria-hidden="true"
                />
                Forking collection…
              </p>
            )}
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

      {/* PSY-892 D1: sticky anchor nav — jump links for the page's three
          long-form sections (Items · Tags · Discussion). Pins below the site
          TopBar; the active link tracks scroll position. */}
      <CollectionAnchorNav sections={ANCHOR_SECTIONS} />

      {/* PSY-555 (was PSY-366): collection graph (toggleable). Renders
          only when the user clicks "Explore graph" in the ⋯ overflow menu.
          The wrapper has `id="graph"` so Cmd+K deep-links resolve. */}
      {showGraph && hasItems && (
        <CollectionGraph slug={slug} collectionTitle={collection.title} />
      )}

      {/* Items list — leads the page (PSY-892 D6: visitors come for items).
          Carries the creator's "+ Add Items" affordance in its header (D7)
          and the `id="items"` anchor for the nav above. */}
      <CollectionItemsList
        items={items}
        slug={slug}
        isCreator={isCreator}
        displayMode={collection.display_mode}
      />

      {/* PSY-354: tag chips + picker — between Items and Discussion per
          PSY-892 D6 (tags are useful metadata but secondary to items).
          Reuses the same EntityTagList that renders on artist/release/etc
          detail pages — chips link to /tags/{slug} for the deep-dive (the
          collection-card override links to /collections?tag=<slug> instead
          because cards prefer the lateral "show me other collections like
          this" path). The per-collection 10-tag cap is enforced server-side
          in catalog.TagService.AddTagToEntity, so this picker honors the
          limit regardless of the picker's UI cap awareness. */}
      <div id="tags" className={cn('mt-8', ANCHOR_SECTION_SCROLL_MT)}>
        <EntityTagList
          entityType="collection"
          entityId={collection.id}
          isAuthenticated={isAuthenticated}
        />
      </div>

      {/* Discussion — stays at the page bottom (PSY-892 D2); the anchor nav
          provides the jump-to affordance. */}
      <div id="discussion" className={ANCHOR_SECTION_SCROLL_MT}>
        <CommentThread entityType="collection" entityId={collection.id} />
      </div>

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
  onSaved,
  onCancel,
}: {
  slug: string
  title: string
  description: string
  isPublic: boolean
  collaborative: boolean
  displayMode: CollectionDisplayMode
  coverImageUrl: string
  /** Called after a successful save (PSY-894 D4 — parent shows the banner). */
  onSaved: () => void
  /** Called when the form closes without saving (Cancel button or Esc). */
  onCancel: () => void
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

  // Single source of truth for "the form may be submitted" — keeps the Save
  // button's disabled state and the ⌘S shortcut in lockstep.
  const canSave =
    title.trim().length > 0 &&
    coverImageUrlError === null &&
    !updateMutation.isPending

  // Dirty tracking: Esc-to-cancel only fires on a pristine form. Discarding
  // typed work on a reflexive Esc press is a data-loss foot-gun (adversarial
  // review finding) — a dirty form requires the deliberate Cancel click.
  const isDirty =
    title !== initialTitle ||
    description !== initialDescription ||
    isPublic !== initialPublic ||
    collaborative !== initialCollaborative ||
    displayMode !== initialDisplayMode ||
    coverImageUrl !== initialCoverImageUrl

  const handleSave = () => {
    if (!canSave) return
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
      { onSuccess: () => onSaved() }
    )
  }

  // PSY-894 D3: keyboard shortcuts — Esc cancels, ⌘/Ctrl+S saves. Attached to
  // the form's root so they work from any focused field. handleSave's canSave
  // guard makes the shortcut respect the same validation as the Save button;
  // the isDirty guard keeps Esc from discarding typed work.
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      if (!isDirty) {
        onCancel()
      }
    } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
      e.preventDefault()
      handleSave()
    }
  }

  return (
    <div
      className="space-y-4 rounded-lg border border-border/50 bg-card p-4"
      onKeyDown={handleKeyDown}
    >
      <div>
        <label htmlFor="edit-title" className="text-sm font-medium mb-1.5 block">
          Title
        </label>
        <Input
          id="edit-title"
          value={title}
          onChange={e => setTitle(e.target.value)}
          autoFocus
          disabled={updateMutation.isPending}
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
          disabled={updateMutation.isPending}
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
            disabled={updateMutation.isPending}
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

      {/* PSY-894 D6: DS Checkbox primitives. Public is a plain toggle —
          NO publish quality gate (PSY-822 removed it; do not re-add). */}
      <div className="flex items-center gap-6">
        <div className="flex items-center gap-2">
          <Checkbox
            id="edit-is-public"
            checked={isPublic}
            onCheckedChange={checked => setIsPublic(checked === true)}
            disabled={updateMutation.isPending}
          />
          <label htmlFor="edit-is-public" className="text-sm cursor-pointer">
            Public
          </label>
        </div>

        <div className="flex items-center gap-2">
          <Checkbox
            id="edit-collaborative"
            checked={collaborative}
            onCheckedChange={checked => setCollaborative(checked === true)}
            disabled={updateMutation.isPending}
          />
          <label
            htmlFor="edit-collaborative"
            className="text-sm cursor-pointer"
          >
            Collaborative
          </label>
        </div>
      </div>

      {updateMutation.error && (
        <p className="text-sm text-destructive">
          {updateMutation.error instanceof Error
            ? updateMutation.error.message
            : 'Failed to update collection'}
        </p>
      )}

      <div className="flex items-center gap-2">
        <Button size="sm" onClick={handleSave} disabled={!canSave}>
          <Check className="h-4 w-4 mr-1" />
          {updateMutation.isPending ? 'Saving...' : 'Save'}
        </Button>
        <Button size="sm" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        {/* PSY-894 D3: keyboard shortcut hint (V1-optional affordance).
            Esc only cancels a pristine form, so the hint drops it once the
            form is dirty — the hint never promises something that won't
            happen. */}
        <p className="ml-auto hidden text-xs text-muted-foreground sm:block">
          {isDirty ? '⌘S to save' : 'Esc to cancel · ⌘S to save'}
        </p>
      </div>
    </div>
  )
}
