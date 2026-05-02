'use client'

import { useState, useMemo } from 'react'
import { Plus, Search, Library, Star, Clock, TrendingUp, User, X } from 'lucide-react'
import { useDebounce } from 'use-debounce'
import { useCollections, useMyCollections, useCreateCollection } from '../hooks'
import { CollectionCard } from './CollectionCard'
import { MarkdownEditor } from './MarkdownEditor'
import { MAX_COLLECTION_MARKDOWN_LENGTH } from '../types'
import { LoadingSpinner } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
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

type BrowseTab = 'all' | 'popular' | 'recent' | 'featured' | 'yours'

export function CollectionList() {
  const { isAuthenticated } = useAuthContext()
  const router = useRouter()
  const searchParams = useSearchParams()
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<BrowseTab>('all')
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

  // Fetch user's own collections (only when on "yours" tab and authenticated)
  const {
    data: myData,
    isLoading: myLoading,
    error: myError,
  } = useMyCollections()

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

        {/* Create button */}
        {isAuthenticated && (
          <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="h-4 w-4 mr-1.5" />
                Create Collection
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create Collection</DialogTitle>
              </DialogHeader>
              <CreateCollectionForm
                onSuccess={(newSlug) => {
                  setCreateDialogOpen(false)
                  if (newSlug) {
                    router.push(`/collections/${newSlug}`)
                  }
                }}
              />
            </DialogContent>
          </Dialog>
        )}
      </div>

      {/* Tabs */}
      <Tabs
        value={activeTab}
        onValueChange={(v) => setActiveTab(v as BrowseTab)}
        className="w-full"
      >
        <TabsList className="mb-4">
          {tabs.map((tab) => (
            <TabsTrigger key={tab.value} value={tab.value} className="gap-1.5">
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

        {/* PSY-354: active tag-filter pill. Distinct from the entity-type
            chips above (those are toggleable filters; this is a "you are
            currently filtering" indicator with an X-to-clear). Empty when
            ?tag=<slug> isn't present in the URL. */}
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
                'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5',
                'border-primary/40 bg-primary/10 text-primary'
              )}
            >
              <span className="font-medium">{tagFilter}</span>
              <button
                type="button"
                onClick={handleClearTag}
                aria-label={`Clear tag filter (${tagFilter})`}
                className="rounded-full p-0.5 hover:bg-primary/20 focus:outline-none focus:ring-2 focus:ring-ring"
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
        'rounded-full border px-3 py-1 text-xs font-medium transition-colors',
        active
          ? 'border-primary bg-primary text-primary-foreground'
          : 'border-border/60 bg-background text-muted-foreground hover:text-foreground hover:border-border'
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

function CreateCollectionForm({ onSuccess }: { onSuccess: (slug?: string) => void }) {
  const createMutation = useCreateCollection()
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [isPublic, setIsPublic] = useState(true)
  const [collaborative, setCollaborative] = useState(false)

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim()) return

    createMutation.mutate(
      {
        title: title.trim(),
        description: description.trim() || undefined,
        is_public: isPublic,
        collaborative,
      },
      {
        onSuccess: (data) => {
          setTitle('')
          setDescription('')
          onSuccess(data?.slug)
        },
      }
    )
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
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

      {createMutation.error && (
        <p className="text-sm text-destructive">
          {createMutation.error instanceof Error
            ? createMutation.error.message
            : 'Failed to create collection'}
        </p>
      )}

      <div className="flex justify-end gap-2">
        <Button
          type="submit"
          disabled={!title.trim() || createMutation.isPending}
        >
          {createMutation.isPending ? 'Creating...' : 'Create'}
        </Button>
      </div>
    </form>
  )
}
