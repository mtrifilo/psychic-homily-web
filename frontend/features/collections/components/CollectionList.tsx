'use client'

import { useState, useMemo } from 'react'
import { Plus, Search, Library, Star, Clock, TrendingUp, User, X } from 'lucide-react'
import Link from 'next/link'
import { useDebounce } from 'use-debounce'
import {
  useCollections,
  useMyCollections,
  useCreateCollection,
  useBulkAddCollectionItems,
} from '../hooks'
import {
  AddItemsPicker,
  type StagedCollectionItem,
} from './AddItemsPicker'
import { CollectionCard } from './CollectionCard'
// MarkdownEditor is lazily loaded (dynamic ssr:false) so its `marked` +
// `dompurify` deps stay out of the global shared client chunk — see
// MarkdownEditorLazy / PSY-951.
import { MarkdownEditor } from './MarkdownEditorLazy'
import {
  MAX_COLLECTION_MARKDOWN_LENGTH,
  MAX_COVER_IMAGE_URL_LENGTH,
  validateCoverImageUrl,
} from '../types'
import { LoadingSpinner } from '@/components/shared'
import { badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useRouter, useSearchParams } from 'next/navigation'
import {
  COLLECTION_ENTITY_TYPES,
  getEntityTypeLabel,
  type Collection,
  type CollectionEntityType,
} from '../types'
import { cn } from '@/lib/utils'
import {
  COLLECTION_UNLIMITED,
  TIERS_HELP_PATH,
  getCollectionLimitForTier,
  getTierInfo,
} from '@/lib/tiers'

// PSY-586: tab values are URL-routable via `?tab=<value>`. Unknown values
// fall back to `all` silently (no redirect). The `yours` tab is auth-gated;
// `?tab=yours` from a logged-out user also coerces to `all` since the tab
// isn't rendered (Radix would otherwise highlight nothing).
const BROWSE_TABS = ['all', 'popular', 'recent', 'featured', 'yours'] as const
type BrowseTab = (typeof BROWSE_TABS)[number]

function isBrowseTab(value: string | null): value is BrowseTab {
  return value !== null && (BROWSE_TABS as readonly string[]).includes(value)
}

function resolveActiveTab(
  rawTab: string | null,
  isAuthenticated: boolean
): BrowseTab {
  if (!isBrowseTab(rawTab)) return 'all'
  if (rawTab === 'yours' && !isAuthenticated) return 'all'
  return rawTab
}

export function CollectionList() {
  const { isAuthenticated } = useAuthContext()
  const router = useRouter()
  const searchParams = useSearchParams()
  const [createDialogOpen, setCreateDialogOpen] = useState(false)

  const activeTab = resolveActiveTab(searchParams.get('tab'), isAuthenticated)

  const handleTabChange = (next: string) => {
    if (!isBrowseTab(next)) return
    // PSY-586: push (not replace) so back/forward restores the previous
    // tab. Preserve unrelated query params (e.g. PSY-354's `?tag=<slug>`).
    // Omit `tab=all` from the URL since it's the default.
    const params = new URLSearchParams(searchParams.toString())
    if (next === 'all') {
      params.delete('tab')
    } else {
      params.set('tab', next)
    }
    const qs = params.toString()
    router.push(qs ? `/collections?${qs}` : '/collections', { scroll: false })
  }

  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch] = useDebounce(searchInput, 300)
  const [entityTypeFilter, setEntityTypeFilter] = useState<CollectionEntityType | 'all'>('all')

  // Determine whether to filter featured on the backend
  const isFeaturedTab = activeTab === 'featured'
  const searchTerm = debouncedSearch.trim()

  // PSY-352: when the Popular tab is active, ask the server to sort by
  // HN gravity (likes / age^1.8). The client-side `subscriber_count` sort
  // we used to do here was an approximation; the server-side gravity sort
  // is the canonical ranking and includes recency-bias.
  const isPopularTab = activeTab === 'popular'

  // PSY-354: URL-driven tag filter. The chip on collection cards links to
  // `/collections?tag=<slug>`; reading from the URL keeps the filter
  // shareable + back-button-restorable. Single-tag for the MVP.
  const tagFilter = searchParams.get('tag') ?? ''

  // Clear the active tag pill — pushes a URL without `tag=`. Use
  // router.replace so the back button doesn't bounce back into the
  // filtered view (the filter clear is part of the same navigation
  // intent, not a separate history entry).
  const handleClearTag = () => {
    const next = new URLSearchParams(searchParams.toString())
    next.delete('tag')
    const qs = next.toString()
    router.replace(qs ? `/collections?${qs}` : '/collections', { scroll: false })
  }

  // Fetch public collections (with search + featured + entity-type filters)
  const {
    data: publicData,
    isLoading: publicLoading,
    error: publicError,
    refetch: publicRefetch,
  } = useCollections({
    search: searchTerm || undefined,
    featured: isFeaturedTab || undefined,
    entityType: entityTypeFilter === 'all' ? undefined : entityTypeFilter,
    sort: isPopularTab ? 'popular' : undefined,
    tag: tagFilter || undefined,
  })

  // Fetch user's own collections (only when on "yours" tab and authenticated).
  // PSY-580: pass the same search term the public-browse hook receives so the
  // Yours tab filters via the backend's expanded search (title / description /
  // item notes / tag names+aliases — PSY-355). Empty / whitespace short-
  // circuits inside the hook.
  const {
    data: myData,
    isLoading: myLoading,
    error: myError,
  } = useMyCollections({ search: searchTerm || undefined })

  // Determine which data to use based on active tab
  const isYoursTab = activeTab === 'yours'
  const isLoading = isYoursTab ? myLoading : publicLoading
  const error = isYoursTab ? myError : publicError
  const rawCollections = isYoursTab
    ? (myData?.collections ?? [])
    : (publicData?.collections ?? [])

  // PSY-352: Popular tab now uses server-side ordering — the API returns
  // results already sorted by HN gravity, so we render them as-is. The
  // Recent tab still uses a client-side sort (newest created_at) since the
  // backend default sort is updated_at; converting that is out of scope.
  // The Yours tab applies an entity-type filter client-side.
  const collections = useMemo(() => {
    let items = [...rawCollections]

    if (isYoursTab && entityTypeFilter !== 'all') {
      items = items.filter(
        c => (c.entity_type_counts?.[entityTypeFilter] ?? 0) > 0
      )
    }

    if (activeTab === 'recent') {
      items.sort(
        (a, b) =>
          new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
      )
    }

    return items
  }, [rawCollections, activeTab, isYoursTab, entityTypeFilter])

  // Available tabs (Yours only for authenticated users)
  const tabs: { value: BrowseTab; label: string; icon: React.ReactNode }[] = [
    { value: 'all', label: 'All', icon: <Library className="h-4 w-4" /> },
    { value: 'popular', label: 'Popular', icon: <TrendingUp className="h-4 w-4" /> },
    { value: 'recent', label: 'Recent', icon: <Clock className="h-4 w-4" /> },
    { value: 'featured', label: 'Featured', icon: <Star className="h-4 w-4" /> },
    ...(isAuthenticated
      ? [
          {
            value: 'yours' as BrowseTab,
            label: 'Yours',
            icon: <User className="h-4 w-4" />,
          },
        ]
      : []),
  ]

  return (
    <section className="w-full max-w-6xl">
      {/* Actions bar */}
      <div className="flex items-center justify-between gap-4 mb-6">
        {/* Search input */}
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="Search collections..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
            aria-label="Search collections"
          />
        </div>

        {/* Create button — PSY-823: drawer (Sheet) replaces the legacy
            inline Dialog so the integrated AddItemsPicker has room to
            scroll a 200-row staged list without clipping. */}
        {isAuthenticated && (
          <Sheet open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
            <SheetTrigger asChild>
              <Button size="sm">
                <Plus className="h-4 w-4 mr-1.5" />
                Create Collection
              </Button>
            </SheetTrigger>
            <SheetContent
              side="right"
              className="w-full sm:max-w-xl flex flex-col overflow-y-auto"
            >
              <SheetHeader>
                <SheetTitle>Create Collection</SheetTitle>
              </SheetHeader>
              <div className="px-4 pb-4">
                <CreateCollectionForm
                  onSuccess={(newSlug) => {
                    setCreateDialogOpen(false)
                    if (newSlug) {
                      router.push(`/collections/${newSlug}`)
                    }
                  }}
                  onCancel={() => setCreateDialogOpen(false)}
                />
              </div>
            </SheetContent>
          </Sheet>
        )}
      </div>

      {/* Tabs — PSY-895 D1 (implemented in PSY-905): mode tabs read as
          NAVIGATION, not pills. Underline-active style (DS TabTrigger
          pattern, matching PSY-898's detail-page anchor nav) replaces the
          boxed pill row so the tabs sit a clear hierarchy level above the
          entity-type refinement chips below.
          NOTE for the future cross-browse cohesion ticket: when this
          underline pattern propagates to other browse pages, extract these
          overrides into a `variant="underline"` on ui/tabs.tsx instead of
          copy-pasting this string — see PSY-895's flagged follow-ups. */}
      <Tabs
        value={activeTab}
        onValueChange={handleTabChange}
        className="w-full"
      >
        <TabsList className="mb-4 h-auto w-full justify-start gap-1 rounded-none border-b border-border/50 bg-transparent p-0 text-muted-foreground">
          {tabs.map((tab) => (
            <TabsTrigger
              key={tab.value}
              value={tab.value}
              className="h-10 flex-none gap-1.5 rounded-none border-b-2 border-transparent px-3 text-muted-foreground hover:text-foreground data-[state=active]:border-b-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none dark:data-[state=active]:border-b-primary dark:data-[state=active]:bg-transparent"
            >
              {tab.icon}
              {tab.label}
            </TabsTrigger>
          ))}
        </TabsList>

        {/* Entity type filter chips */}
        <div className="mb-3 flex flex-wrap gap-1.5" role="group" aria-label="Filter by entity type">
          <TypeChip
            active={entityTypeFilter === 'all'}
            onClick={() => setEntityTypeFilter('all')}
            label="All types"
          />
          {COLLECTION_ENTITY_TYPES.map(type => (
            <TypeChip
              key={type}
              active={entityTypeFilter === type}
              onClick={() => setEntityTypeFilter(type)}
              label={`${getEntityTypeLabel(type)}s`}
            />
          ))}
        </div>

        {/* PSY-354: active tag-filter chip. Distinct from the entity-type
            chips above (those are toggleable filters; this is a "you are
            currently filtering" indicator with an X-to-clear). Empty when
            ?tag=<slug> isn't present in the URL. PSY-905: squared to match
            the entity-type chips (rounded-full is DS-banned). */}
        {tagFilter && (
          <div
            className="mb-6 flex flex-wrap items-center gap-2 text-sm"
            role="status"
            aria-live="polite"
            data-testid="collection-tag-filter-pill"
          >
            <span className="text-muted-foreground">Tagged with</span>
            <span
              className={cn(
                'inline-flex items-center gap-1.5 rounded-md border px-2.5 py-0.5',
                'border-primary/40 bg-primary/10 text-primary'
              )}
            >
              <span className="font-medium">{tagFilter}</span>
              <button
                type="button"
                onClick={handleClearTag}
                aria-label={`Clear tag filter (${tagFilter})`}
                className="rounded-md p-0.5 hover:bg-primary/20 focus:outline-none focus:ring-2 focus:ring-ring"
                data-testid="collection-tag-filter-clear"
              >
                <X className="h-3 w-3" />
              </button>
            </span>
          </div>
        )}

        {/* All tab content areas render the same grid — content differs by data source */}
        {tabs.map((tab) => (
          <TabsContent key={tab.value} value={tab.value}>
            <CollectionGrid
              collections={collections}
              isLoading={isLoading}
              error={error}
              onRetry={() => publicRefetch()}
              emptyState={
                <EmptyState
                  tab={tab.value}
                  hasSearch={!!searchTerm}
                  isAuthenticated={isAuthenticated}
                  onCreateClick={() => setCreateDialogOpen(true)}
                />
              }
            />
          </TabsContent>
        ))}
      </Tabs>
    </section>
  )
}

// ──────────────────────────────────────────────
// Entity Type Filter Chip
// ──────────────────────────────────────────────

/**
 * PSY-895 D1 (implemented in PSY-905): entity-type chips use the DS Badge
 * shape — square-ish (`rounded-md`), not pills (`rounded-full` is banned by
 * the DS editorial direction). Active state is a subtle Secondary fill, NOT
 * the previous solid primary that out-shouted the mode tabs above; the chips
 * read as a refinement level below the tabs' navigation level.
 */
function TypeChip({
  active,
  onClick,
  label,
}: {
  active: boolean
  onClick: () => void
  label: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        badgeVariants({ variant: active ? 'secondary' : 'outline' }),
        // py-1 restores the pre-redesign tap-target height (≥24px, WCAG
        // 2.5.8) — the Badge primitive's py-0.5 is sized for non-interactive
        // chips.
        'cursor-pointer py-1',
        !active &&
          'border-border/60 text-muted-foreground hover:border-border hover:text-foreground'
      )}
    >
      {label}
    </button>
  )
}

// ──────────────────────────────────────────────
// Collection Grid
// ──────────────────────────────────────────────

function CollectionGrid({
  collections,
  isLoading,
  error,
  onRetry,
  emptyState,
}: {
  collections: Collection[]
  isLoading: boolean
  error: Error | null
  onRetry: () => void
  emptyState: React.ReactNode
}) {
  if (isLoading && collections.length === 0) {
    return (
      <div className="flex justify-center items-center py-12">
        <LoadingSpinner />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load collections. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={onRetry}>
          Retry
        </Button>
      </div>
    )
  }

  if (collections.length === 0) {
    return <>{emptyState}</>
  }

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {collections.map((collection) => (
        <CollectionCard key={collection.id} collection={collection} />
      ))}
    </div>
  )
}

// ──────────────────────────────────────────────
// Empty States
// ──────────────────────────────────────────────

function EmptyState({
  tab,
  hasSearch,
  isAuthenticated,
  onCreateClick,
}: {
  tab: BrowseTab
  hasSearch: boolean
  isAuthenticated: boolean
  onCreateClick: () => void
}) {
  if (hasSearch) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Search className="h-12 w-12 mx-auto mb-3 text-muted-foreground/30" />
        <p className="text-lg mb-1">No collections found</p>
        <p className="text-sm">
          Try a different search term or browse all collections.
        </p>
      </div>
    )
  }

  if (tab === 'yours') {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Library className="h-12 w-12 mx-auto mb-3 text-muted-foreground/30" />
        <p className="text-lg mb-1">You haven&apos;t created any collections yet</p>
        <p className="text-sm mb-4">
          Collections are curated lists of shows, artists, releases, and more.
        </p>
        <Button size="sm" onClick={onCreateClick}>
          <Plus className="h-4 w-4 mr-1.5" />
          Create Collection
        </Button>
      </div>
    )
  }

  if (tab === 'featured') {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <Star className="h-12 w-12 mx-auto mb-3 text-muted-foreground/30" />
        <p className="text-lg mb-1">No featured collections yet</p>
        <p className="text-sm">
          Featured collections are curated by the community and highlighted by moderators.
        </p>
      </div>
    )
  }

  return (
    <div className="text-center py-12 text-muted-foreground">
      <Library className="h-12 w-12 mx-auto mb-3 text-muted-foreground/30" />
      <p className="text-lg mb-1">No collections yet</p>
      {isAuthenticated ? (
        <>
          <p className="text-sm mb-4">Be the first to create one!</p>
          <Button size="sm" onClick={onCreateClick}>
            <Plus className="h-4 w-4 mr-1.5" />
            Create Collection
          </Button>
        </>
      ) : (
        <p className="text-sm">
          Sign in to create and curate your own collections.
        </p>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Create Collection Form (inline in dialog)
// ──────────────────────────────────────────────

function CreateCollectionForm({
  onSuccess,
  onCancel,
}: {
  onSuccess: (slug?: string) => void
  /** PSY-823: cancel affordance for the Sheet footer. Optional so legacy
   *  Dialog callers can still mount the form without a cancel button. */
  onCancel?: () => void
}) {
  const createMutation = useCreateCollection()
  const bulkAddMutation = useBulkAddCollectionItems()
  const { user } = useAuthContext()
  // PSY-358: per-tier owned-collection cap. Read user's collections so we
  // can render "X of Y collections" before they submit. We filter to OWNED
  // (creator_id == user.id) and exclude FORKS — same shape the backend
  // uses for enforcement. Admins bypass the cap entirely.
  const myCollections = useMyCollections()
  const ownedCount = useMemo(() => {
    if (!user?.id) return 0
    const userId = Number(user.id)
    return (myCollections.data?.collections ?? []).filter(
      (c) => c.creator_id === userId && c.forked_from_collection_id == null
    ).length
  }, [myCollections.data?.collections, user?.id])

  const tier = user?.user_tier ?? 'new_user'
  const limit = getCollectionLimitForTier(tier)
  const isUnlimited = user?.is_admin === true || limit === COLLECTION_UNLIMITED
  const atOrOverCap = !isUnlimited && ownedCount >= limit
  const tierLabel = getTierInfo(tier).label

  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [isPublic, setIsPublic] = useState(true)
  const [collaborative, setCollaborative] = useState(false)
  // PSY-823: items staged via the AddItemsPicker. Submitted via the bulk-add
  // endpoint immediately after the collection is created — sequential because
  // the bulk endpoint is keyed on the new collection's slug.
  const [stagedItems, setStagedItems] = useState<StagedCollectionItem[]>([])
  // Post-create per-row error display from the bulk-add response. Surfaced
  // inline so the user knows which paste rows didn't commit before they
  // navigate to the new collection's detail page.
  const [bulkAddRejectedCount, setBulkAddRejectedCount] = useState(0)
  // PSY-585: cover image URL on create — mirrors the Edit form's field
  // shape (validation, preview, clear button) so users can set the cover
  // in one step instead of create-then-immediately-edit. Empty string is
  // the "no cover" affordance and is the default; we omit the field from
  // the request payload entirely when it's empty so the backend stores
  // null rather than an empty string.
  const [coverImageUrl, setCoverImageUrl] = useState('')
  const [coverImageUrlTouched, setCoverImageUrlTouched] = useState(false)

  const trimmedCoverUrl = coverImageUrl.trim()
  const coverImageUrlError = validateCoverImageUrl(coverImageUrl)
  const showCoverImageUrlError =
    coverImageUrlTouched && coverImageUrlError !== null
  const showCoverImagePreview =
    trimmedCoverUrl.length > 0 && coverImageUrlError === null

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim()) return
    if (coverImageUrlError) return

    // PSY-823: sequential flow — create collection, then bulk-add staged
    // items. The bulk endpoint requires the collection's slug, so this
    // pair can't collapse into a single backend hit without a new
    // composite endpoint (out of scope for V1).
    try {
      const newCollection = await createMutation.mutateAsync({
        title: title.trim(),
        description: description.trim() || undefined,
        is_public: isPublic,
        collaborative,
        cover_image_url:
          trimmedCoverUrl.length === 0 ? undefined : trimmedCoverUrl,
      })

      if (stagedItems.length > 0 && newCollection?.slug) {
        try {
          const bulkResp = await bulkAddMutation.mutateAsync({
            slug: newCollection.slug,
            items: stagedItems.map((s) => ({
              entity_type: s.entityType,
              entity_id: s.entityId,
            })),
          })
          if (bulkResp.errors.length > 0) {
            // Collection still created; surface the rejected count so the
            // user can investigate on the detail page.
            setBulkAddRejectedCount(bulkResp.errors.length)
          }
        } catch (bulkErr) {
          // Bulk-add failed entirely (network/5xx). The collection still
          // exists. We navigate to the new collection so the user lands
          // on its detail page (where the empty-state picker prompts a
          // retry). Inline failure feedback in the drawer would help, but
          // the drawer auto-closes on onSuccess — surfacing the failure
          // here would require holding the user in the drawer, which
          // conflicts with the same-title slug-collision retry path. V1
          // accepts the silent navigation; follow-up handles total-failure
          // UX. See PSY-829 / new follow-up.
          // eslint-disable-next-line no-console
          console.error('bulk-add failed after collection create', bulkErr)
        }
      }

      setTitle('')
      setDescription('')
      setCoverImageUrl('')
      setCoverImageUrlTouched(false)
      setStagedItems([])
      onSuccess(newCollection?.slug)
    } catch {
      // create-collection failure: the mutation surfaces its error inline
      // (see the existing createMutation.error render below) — nothing
      // more to do here.
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {/* PSY-358: per-tier owned-collection limit explainer. Hidden for
          admins and unlimited tiers (local_ambassador). */}
      {!isUnlimited && (
        <div
          className={cn(
            'rounded-md border px-3 py-2 text-xs',
            atOrOverCap
              ? 'border-destructive/50 bg-destructive/5 text-destructive'
              : 'border-border bg-muted/30 text-muted-foreground'
          )}
          data-testid="collection-tier-limit-banner"
        >
          {atOrOverCap ? (
            <>
              You&apos;ve reached your limit of {limit} collections at the{' '}
              <span className="font-medium">{tierLabel}</span> tier ({ownedCount}/{limit}).{' '}
              <Link href={TIERS_HELP_PATH} className="underline">
                Learn how to advance
              </Link>{' '}
              or delete an existing collection to make room.
            </>
          ) : (
            <>
              {ownedCount} of {limit} collections used at the{' '}
              <span className="font-medium">{tierLabel}</span> tier.{' '}
              <Link href={TIERS_HELP_PATH} className="underline">
                Tier limits
              </Link>
              .
            </>
          )}
        </div>
      )}

      <div>
        <label
          htmlFor="collection-title"
          className="text-sm font-medium mb-1.5 block"
        >
          Title
        </label>
        <Input
          id="collection-title"
          value={title}
          onChange={e => setTitle(e.target.value)}
          placeholder="My Favorite Artists"
          required
          autoFocus
        />
      </div>

      <div>
        <label
          htmlFor="collection-description"
          className="text-sm font-medium mb-1.5 block"
        >
          Description (optional)
        </label>
        <MarkdownEditor
          id="collection-description"
          value={description}
          onChange={setDescription}
          placeholder="A brief description of this collection... (markdown supported)"
          rows={3}
          maxLength={MAX_COLLECTION_MARKDOWN_LENGTH}
          testId="create-collection-description-editor"
        />
      </div>

      {/* PSY-585: Cover image URL (parity with Edit form). Optional; empty
          submits cleanly with no cover. Validation, inline preview, and
          clear-button mirror the Edit form's shape — only the helper text
          differs (no "remove the current cover" half on create). */}
      <div>
        <label
          htmlFor="create-cover-image-url"
          className="text-sm font-medium mb-1.5 block"
        >
          Cover image URL{' '}
          <span className="text-xs font-normal text-muted-foreground">
            (optional)
          </span>
        </label>
        <div className="flex gap-2">
          <Input
            id="create-cover-image-url"
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
                ? 'create-cover-image-url-error'
                : 'create-cover-image-url-help'
            }
            data-testid="create-cover-image-url-input"
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
              data-testid="create-cover-image-url-clear"
            >
              <X className="h-4 w-4 mr-1" />
              Clear
            </Button>
          )}
        </div>
        {showCoverImageUrlError ? (
          <p
            id="create-cover-image-url-error"
            className="text-xs text-destructive mt-1.5"
            role="alert"
          >
            {coverImageUrlError}
          </p>
        ) : (
          <p
            id="create-cover-image-url-help"
            className="text-xs text-muted-foreground mt-1.5"
          >
            Paste a direct image URL (e.g. Bandcamp art).
          </p>
        )}
        {showCoverImagePreview && (
          <div className="mt-2 h-24 w-24 rounded-lg overflow-hidden border border-border/50 bg-muted/50">
            <img
              src={trimmedCoverUrl}
              alt="Cover image preview"
              className="h-full w-full object-cover"
              data-testid="create-cover-image-url-preview"
            />
          </div>
        )}
      </div>

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

      {/* PSY-823: integrated AddItemsPicker — stages items as the user
          fills the form so they can land a fully populated collection in
          one drawer interaction. Staged items are POSTed to the bulk-add
          endpoint immediately after the collection is created. */}
      <div className="border-t border-border/50 pt-4">
        <AddItemsPicker
          existingItems={[]}
          stagedItems={stagedItems}
          onStagedItemsChange={setStagedItems}
        />
      </div>

      {createMutation.error && (
        <p className="text-sm text-destructive" data-testid="collection-create-error">
          {createMutation.error instanceof Error
            ? createMutation.error.message
            : 'Failed to create collection'}
        </p>
      )}

      {bulkAddRejectedCount > 0 && (
        <p
          className="text-sm text-amber-600 dark:text-amber-400"
          data-testid="collection-create-bulk-rejected"
        >
          Collection created, but {bulkAddRejectedCount}{' '}
          {bulkAddRejectedCount === 1 ? 'item' : 'items'} couldn&apos;t be added.
          You can retry from the collection page.
        </p>
      )}

      <div className="flex justify-end gap-2">
        {onCancel && (
          <Button
            type="button"
            variant="outline"
            onClick={onCancel}
            disabled={createMutation.isPending || bulkAddMutation.isPending}
          >
            Cancel
          </Button>
        )}
        <Button
          type="submit"
          disabled={
            !title.trim() ||
            coverImageUrlError !== null ||
            createMutation.isPending ||
            bulkAddMutation.isPending ||
            atOrOverCap
          }
        >
          {createMutation.isPending || bulkAddMutation.isPending
            ? 'Creating...'
            : 'Create'}
        </Button>
      </div>
    </form>
  )
}
