'use client'

/**
 * AddItemsPicker — PSY-823
 *
 * Shared component used by:
 *   1. The Create Collection drawer (CollectionList.tsx) — stage items
 *      alongside the title/description fields so a user can land a fully
 *      populated public collection in one drawer interaction.
 *   2. CollectionDetail's AddItemsSection — same picker, same modes, just
 *      against an existing collection's slug.
 *
 * Two input modes for V1:
 *   - "Search" — reuses the existing useEntitySearch hook + result rows.
 *     Clicking [Add] STAGES the row (not commit). Parent commits the
 *     staged list via its own submit affordance.
 *   - "Paste URLs" — textarea accepting canonical PH paths
 *     (`https://psychichomily.com/artists/<slug>` or `/artists/<slug>`).
 *     Lines are parsed client-side and resolved via a single backend
 *     round-trip (useResolveCollectionItems). Plain-text lines fall to
 *     UNRESOLVED for V1 — plain-text auto-match is a follow-up.
 *
 * AI mode (third tab) mounts AICollectionFiller for paste-an-article
 * extraction via Claude Haiku.
 */

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from 'react'
import { useDebounce } from 'use-debounce'
import {
  Plus,
  Search,
  X,
  Check,
  AlertCircle,
  Library,
  Loader2,
  Info,
  GripVertical,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { InlineErrorBanner } from '@/components/shared'
import {
  useEntitySearch,
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE,
  type EntitySearchResult,
} from '@/lib/hooks/common/useEntitySearch'
import { useResolveCollectionItems } from '../hooks'
import { getEntityTypeLabel, type CollectionEntityType } from '../types'
import { cn } from '@/lib/utils'
import { AICollectionFiller } from './AICollectionFiller'
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
  arrayMove,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ENTITY_ICONS } from './collectionDetailShared'

// ──────────────────────────────────────────────
// Types
// ──────────────────────────────────────────────

// Re-exported so existing importers of this symbol from AddItemsPicker keep
// working; `../types` (derived from COLLECTION_ENTITY_TYPES) is the single
// backend-synced source of truth — there is no second hand-written union to
// drift (PSY-961 adversarial-review fix).
export type { CollectionEntityType }

/**
 * A staged item that's queued for bulk-add but not yet committed. The
 * parent reads these via `onStagedItemsChange` and POSTs them to the
 * bulk-add endpoint on submit.
 */
export interface StagedCollectionItem {
  entityType: CollectionEntityType
  entityId: number
  name: string
  subtitle: string | null
}

/**
 * Stable identity for a staged item — used as BOTH the React key and the
 * @dnd-kit sortable id. These MUST agree character-for-character (the
 * SortableContext `items` list vs each row's `useSortable` id) or drag-reorder
 * silently no-ops. Single-source it so the call sites can't drift (PSY-962).
 */
const stagedKey = (s: { entityType: string; entityId: number }): string =>
  `${s.entityType}-${s.entityId}`

/**
 * Pure reorder behind the drag-end handler — exported so the reorder contract
 * (preserve every item, no dupes, correct new order) is unit-testable without
 * driving @dnd-kit. Returns the reordered array, or null for a no-op (no drop
 * target, dropped on itself, or an id not in the list).
 */
export function reorderStagedItems(
  items: StagedCollectionItem[],
  activeId: string,
  overId: string | null
): StagedCollectionItem[] | null {
  if (!overId || activeId === overId) return null
  const oldIndex = items.findIndex((s) => stagedKey(s) === activeId)
  const newIndex = items.findIndex((s) => stagedKey(s) === overId)
  if (oldIndex === -1 || newIndex === -1) return null
  return arrayMove(items, oldIndex, newIndex)
}

interface ExistingItemKey {
  entity_type: string
  entity_id: number
}

interface ParsedPasteLine {
  raw: string
  url: { entityType: CollectionEntityType; slug: string } | null
}

type PreviewStatus = 'matched' | 'unresolved' | 'loading'

interface PreviewRow {
  raw: string
  status: PreviewStatus
  item: StagedCollectionItem | null
}

// ──────────────────────────────────────────────
// URL parsing
// ──────────────────────────────────────────────

const URL_PATH_TO_ENTITY_TYPE: Record<string, CollectionEntityType> = {
  artists: 'artist',
  releases: 'release',
  labels: 'label',
  shows: 'show',
  venues: 'venue',
  festivals: 'festival',
}

const URL_PATH_REGEX =
  /^\/(artists|releases|labels|shows|venues|festivals)\/([^/?#]+)/i

/**
 * Parses one textarea line. Returns the URL components when it matches a
 * canonical PH path (with or without protocol/host), else `url: null` so
 * the line falls to UNRESOLVED in the preview.
 */
export function parsePasteLine(line: string): ParsedPasteLine {
  const trimmed = line.trim()
  if (trimmed.length === 0) return { raw: trimmed, url: null }

  // Allow either fully-qualified URL or bare path. Try to extract the
  // pathname when a host is present.
  let path = trimmed
  if (/^https?:\/\//i.test(trimmed)) {
    try {
      path = new URL(trimmed).pathname
    } catch {
      return { raw: trimmed, url: null }
    }
  }

  // Normalize leading slash so `/artists/foo` and `artists/foo` both match.
  if (!path.startsWith('/')) path = `/${path}`

  const match = path.match(URL_PATH_REGEX)
  if (!match) return { raw: trimmed, url: null }

  const entityType = URL_PATH_TO_ENTITY_TYPE[match[1].toLowerCase()]
  const slug = match[2].toLowerCase()
  return { raw: trimmed, url: { entityType, slug } }
}

// ──────────────────────────────────────────────
// Component
// ──────────────────────────────────────────────

export interface AddItemsPickerProps {
  /**
   * Items already in the target collection. Used to mark search results +
   * paste-preview rows as "already added" so the user doesn't dupe them.
   * Pass an empty array for the Create flow (collection doesn't exist yet).
   */
  existingItems?: ExistingItemKey[]
  /**
   * Controlled staged list. Parent owns the array so it can clear it
   * post-submit (a `useState`-seeded internal copy would ignore the
   * reset). Picker mutates the list by calling onStagedItemsChange with
   * the next value — same shape as a controlled <input>.
   */
  stagedItems: StagedCollectionItem[]
  onStagedItemsChange: (items: StagedCollectionItem[]) => void
}

/** Maximum visible rows in the staged list before scrolling. Tracks the
 *  Figma's 10-row visible window on state 05. */
const STAGED_LIST_MAX_VISIBLE = 10

/** Locked copy (PSY-867 design review, 2026-05-26). The "From text (AI)"
 *  tab accepts any pasted text, not just articles — this explainer sets
 *  the expectation (any text in, best-effort extraction out) and the
 *  honest caveat that the model is fallible. */
const AI_TAB_TOOLTIP_COPY =
  'Paste any text, and the AI will do its best to extract any artists or releases referenced. AI can and will make mistakes.'

/**
 * The ⓘ explainer for the "From text (AI)" tab. Rendered as a SIBLING of
 * the tab trigger (not a child) — the trigger is a `<button>` and nesting
 * another focusable element inside it would be invalid interactive-content
 * nesting. As a sibling it gets its own focus stop, so the tooltip opens on
 * hover AND keyboard focus of just the glyph, while clicking the tab itself
 * still switches modes.
 *
 * The Tooltip composition mirrors the canonical `TransitiveTagTooltip`
 * (TagFacetPanel.tsx). The placement as a non-tab sibling INSIDE the Radix
 * tablist is specific to this tab context: Radix's roving-tabindex only
 * governs `role="tab"` descendants, so the glyph stays an ordinary Tab stop
 * rather than joining the arrow-key tab cycle. Verified manually in-browser
 * (arrow keys still move between the three tabs; the glyph is its own Tab
 * stop) — the unit tests mock `@/components/ui/tabs`, so they do not cover
 * the real-Radix focus path. A future shared `InfoTooltip` extraction (PSY
 * follow-up) would standardize this.
 */
function AiTabInfoTooltip() {
  return (
    <TooltipProvider delayDuration={120}>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label="What can I paste into the AI tab?"
            className="inline-flex items-center rounded-full p-0.5 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-testid="ai-tab-info"
          >
            <Info className="h-3.5 w-3.5" aria-hidden />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs text-xs">
          {AI_TAB_TOOLTIP_COPY}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export function AddItemsPicker({
  existingItems = [],
  stagedItems,
  onStagedItemsChange,
}: AddItemsPickerProps) {
  // Active mode tab — all three modes (search | paste | ai) are live; the
  // AI tab was enabled in PSY-824.
  const [tab, setTab] = useState<'search' | 'paste' | 'ai'>('search')

  // ─── Search mode state ───
  const [searchQuery, setSearchQuery] = useState('')
  const {
    data: searchResults,
    isSearching,
    searchError,
  } = useEntitySearch({
    query: searchQuery,
    enabled: tab === 'search',
  })

  // ─── Paste mode state ───
  const [pasteText, setPasteText] = useState('')
  const [debouncedPaste] = useDebounce(pasteText, 400)
  const resolveMutation = useResolveCollectionItems()
  const [previewRows, setPreviewRows] = useState<PreviewRow[]>([])
  // Stale-response guard: each resolve fire bumps this counter; the
  // onSuccess handler only commits its result when its captured token
  // matches the latest. Protects against out-of-order responses when the
  // user types fast enough that the older request resolves AFTER a newer
  // one — without this, an earlier response would overwrite the newer
  // preview state.
  const resolveGenerationRef = useRef(0)

  // Resolve paste textarea contents whenever the debounced value changes.
  // Only URL lines hit the backend; plain-text lines fall through as
  // UNRESOLVED until the plain-text auto-match follow-up ships.
  useEffect(() => {
    const lines = debouncedPaste
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l.length > 0)
    if (lines.length === 0) {
      setPreviewRows([])
      return
    }

    const parsed = lines.map((l) => parsePasteLine(l))
    const urlEntries = parsed
      .filter((p) => p.url !== null)
      .map((p) => ({ entity_type: p.url!.entityType, slug: p.url!.slug }))

    if (urlEntries.length === 0) {
      // Nothing to resolve; mark all rows as unresolved.
      setPreviewRows(
        parsed.map((p) => ({
          raw: p.raw,
          status: 'unresolved',
          item: null,
        }))
      )
      return
    }

    // Show "loading" rows for the URL lines while the request is in flight,
    // unresolved for the rest, so the UI doesn't flicker on every keystroke.
    setPreviewRows(
      parsed.map((p) => ({
        raw: p.raw,
        status: p.url ? 'loading' : 'unresolved',
        item: null,
      }))
    )

    // Bump the generation; only this fire's onSuccess will commit. Older
    // in-flight requests' onSuccess will see a stale token and bail.
    resolveGenerationRef.current += 1
    const myGeneration = resolveGenerationRef.current

    resolveMutation.mutate(urlEntries, {
      onSuccess: (data) => {
        if (resolveGenerationRef.current !== myGeneration) return
        const resolvedBySlug = new Map<string, StagedCollectionItem>()
        for (const r of data.resolved) {
          const key = `${r.entity_type}:${r.slug}`
          resolvedBySlug.set(key, {
            entityType: r.entity_type as CollectionEntityType,
            entityId: r.entity_id,
            name: r.name,
            subtitle: r.subtitle ?? null,
          })
        }

        setPreviewRows(
          parsed.map((p) => {
            if (!p.url) {
              return { raw: p.raw, status: 'unresolved', item: null }
            }
            const key = `${p.url.entityType}:${p.url.slug}`
            const match = resolvedBySlug.get(key)
            if (!match) {
              return { raw: p.raw, status: 'unresolved', item: null }
            }
            return {
              raw: p.raw,
              status: 'matched',
              item: match,
            }
          })
        )
      },
      onError: () => {
        if (resolveGenerationRef.current !== myGeneration) return
        // Network/server error — mark URL rows as unresolved so the user
        // can retry. (We could surface an explicit error banner, but
        // unresolved + the InlineErrorBanner below the textarea is enough
        // signal for V1.)
        setPreviewRows(
          parsed.map((p) => ({
            raw: p.raw,
            status: 'unresolved',
            item: null,
          }))
        )
      },
    })
    // We intentionally only re-resolve when the debounced paste text
    // changes. The alreadyStaged flag is computed at render time off the
    // current stagedItems prop — see PastePreviewRow — so existingItems
    // / stagedItems changes don't need to re-fire the resolver.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedPaste])

  // Flattened search results for the active query. Mirrors the existing
  // AddItemsSection shape so users get a familiar list.
  const searchRows: EntitySearchResult[] = useMemo(() => {
    if (!searchResults) return []
    return [
      ...searchResults.artists,
      ...searchResults.shows,
      ...searchResults.venues,
      ...searchResults.releases,
      ...searchResults.labels,
      ...searchResults.festivals,
    ]
  }, [searchResults])

  // ─── Staging helpers ───
  // Single-call updates only — multiple successive onStagedItemsChange
  // calls in the same render closure would race against React's
  // setState batching (each call reads the same stale stagedItems prop,
  // last write wins). Both helpers compute the next array in one pass
  // and call the parent callback exactly once.
  const stageBatch = (items: StagedCollectionItem[]) => {
    const fresh = items.filter(
      (incoming) =>
        !stagedItems.some(
          (s) =>
            s.entityType === incoming.entityType &&
            s.entityId === incoming.entityId
        )
    )
    if (fresh.length === 0) return
    onStagedItemsChange([...stagedItems, ...fresh])
  }

  const stageItem = (item: StagedCollectionItem) => stageBatch([item])

  const unstageItem = (entityType: string, entityId: number) => {
    onStagedItemsChange(
      stagedItems.filter(
        (s) => !(s.entityType === entityType && s.entityId === entityId)
      )
    )
  }

  // ─── Reorder (PSY-962) ───
  // Drag-to-reorder the staged list; the overview strip mirrors this order.
  // Sensors mirror the collections drag-drop primitive (PSY-348): pointer 8px,
  // touch long-press, and KeyboardSensor for keyboard reorder (focus the drag
  // handle → Space to lift → arrow keys to move → Space to drop). Unlike
  // CollectionItemCard (the heavier detail-page surface), this transient
  // staging list intentionally omits the separate up/down arrow BUTTONS — the
  // locked PSY-962 design is a drag-handle-only row; all three input modalities
  // (pointer/touch/keyboard) can still reorder via the sensors above.
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 200, tolerance: 8 },
    }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  )
  const stagedIds = useMemo(
    () => stagedItems.map(stagedKey),
    [stagedItems]
  )
  const handleReorder = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event
      const next = reorderStagedItems(
        stagedItems,
        String(active.id),
        over ? String(over.id) : null
      )
      if (next) onStagedItemsChange(next)
    },
    [stagedItems, onStagedItemsChange]
  )

  // ─── Render ───

  const stagedCount = stagedItems.length

  return (
    <div className="space-y-3" data-testid="add-items-picker">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold">
          {stagedCount > 0 ? `Items (${stagedCount})` : 'Add items'}
        </h3>
      </div>

      <Tabs
        value={tab}
        onValueChange={(v) => setTab(v as typeof tab)}
        className="w-full"
      >
        <TabsList className="w-full justify-start">
          <TabsTrigger value="search">Search</TabsTrigger>
          <TabsTrigger value="paste">Paste URLs</TabsTrigger>
          {/* The AI tab + its ⓘ explainer share one flex slot so they read
              as "From text (AI) ⓘ". The info trigger is a sibling button,
              not a child of the tab trigger — nesting a focusable element
              inside the trigger <button> would be invalid. See
              AiTabInfoTooltip. */}
          <div className="inline-flex min-w-0 flex-1 items-center justify-center gap-1">
            <TabsTrigger value="ai" className="flex-none">
              From text (AI)
            </TabsTrigger>
            <AiTabInfoTooltip />
          </div>
        </TabsList>

        {tab === 'search' && (
          <SearchModePane
            query={searchQuery}
            onQueryChange={setSearchQuery}
            isSearching={isSearching}
            searchError={searchError}
            rows={searchRows}
            existingItems={existingItems}
            stagedItems={stagedItems}
            onStage={stageItem}
          />
        )}

        {tab === 'paste' && (
          <PasteModePane
            text={pasteText}
            onTextChange={setPasteText}
            previewRows={previewRows}
            existingItems={existingItems}
            stagedItems={stagedItems}
            onStage={stageItem}
            onStageBatch={stageBatch}
          />
        )}

        {tab === 'ai' && (
          <AICollectionFiller
            onStageItems={stageBatch}
            alreadyStaged={(entityType, entityId) =>
              isAlreadyStaged(
                { entityType, entityId, name: '', subtitle: null },
                existingItems,
                stagedItems
              )
            }
          />
        )}
      </Tabs>

      {/* Staged list (PSY-962: overview strip + drag-reorderable detail list) */}
      {stagedCount > 0 && (
        <div className="mt-3 border-t border-border/50 pt-3 space-y-2">
          <StagedOverviewStrip items={stagedItems} />
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleReorder}
          >
            <SortableContext
              items={stagedIds}
              strategy={verticalListSortingStrategy}
            >
              <div
                className={cn(
                  'space-y-0.5',
                  stagedCount > STAGED_LIST_MAX_VISIBLE &&
                    'max-h-[420px] overflow-y-auto'
                )}
                data-testid="add-items-picker-staged-list"
              >
                {stagedItems.map((item, index) => (
                  <StagedRow
                    key={stagedKey(item)}
                    index={index}
                    item={item}
                    canReorder={stagedCount > 1}
                    onRemove={() => unstageItem(item.entityType, item.entityId)}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        </div>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Subcomponents
// ──────────────────────────────────────────────

function SearchModePane({
  query,
  onQueryChange,
  isSearching,
  searchError,
  rows,
  existingItems,
  stagedItems,
  onStage,
}: {
  query: string
  onQueryChange: (v: string) => void
  isSearching: boolean
  searchError: boolean
  rows: EntitySearchResult[]
  existingItems: ExistingItemKey[]
  stagedItems: StagedCollectionItem[]
  onStage: (item: StagedCollectionItem) => void
}) {
  return (
    <div className="mt-3 space-y-3">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search artists, shows, venues, releases, labels, festivals..."
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          className="pl-9"
          autoFocus
          data-testid="add-items-picker-search-input"
        />
      </div>

      {query.trim().length === 0 ? (
        <p className="text-sm text-muted-foreground py-3 text-center">
          — search artists, shows, venues, releases, labels, festivals —
        </p>
      ) : query.trim().length < 2 ? (
        <p className="text-sm text-muted-foreground py-3 text-center">
          Keep typing to search…
        </p>
      ) : isSearching ? (
        <div className="flex items-center justify-center py-4">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : searchError ? (
        <InlineErrorBanner testId="add-items-picker-search-error-banner">
          {ENTITY_SEARCH_UNAVAILABLE_MESSAGE}
        </InlineErrorBanner>
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground py-3 text-center">
          No results found for &quot;{query}&quot;
        </p>
      ) : (
        <div className="max-h-64 overflow-y-auto space-y-1">
          {rows.map((row) => {
            const alreadyAdded = isAlreadyStaged(
              {
                entityType: row.entityType as CollectionEntityType,
                entityId: row.id,
                name: row.name,
                subtitle: row.subtitle,
              },
              existingItems,
              stagedItems
            )
            return (
              <SearchResultRow
                key={`${row.entityType}-${row.id}`}
                row={row}
                alreadyAdded={alreadyAdded}
                onAdd={() =>
                  onStage({
                    entityType: row.entityType as CollectionEntityType,
                    entityId: row.id,
                    name: row.name,
                    subtitle: row.subtitle,
                  })
                }
              />
            )
          })}
        </div>
      )}
    </div>
  )
}

function SearchResultRow({
  row,
  alreadyAdded,
  onAdd,
}: {
  row: EntitySearchResult
  alreadyAdded: boolean
  onAdd: () => void
}) {
  return (
    <div
      className="flex items-center gap-3 rounded-md p-2 hover:bg-muted/50"
      data-testid="add-items-picker-search-row"
    >
      <div className="h-7 w-7 shrink-0 rounded bg-muted/50 flex items-center justify-center">
        <Library className="h-3.5 w-3.5 text-muted-foreground/60" />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium truncate">{row.name}</span>
          <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
            {getEntityTypeLabel(row.entityType)}
          </Badge>
        </div>
        {row.subtitle && (
          <p className="text-xs text-muted-foreground truncate">
            {row.subtitle}
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
          onClick={onAdd}
        >
          <Plus className="h-3.5 w-3.5 mr-1" />
          Add
        </Button>
      )}
    </div>
  )
}

function PasteModePane({
  text,
  onTextChange,
  previewRows,
  existingItems,
  stagedItems,
  onStage,
  onStageBatch,
}: {
  text: string
  onTextChange: (v: string) => void
  previewRows: PreviewRow[]
  existingItems: ExistingItemKey[]
  stagedItems: StagedCollectionItem[]
  onStage: (item: StagedCollectionItem) => void
  onStageBatch: (items: StagedCollectionItem[]) => void
}) {
  const matchedCount = previewRows.filter((r) => r.status === 'matched').length
  const unresolvedCount = previewRows.filter((r) => r.status === 'unresolved').length
  const loadingCount = previewRows.filter((r) => r.status === 'loading').length

  // "Add all" affordance: stages every matched row at once. Bypasses the
  // per-row [Add] button so the canon-list use case (200 URLs pasted) is
  // one click instead of N. Routes through onStageBatch so the parent
  // computes the next staged array in a single setState — calling onStage
  // per row would race React's setState batching (each call would read
  // the same stale stagedItems prop and the last write would win).
  const addAll = () => {
    const toAdd: StagedCollectionItem[] = []
    for (const row of previewRows) {
      if (
        row.status === 'matched' &&
        row.item &&
        !isAlreadyStaged(row.item, existingItems, stagedItems)
      ) {
        toAdd.push(row.item)
      }
    }
    if (toAdd.length === 0) return
    onStageBatch(toAdd)
  }
  const addAllEligible = previewRows.filter(
    (r) =>
      r.status === 'matched' &&
      r.item &&
      !isAlreadyStaged(r.item, existingItems, stagedItems)
  ).length

  return (
    <div className="mt-3 space-y-3">
      <textarea
        value={text}
        onChange={(e) => onTextChange(e.target.value)}
        placeholder={
          'One item per line:\n' +
          'https://psychichomily.com/artists/kendrick-lamar\n' +
          '/releases/to-pimp-a-butterfly\n' +
          '/artists/frank-ocean'
        }
        rows={6}
        className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm font-mono shadow-xs focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        data-testid="add-items-picker-paste-textarea"
      />

      {previewRows.length > 0 && (
        <div
          className="flex flex-wrap items-center justify-between gap-2"
          data-testid="add-items-picker-paste-summary"
        >
          <p className="text-xs text-muted-foreground">
            {matchedCount > 0 && `${matchedCount} matched`}
            {matchedCount > 0 && (loadingCount > 0 || unresolvedCount > 0) && ' · '}
            {loadingCount > 0 && `${loadingCount} resolving`}
            {loadingCount > 0 && unresolvedCount > 0 && ' · '}
            {unresolvedCount > 0 && `${unresolvedCount} unresolved`}
          </p>
          {addAllEligible > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={addAll}
              data-testid="add-items-picker-paste-add-all"
            >
              <Plus className="h-3.5 w-3.5 mr-1" />
              Add all {addAllEligible}
            </Button>
          )}
        </div>
      )}

      {previewRows.length > 0 && (
        <div className="max-h-64 overflow-y-auto space-y-1">
          {previewRows.map((row, index) => (
            <PastePreviewRow
              key={`${index}-${row.raw}`}
              row={row}
              alreadyStaged={
                row.item
                  ? isAlreadyStaged(row.item, existingItems, stagedItems)
                  : false
              }
              onAdd={() => row.item && onStage(row.item)}
            />
          ))}
        </div>
      )}

      {previewRows.length > 0 && unresolvedCount > 0 && (
        <p className="text-xs text-muted-foreground">
          Unresolved lines must be canonical PH paths like{' '}
          <code className="px-1 rounded bg-muted">/artists/&lt;slug&gt;</code>.
          For an article URL or pasted prose, switch to the AI tab. Free-text
          auto-match in this paste-URL field is a follow-up.
        </p>
      )}
    </div>
  )
}

function PastePreviewRow({
  row,
  alreadyStaged,
  onAdd,
}: {
  row: PreviewRow
  alreadyStaged: boolean
  onAdd: () => void
}) {
  return (
    <div
      className="flex items-center gap-3 rounded-md p-2 hover:bg-muted/50"
      data-testid="add-items-picker-paste-row"
    >
      <div className="h-7 w-7 shrink-0 rounded bg-muted/50 flex items-center justify-center">
        <Library className="h-3.5 w-3.5 text-muted-foreground/60" />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium truncate">
            {row.item?.name ?? row.raw}
          </span>
          {row.item && (
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
              {getEntityTypeLabel(row.item.entityType)}
            </Badge>
          )}
        </div>
        {row.item?.subtitle && (
          <p className="text-xs text-muted-foreground truncate">
            {row.item.subtitle}
          </p>
        )}
        {!row.item && (
          <p className="text-xs text-muted-foreground truncate font-mono">
            {row.raw}
          </p>
        )}
      </div>

      {row.status === 'loading' && (
        <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-muted-foreground" />
      )}
      {row.status === 'matched' && (
        <>
          <Badge
            variant="secondary"
            className="text-[10px] px-1.5 py-0 shrink-0 bg-success text-success-foreground motion-safe:animate-in motion-safe:fade-in"
          >
            <Check className="h-3 w-3 mr-0.5" />
            MATCH
          </Badge>
          {alreadyStaged ? (
            <Badge variant="secondary" className="text-xs shrink-0">
              Added
            </Badge>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 shrink-0"
              onClick={onAdd}
            >
              <Plus className="h-3.5 w-3.5 mr-1" />
              Add
            </Button>
          )}
        </>
      )}
      {row.status === 'unresolved' && (
        <Badge
          variant="secondary"
          className="text-[10px] px-1.5 py-0 shrink-0 bg-destructive/10 text-destructive motion-safe:animate-in motion-safe:fade-in"
        >
          <AlertCircle className="h-3 w-3 mr-0.5" />
          NO MATCH
        </Badge>
      )}
    </div>
  )
}

/**
 * PSY-962: at-a-glance overview strip above the staged list — item count + a
 * capped row of entity-type icon chips. The numbered list below stays the
 * detail + drag-reorder surface; this strip mirrors its order. Icon-only +
 * monochrome by design (color is reserved for the AI status chips). Enter
 * animation is gated on `motion-safe` so it honors prefers-reduced-motion.
 */
/** Overview-strip preview cap — render at most this many entity-type icon
 *  chips, then a "+N" overflow chip. ~2 wrapped rows of 28px chips at the
 *  drawer's min width; the numbered list below stays the complete view. */
const STRIP_PREVIEW_CAP = 24
function StagedOverviewStrip({ items }: { items: StagedCollectionItem[] }) {
  const shown = items.slice(0, STRIP_PREVIEW_CAP)
  const overflow = items.length - shown.length
  return (
    <div className="space-y-1.5" data-testid="add-items-picker-overview-strip">
      <div className="flex items-center justify-between">
        <span className="text-xs font-mono text-muted-foreground">
          {items.length} {items.length === 1 ? 'item' : 'items'}
        </span>
        {items.length > 1 && (
          <span className="text-[10px] font-mono text-muted-foreground">
            ⇅ drag to reorder
          </span>
        )}
      </div>
      {/* Decorative: icons duplicate the numbered list below, which is the
          accessible source of truth (full names + type badges). */}
      <div className="flex flex-wrap gap-1.5" aria-hidden="true">
        {shown.map((item) => {
          const Icon = ENTITY_ICONS[item.entityType] ?? Library
          return (
            <span
              key={stagedKey(item)}
              className="flex h-7 w-7 items-center justify-center rounded border border-border bg-secondary text-secondary-foreground motion-safe:animate-in motion-safe:fade-in"
              title={`${item.name} — ${getEntityTypeLabel(item.entityType)}`}
            >
              <Icon className="h-3.5 w-3.5" aria-hidden="true" />
            </span>
          )
        })}
        {overflow > 0 && (
          <span className="flex h-7 items-center rounded border border-border bg-muted px-2 text-[10px] font-mono text-muted-foreground">
            +{overflow}
          </span>
        )}
      </div>
    </div>
  )
}

function StagedRow({
  index,
  item,
  canReorder,
  onRemove,
}: {
  index: number
  item: StagedCollectionItem
  canReorder: boolean
  onRemove: () => void
}) {
  // useSortable returns no-op refs/listeners when reorder is disabled (single
  // item), keeping hook order stable across renders.
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id: stagedKey(item),
    disabled: !canReorder,
  })
  const sortableStyle: CSSProperties = canReorder
    ? {
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.6 : undefined,
      }
    : {}
  return (
    <div
      ref={canReorder ? setNodeRef : undefined}
      style={sortableStyle}
      className="flex items-center gap-2 rounded-md px-2 py-1.5 border border-border/40 bg-popover motion-safe:animate-in motion-safe:fade-in"
      data-testid="add-items-picker-staged-row"
    >
      {canReorder && (
        <button
          type="button"
          {...attributes}
          {...listeners}
          className="touch-none cursor-grab active:cursor-grabbing flex h-6 w-4 shrink-0 items-center justify-center text-muted-foreground hover:text-foreground rounded focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label={`Drag to reorder ${item.name}. Use space to lift, arrow keys to move.`}
          data-testid="staged-row-drag-handle"
        >
          <GripVertical className="h-3.5 w-3.5" />
        </button>
      )}
      <span className="text-xs font-mono text-muted-foreground w-6 shrink-0 text-right">
        {String(index + 1).padStart(2, '0')}
      </span>
      <span className="text-sm flex-1 min-w-0 truncate">
        {item.name}
        {item.subtitle && (
          <span className="text-muted-foreground"> — {item.subtitle}</span>
        )}
      </span>
      <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
        {getEntityTypeLabel(item.entityType)}
      </Badge>
      <Button
        variant="ghost"
        size="sm"
        className="h-7 w-7 p-0 shrink-0"
        onClick={onRemove}
        aria-label={`Remove ${item.name}`}
      >
        <X className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

function isAlreadyStaged(
  candidate: StagedCollectionItem,
  existing: ExistingItemKey[],
  staged: StagedCollectionItem[]
): boolean {
  for (const e of existing) {
    if (e.entity_type === candidate.entityType && e.entity_id === candidate.entityId) {
      return true
    }
  }
  for (const s of staged) {
    if (s.entityType === candidate.entityType && s.entityId === candidate.entityId) {
      return true
    }
  }
  return false
}
