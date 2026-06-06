'use client'

import { useState, useMemo } from 'react'
import { Plus, Search, Library, Star, Clock, TrendingUp, User, X } from 'lucide-react'
import { useDebounce } from 'use-debounce'
import { useCollections, useMyCollections } from '../hooks'
import { CollectionCard } from './CollectionCard'
import { useCreateCollectionDrawer } from './CreateCollectionDrawer'
import { LoadingSpinner } from '@/components/shared'
import { badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
  const { openCreateDrawer } = useCreateCollectionDrawer()

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

        {/* PSY-961: the Create drawer is now an app-level component
            (CreateCollectionDrawerProvider, mounted at the root) so it can be
            opened from entity pages too — this button just opens it. */}
        {isAuthenticated && (
          <Button size="sm" onClick={() => openCreateDrawer()}>
            <Plus className="h-4 w-4 mr-1.5" />
            Create Collection
          </Button>
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
                  onCreateClick={() => openCreateDrawer()}
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
