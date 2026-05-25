'use client'

/**
 * /admin/featured — admin UI for the /explore landing's two editorial
 * slots (Featured Bill + Featured Collection). Wires up PSY-835 backend
 * (PSY-838 frontend).
 *
 * Layout: two parallel panels (Bill / Collection). Each panel has
 *   - the current active card (read from /explore/featured for hydrated
 *     referent details; the admin list endpoint returns only entity_id).
 *   - a "Set new" form (entity picker → curator note → Save).
 *
 * No cadence UI — admin sets a slot whenever they want; current pick
 * stays visible until retired or replaced (locked product decision per
 * docs/open-questions/explore-landing.md).
 *
 * Mutation feedback follows the project convention
 * (pattern_mutation_feedback.md): inline banners via StatusBanner /
 * InlineErrorBanner. No toast library.
 */

import { useCallback, useMemo, useState } from 'react'
import Link from 'next/link'
import Image from 'next/image'
import {
  CalendarDays,
  Eye,
  Library,
  Loader2,
  Search,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { InlineErrorBanner, StatusBanner } from '@/components/shared'
import { useEntitySearch } from '@/lib/hooks/common/useEntitySearch'
import {
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE,
  type EntitySearchResult,
} from '@/lib/hooks/common/useEntitySearch'
import { useDebounce } from 'use-debounce'
import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import {
  MarkdownContent,
  MarkdownEditor,
} from '@/features/collections/components/MarkdownEditor'
import {
  useExploreFeatured,
  useRetireFeaturedSlot,
  useSetFeaturedSlot,
} from './useFeaturedSlots'
import {
  FEATURED_SLOT_LABEL,
  MAX_CURATOR_NOTE_LENGTH,
  type ExploreFeaturedBill,
  type ExploreFeaturedCollection,
  type FeaturedSlotType,
} from './types'

// ──────────────────────────────────────────────
// Collection search
// ──────────────────────────────────────────────
//
// `useEntitySearch` covers artists/venues/shows/releases/labels/festivals/
// tags — collections are intentionally excluded from the cross-entity
// search (they're a meta-entity over the others). For the Featured
// Collection picker we hit `GET /collections?search=` directly. The
// result shape is `CollectionListResponse` — title + slug + id +
// cover_image_url is everything the picker tile needs.

interface CollectionSearchItem {
  id: number
  title: string
  slug: string
  cover_image_url?: string | null
  creator_name?: string
}

interface CollectionListResponseEnvelope {
  collections: CollectionSearchItem[]
  total: number
}

function useCollectionSearch(query: string, enabled: boolean) {
  const [debouncedQuery] = useDebounce(query.trim(), 300)
  const isLongEnough = debouncedQuery.length >= 2
  return useQuery({
    queryKey: ['collections', 'admin-featured-search', debouncedQuery],
    queryFn: () =>
      apiRequest<CollectionListResponseEnvelope>(
        `${API_ENDPOINTS.COLLECTIONS.LIST}?search=${encodeURIComponent(debouncedQuery)}&limit=10`
      ),
    enabled: enabled && isLongEnough,
    staleTime: 30 * 1000,
  })
}

// ──────────────────────────────────────────────
// Active-card sub-components
// ──────────────────────────────────────────────

function ActiveBillCard({
  bill,
  onRetire,
  isRetiring,
}: {
  bill: ExploreFeaturedBill
  onRetire: () => void
  isRetiring: boolean
}) {
  const eventDate = new Date(bill.event_date)
  const dateLabel = Number.isNaN(eventDate.getTime())
    ? null
    : eventDate.toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
      })

  return (
    <div
      className="rounded-lg border border-border/60 bg-card p-4 space-y-3"
      data-testid="featured-admin-active-bill"
    >
      <div className="flex items-start gap-3">
        <div className="h-14 w-14 shrink-0 rounded bg-muted/50 flex items-center justify-center overflow-hidden">
          {bill.image_url ? (
            <Image
              src={bill.image_url}
              alt=""
              width={56}
              height={56}
              className="h-full w-full object-cover"
              unoptimized
            />
          ) : (
            <CalendarDays className="h-5 w-5 text-muted-foreground/60" />
          )}
        </div>
        <div className="flex-1 min-w-0">
          <Link
            href={`/shows/${bill.slug}`}
            className="text-base font-semibold hover:underline truncate block"
            target="_blank"
            rel="noreferrer"
          >
            {bill.headliner_name || bill.title || `Show #${bill.id}`}
          </Link>
          <p className="text-xs text-muted-foreground truncate">
            {bill.venue_name}
            {bill.venue_city && bill.venue_state ? ` · ${bill.venue_city}, ${bill.venue_state}` : ''}
            {dateLabel ? ` · ${dateLabel}` : ''}
          </p>
        </div>
      </div>

      {bill.curator_note_html && (
        <div className="rounded-md border border-border/30 bg-muted/20 px-3 py-2">
          <div className="flex items-center gap-1 text-[11px] uppercase tracking-wide text-muted-foreground mb-1">
            <Sparkles className="h-3 w-3" />
            Curator note
          </div>
          <MarkdownContent
            html={bill.curator_note_html}
            testId="featured-admin-active-bill-note"
          />
        </div>
      )}

      <div className="flex items-center justify-end gap-2 border-t border-border/40 pt-3">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRetire}
          disabled={isRetiring}
          data-testid="featured-admin-retire-bill"
        >
          {isRetiring ? (
            <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
          ) : (
            <Trash2 className="h-3.5 w-3.5 mr-1.5" />
          )}
          Retire current
        </Button>
      </div>
    </div>
  )
}

function ActiveCollectionCard({
  collection,
  onRetire,
  isRetiring,
}: {
  collection: ExploreFeaturedCollection
  onRetire: () => void
  isRetiring: boolean
}) {
  return (
    <div
      className="rounded-lg border border-border/60 bg-card p-4 space-y-3"
      data-testid="featured-admin-active-collection"
    >
      <div className="flex items-start gap-3">
        <div className="h-14 w-14 shrink-0 rounded bg-muted/50 flex items-center justify-center overflow-hidden">
          {collection.cover_image_url ? (
            <Image
              src={collection.cover_image_url}
              alt=""
              width={56}
              height={56}
              className="h-full w-full object-cover"
              unoptimized
            />
          ) : (
            <Library className="h-5 w-5 text-muted-foreground/60" />
          )}
        </div>
        <div className="flex-1 min-w-0">
          <Link
            href={`/collections/${collection.slug}`}
            className="text-base font-semibold hover:underline truncate block"
            target="_blank"
            rel="noreferrer"
          >
            {collection.title || `Collection #${collection.id}`}
          </Link>
          {collection.description && (
            <p className="text-xs text-muted-foreground truncate">
              {collection.description}
            </p>
          )}
        </div>
      </div>

      {collection.curator_note_html && (
        <div className="rounded-md border border-border/30 bg-muted/20 px-3 py-2">
          <div className="flex items-center gap-1 text-[11px] uppercase tracking-wide text-muted-foreground mb-1">
            <Sparkles className="h-3 w-3" />
            Curator note
          </div>
          <MarkdownContent
            html={collection.curator_note_html}
            testId="featured-admin-active-collection-note"
          />
        </div>
      )}

      <div className="flex items-center justify-end gap-2 border-t border-border/40 pt-3">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onRetire}
          disabled={isRetiring}
          data-testid="featured-admin-retire-collection"
        >
          {isRetiring ? (
            <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
          ) : (
            <Trash2 className="h-3.5 w-3.5 mr-1.5" />
          )}
          Retire current
        </Button>
      </div>
    </div>
  )
}

function NoActivePick({ slotType }: { slotType: FeaturedSlotType }) {
  return (
    <div
      className="rounded-lg border border-dashed border-border/60 bg-muted/10 p-6 text-center"
      data-testid={`featured-admin-empty-${slotType}`}
    >
      <p className="text-sm text-muted-foreground">
        No active {FEATURED_SLOT_LABEL[slotType]}. Pick one below — the
        /explore page collapses this slot until you do.
      </p>
    </div>
  )
}

// ──────────────────────────────────────────────
// Set-new form — bill (delegates to useEntitySearch.shows)
// ──────────────────────────────────────────────

interface PickedShow {
  id: number
  name: string
  href: string
  subtitle?: string | null
}

function SetBillForm() {
  const [query, setQuery] = useState('')
  const [picked, setPicked] = useState<PickedShow | null>(null)
  const [note, setNote] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  const setMutation = useSetFeaturedSlot()
  // useEntitySearch fans out to seven endpoints; the bill picker only
  // surfaces `shows` so the other groups are ignored at render time.
  const { data: searchResults, isSearching, searchError } = useEntitySearch({
    query,
    enabled: !picked,
  })

  const showResults = searchResults?.shows ?? []

  const handlePick = useCallback((r: EntitySearchResult) => {
    setPicked({ id: r.id, name: r.name, href: r.href, subtitle: r.subtitle })
    setQuery('')
  }, [])

  const handleSave = useCallback(() => {
    if (!picked) return
    setError(null)
    setSuccessMessage(null)
    setMutation.mutate(
      {
        slot_type: 'bill',
        entity_id: picked.id,
        curator_note: note.trim() ? note.trim() : null,
      },
      {
        onSuccess: () => {
          setSuccessMessage(`Featured Bill set to "${picked.name}"`)
          setPicked(null)
          setNote('')
        },
        onError: (err) => {
          setError(err instanceof Error ? err.message : 'Failed to set Featured Bill.')
        },
      }
    )
  }, [picked, note, setMutation])

  return (
    <div className="space-y-3" data-testid="featured-admin-set-bill-form">
      <h3 className="text-sm font-semibold flex items-center gap-1.5">
        <Sparkles className="h-3.5 w-3.5" />
        Set new Featured Bill
      </h3>

      {/* Banners */}
      {successMessage && (
        <StatusBanner
          variant="success"
          dismissAfterMs={4000}
          onDismiss={() => setSuccessMessage(null)}
          testId="featured-admin-set-bill-success"
        >
          <p className="text-sm">{successMessage}</p>
        </StatusBanner>
      )}
      {error && (
        <InlineErrorBanner testId="featured-admin-set-bill-error">
          {error}
        </InlineErrorBanner>
      )}

      {/* Picker — search until something is chosen, then show the selected card. */}
      {!picked ? (
        <>
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              type="text"
              placeholder="Search shows by headliner, venue, date..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="pl-9"
              data-testid="featured-admin-bill-search-input"
              aria-label="Search shows"
            />
          </div>

          {query.trim().length >= 2 && (
            <div className="max-h-64 overflow-y-auto rounded-md border border-border/40 bg-card">
              {isSearching ? (
                <div className="flex items-center justify-center py-4">
                  <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                </div>
              ) : searchError ? (
                <div className="p-3">
                  <InlineErrorBanner>
                    {ENTITY_SEARCH_UNAVAILABLE_MESSAGE}
                  </InlineErrorBanner>
                </div>
              ) : showResults.length === 0 ? (
                <p className="text-sm text-muted-foreground py-3 text-center">
                  No shows match &quot;{query}&quot;
                </p>
              ) : (
                <ul className="divide-y divide-border/30">
                  {showResults.map((r) => (
                    <li key={r.id}>
                      <button
                        type="button"
                        onClick={() => handlePick(r)}
                        className="w-full text-left p-2 hover:bg-muted/50 flex items-center gap-3"
                        data-testid={`featured-admin-bill-result-${r.id}`}
                      >
                        <CalendarDays className="h-4 w-4 text-muted-foreground/60 shrink-0" />
                        <span className="text-sm truncate">{r.name}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </>
      ) : (
        <div
          className="rounded-md border border-border/60 bg-card p-3 flex items-start gap-3"
          data-testid="featured-admin-bill-selected"
        >
          <CalendarDays className="h-5 w-5 text-muted-foreground/70 shrink-0 mt-0.5" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium truncate">{picked.name}</p>
            {picked.subtitle && (
              <p className="text-xs text-muted-foreground truncate">
                {picked.subtitle}
              </p>
            )}
            <Link
              href={picked.href}
              target="_blank"
              rel="noreferrer"
              className="text-[11px] text-muted-foreground hover:underline inline-flex items-center gap-0.5 mt-1"
            >
              <Eye className="h-3 w-3" />
              View on site
            </Link>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0"
            onClick={() => setPicked(null)}
            aria-label="Clear selection"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      )}

      {/* Curator note — markdown editor, same primitive as comment composer. */}
      <div className="space-y-1.5">
        <Label htmlFor="bill-curator-note" className="text-sm">
          Curator note <span className="text-muted-foreground">(optional, markdown)</span>
        </Label>
        <MarkdownEditor
          id="bill-curator-note"
          value={note}
          onChange={setNote}
          rows={3}
          maxLength={MAX_CURATOR_NOTE_LENGTH}
          placeholder="Why this bill? 1–2 sentences from the curator."
          testId="featured-admin-bill-curator-note"
          ariaLabel="Curator note for Featured Bill"
          disabled={setMutation.isPending}
        />
      </div>

      <div className="flex justify-end">
        <Button
          type="button"
          onClick={handleSave}
          disabled={!picked || setMutation.isPending || note.length > MAX_CURATOR_NOTE_LENGTH}
          data-testid="featured-admin-save-bill"
        >
          {setMutation.isPending ? (
            <>
              <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
              Saving...
            </>
          ) : (
            'Save Featured Bill'
          )}
        </Button>
      </div>
    </div>
  )
}

// ──────────────────────────────────────────────
// Set-new form — collection (uses GET /collections?search=)
// ──────────────────────────────────────────────

interface PickedCollection {
  id: number
  title: string
  slug: string
  cover_image_url?: string | null
}

function SetCollectionForm() {
  const [query, setQuery] = useState('')
  const [picked, setPicked] = useState<PickedCollection | null>(null)
  const [note, setNote] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)

  const setMutation = useSetFeaturedSlot()
  const { data, isFetching, isError } = useCollectionSearch(query, !picked)

  const handlePick = useCallback((c: CollectionSearchItem) => {
    setPicked({
      id: c.id,
      title: c.title,
      slug: c.slug,
      cover_image_url: c.cover_image_url,
    })
    setQuery('')
  }, [])

  const handleSave = useCallback(() => {
    if (!picked) return
    setError(null)
    setSuccessMessage(null)
    setMutation.mutate(
      {
        slot_type: 'collection',
        entity_id: picked.id,
        curator_note: note.trim() ? note.trim() : null,
      },
      {
        onSuccess: () => {
          setSuccessMessage(`Featured Collection set to "${picked.title}"`)
          setPicked(null)
          setNote('')
        },
        onError: (err) => {
          setError(
            err instanceof Error ? err.message : 'Failed to set Featured Collection.'
          )
        },
      }
    )
  }, [picked, note, setMutation])

  return (
    <div className="space-y-3" data-testid="featured-admin-set-collection-form">
      <h3 className="text-sm font-semibold flex items-center gap-1.5">
        <Sparkles className="h-3.5 w-3.5" />
        Set new Featured Collection
      </h3>

      {successMessage && (
        <StatusBanner
          variant="success"
          dismissAfterMs={4000}
          onDismiss={() => setSuccessMessage(null)}
          testId="featured-admin-set-collection-success"
        >
          <p className="text-sm">{successMessage}</p>
        </StatusBanner>
      )}
      {error && (
        <InlineErrorBanner testId="featured-admin-set-collection-error">
          {error}
        </InlineErrorBanner>
      )}

      {!picked ? (
        <>
          <div className="relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              type="text"
              placeholder="Search public collections by title..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className="pl-9"
              data-testid="featured-admin-collection-search-input"
              aria-label="Search collections"
            />
          </div>

          {query.trim().length >= 2 && (
            <div className="max-h-64 overflow-y-auto rounded-md border border-border/40 bg-card">
              {isFetching ? (
                <div className="flex items-center justify-center py-4">
                  <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                </div>
              ) : isError ? (
                <div className="p-3">
                  <InlineErrorBanner>
                    Could not search collections. Try again in a moment.
                  </InlineErrorBanner>
                </div>
              ) : !data || data.collections.length === 0 ? (
                <p className="text-sm text-muted-foreground py-3 text-center">
                  No collections match &quot;{query}&quot;
                </p>
              ) : (
                <ul className="divide-y divide-border/30">
                  {data.collections.map((c) => (
                    <li key={c.id}>
                      <button
                        type="button"
                        onClick={() => handlePick(c)}
                        className="w-full text-left p-2 hover:bg-muted/50 flex items-center gap-3"
                        data-testid={`featured-admin-collection-result-${c.id}`}
                      >
                        <Library className="h-4 w-4 text-muted-foreground/60 shrink-0" />
                        <div className="flex-1 min-w-0">
                          <p className="text-sm truncate">{c.title}</p>
                          {c.creator_name && (
                            <p className="text-xs text-muted-foreground truncate">
                              by {c.creator_name}
                            </p>
                          )}
                        </div>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </>
      ) : (
        <div
          className="rounded-md border border-border/60 bg-card p-3 flex items-start gap-3"
          data-testid="featured-admin-collection-selected"
        >
          <Library className="h-5 w-5 text-muted-foreground/70 shrink-0 mt-0.5" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium truncate">{picked.title}</p>
            <Link
              href={`/collections/${picked.slug}`}
              target="_blank"
              rel="noreferrer"
              className="text-[11px] text-muted-foreground hover:underline inline-flex items-center gap-0.5 mt-1"
            >
              <Eye className="h-3 w-3" />
              View on site
            </Link>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0"
            onClick={() => setPicked(null)}
            aria-label="Clear selection"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      )}

      <div className="space-y-1.5">
        <Label htmlFor="collection-curator-note" className="text-sm">
          Curator note <span className="text-muted-foreground">(optional, markdown)</span>
        </Label>
        <MarkdownEditor
          id="collection-curator-note"
          value={note}
          onChange={setNote}
          rows={3}
          maxLength={MAX_CURATOR_NOTE_LENGTH}
          placeholder="Why this collection? 1–2 sentences from the curator."
          testId="featured-admin-collection-curator-note"
          ariaLabel="Curator note for Featured Collection"
          disabled={setMutation.isPending}
        />
      </div>

      <div className="flex justify-end">
        <Button
          type="button"
          onClick={handleSave}
          disabled={!picked || setMutation.isPending || note.length > MAX_CURATOR_NOTE_LENGTH}
          data-testid="featured-admin-save-collection"
        >
          {setMutation.isPending ? (
            <>
              <Loader2 className="h-4 w-4 mr-1.5 animate-spin" />
              Saving...
            </>
          ) : (
            'Save Featured Collection'
          )}
        </Button>
      </div>
    </div>
  )
}

// ──────────────────────────────────────────────
// Panel (one per slot type)
// ──────────────────────────────────────────────

interface FeaturedSlotPanelProps {
  slotType: FeaturedSlotType
  /** Hydrated active referent from /explore/featured, or null. */
  activeBill: ExploreFeaturedBill | null
  activeCollection: ExploreFeaturedCollection | null
  /** Skeleton while the first fetch is in flight. */
  isLoading: boolean
}

function FeaturedSlotPanel({
  slotType,
  activeBill,
  activeCollection,
  isLoading,
}: FeaturedSlotPanelProps) {
  const retireMutation = useRetireFeaturedSlot()
  const [retireError, setRetireError] = useState<string | null>(null)
  const [retireSuccess, setRetireSuccess] = useState<string | null>(null)

  const handleRetire = useCallback(() => {
    setRetireError(null)
    setRetireSuccess(null)
    retireMutation.mutate(slotType, {
      onSuccess: () => {
        setRetireSuccess(`${FEATURED_SLOT_LABEL[slotType]} retired.`)
      },
      onError: (err) => {
        setRetireError(
          err instanceof Error
            ? err.message
            : `Failed to retire ${FEATURED_SLOT_LABEL[slotType]}.`
        )
      },
    })
  }, [slotType, retireMutation])

  const hasActive = slotType === 'bill' ? !!activeBill : !!activeCollection

  return (
    <section
      className="rounded-xl border border-border/60 bg-card/40 p-5 space-y-4"
      data-testid={`featured-admin-panel-${slotType}`}
      aria-label={`${FEATURED_SLOT_LABEL[slotType]} controls`}
    >
      <div>
        <h2 className="text-lg font-semibold tracking-tight">
          {FEATURED_SLOT_LABEL[slotType]}
        </h2>
        <p className="text-xs text-muted-foreground mt-0.5">
          Pick the {slotType === 'bill' ? 'show' : 'collection'} that appears in the
          {' '}Featured slot on /explore. Stays visible until you retire or
          replace it.
        </p>
      </div>

      {retireSuccess && (
        <StatusBanner
          variant="success"
          dismissAfterMs={4000}
          onDismiss={() => setRetireSuccess(null)}
          testId={`featured-admin-retire-${slotType}-success`}
        >
          <p className="text-sm">{retireSuccess}</p>
        </StatusBanner>
      )}
      {retireError && (
        <InlineErrorBanner testId={`featured-admin-retire-${slotType}-error`}>
          {retireError}
        </InlineErrorBanner>
      )}

      {/* Current active card OR empty state OR skeleton */}
      <div className="space-y-2">
        <div className="text-[11px] uppercase tracking-wide text-muted-foreground">
          Current active
        </div>
        {isLoading ? (
          <div
            className="rounded-lg border border-border/40 bg-card p-4 flex items-center justify-center"
            data-testid={`featured-admin-loading-${slotType}`}
          >
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        ) : hasActive && slotType === 'bill' && activeBill ? (
          <ActiveBillCard
            bill={activeBill}
            onRetire={handleRetire}
            isRetiring={retireMutation.isPending}
          />
        ) : hasActive && slotType === 'collection' && activeCollection ? (
          <ActiveCollectionCard
            collection={activeCollection}
            onRetire={handleRetire}
            isRetiring={retireMutation.isPending}
          />
        ) : (
          <NoActivePick slotType={slotType} />
        )}
      </div>

      <div className="border-t border-border/40 pt-4">
        {slotType === 'bill' ? <SetBillForm /> : <SetCollectionForm />}
      </div>
    </section>
  )
}

// ──────────────────────────────────────────────
// Top-level component
// ──────────────────────────────────────────────

export function FeaturedAdmin() {
  const { data: featured, isLoading, isError } = useExploreFeatured()

  const activeBill = useMemo(() => featured?.bill ?? null, [featured])
  const activeCollection = useMemo(
    () => featured?.collection ?? null,
    [featured]
  )

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-6xl space-y-6">
        <header>
          <div className="flex items-center gap-2 mb-1">
            <Sparkles className="h-5 w-5 text-primary" />
            <h1 className="text-2xl font-bold tracking-tight">Featured curation</h1>
          </div>
          <p className="text-sm text-muted-foreground">
            Set the active Featured Bill and Featured Collection that appear
            on the /explore landing. No cadence requirement — pick whenever,
            stays live until retired or replaced.
          </p>
        </header>

        {isError && (
          <InlineErrorBanner
            variant="queryFallback"
            testId="featured-admin-load-error"
          >
            Could not load the current featured picks. The admin endpoints may
            be unreachable — try refreshing in a moment.
          </InlineErrorBanner>
        )}

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
          <FeaturedSlotPanel
            slotType="bill"
            activeBill={activeBill}
            activeCollection={null}
            isLoading={isLoading}
          />
          <FeaturedSlotPanel
            slotType="collection"
            activeBill={null}
            activeCollection={activeCollection}
            isLoading={isLoading}
          />
        </div>
      </div>
    </div>
  )
}

export default FeaturedAdmin
