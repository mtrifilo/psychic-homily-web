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
 *     (`https://psychichomily.com/artists/<slug>` or `/artists/<slug>`)
 *     AND free plain-text lines (PSY-845). URL lines are parsed client-side
 *     and resolved via a single backend round-trip (useResolveCollectionItems).
 *     Plain-text lines auto-search the entity endpoints (bounded to 5 in
 *     flight): exactly one result ⇒ MATCH (stageable); 2+ ⇒ AMBIGUOUS with an
 *     inline [Pick] dropdown (≤5); zero ⇒ queue-for-review (POSTs an
 *     entity_request for an admin to approve). See usePastePreview.
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
  AlertTriangle,
  Library,
  Loader2,
  GripVertical,
  Inbox,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { InfoTooltip } from '@/components/shared/InfoTooltip'
import { InlineErrorBanner } from '@/components/shared'
import {
  useEntitySearch,
  fetchEntitySearch,
  flattenEntitySearchResults,
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE,
  type EntitySearchResult,
} from '@/lib/hooks/common/useEntitySearch'
import { useResolveCollectionItems } from '../hooks'
import { getEntityTypeLabel, type CollectionEntityType } from '../types'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
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

/**
 * Preview-row lifecycle states (PSY-845).
 *
 * URL lines (canonical PH paths) resolve through the batch
 * `useResolveCollectionItems` round-trip and only ever land on
 * `loading` → `matched` | `unresolved`.
 *
 * Plain-text lines auto-search the entity endpoints per line and land on:
 *   - `searching`     — search in flight
 *   - `matched`       — exactly ONE result across all types ⇒ stageable
 *   - `ambiguous`     — 2+ candidates ⇒ inline [Pick] dropdown (≤5)
 *   - `queuing`       — zero results ⇒ entity_request POST in flight
 *   - `queued`        — zero results ⇒ request filed for admin review
 *   - `queue_failed`  — zero results ⇒ the request POST errored (retryable)
 */
type PreviewStatus =
  | 'matched'
  | 'unresolved'
  | 'loading'
  | 'searching'
  | 'ambiguous'
  | 'queuing'
  | 'queued'
  | 'queue_failed'

/** A candidate offered for an AMBIGUOUS plain-text line's [Pick] dropdown. */
interface PreviewCandidate {
  entityType: CollectionEntityType
  entityId: number
  name: string
  subtitle: string | null
}

interface PreviewRow {
  raw: string
  status: PreviewStatus
  /** The resolved/picked entity for `matched` rows; null otherwise. */
  item: StagedCollectionItem | null
  /**
   * AMBIGUOUS candidates (≤5) when `status === 'ambiguous'`. The user picks
   * one via the inline dropdown, which promotes the row to `matched`.
   */
  candidates?: PreviewCandidate[]
}

/** Max candidates surfaced in an AMBIGUOUS line's [Pick] dropdown. */
const MAX_PICK_CANDIDATES = 5

/** Bounded in-flight plain-text searches — don't hammer the backend on a
 *  200-line paste. (PSY-845 locked decision.) */
const PLAINTEXT_SEARCH_CONCURRENCY = 5

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
// Queue-for-review (PSY-845 + PSY-997)
// ──────────────────────────────────────────────

/**
 * LOCAL queue-create call (PSY-845). Posts an `entity_request` to PSY-997's
 * `POST /entity-requests` so a plain-text line with ZERO matches becomes an
 * admin-reviewable artist-creation request rather than being silently dropped.
 *
 * Deliberately LOCAL (not a shared exported hook): PSY-853 posts to the same
 * endpoint from AICollectionFiller in parallel. Keeping each consumer's
 * queue-create local avoids a cross-PR file collision; a future ticket dedups
 * the two into one shared hook. The small duplication is intentional (see
 * coordination note on PSY-845).
 *
 * A plain async function, NOT a `useMutation` hook: a single paste can queue
 * several zero-result lines CONCURRENTLY (the bounded worker pool), and one
 * shared `useMutation` observer only tracks the latest in-flight mutation —
 * rapid successive `.mutate()` calls drop earlier per-call callbacks, so only
 * the last line would end up queued. Per-call `apiRequest` has no such shared
 * state; usePastePreview owns the per-row status, so the hook's
 * data/isPending/error were unused anyway.
 *
 * Entity type is `artist`: a bare plain-text line in a music collection picker
 * is overwhelmingly an artist name, and `artist` is the only entity_request
 * payload whose sole required field is `name` (releases need a title, shows an
 * event_date, venues city+state, etc.) — so the line text is sufficient to
 * file a well-formed request. The admin reviewing the queue retypes /
 * reclassifies if it was actually a release or venue.
 */
function queueEntityRequest(name: string): Promise<unknown> {
  return apiRequest(API_ENDPOINTS.COLLECTIONS.ENTITY_REQUESTS, {
    method: 'POST',
    body: JSON.stringify({
      entity_type: 'artist',
      payload: { name },
      source_context: 'paste_mode',
    }),
  })
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
 * Delegates to the shared `InfoTooltip` primitive (PSY-969). The placement as
 * a non-tab sibling INSIDE the Radix tablist is specific to this tab context:
 * Radix's roving-tabindex only governs `role="tab"` descendants, so the glyph
 * stays an ordinary Tab stop rather than joining the arrow-key tab cycle.
 * Verified manually in-browser (arrow keys still move between the three tabs;
 * the glyph is its own Tab stop) — the unit tests mock `@/components/ui/tabs`,
 * so they do not cover the real-Radix focus path.
 */
function AiTabInfoTooltip() {
  return (
    <InfoTooltip
      copy={AI_TAB_TOOLTIP_COPY}
      label="What can I paste into the AI tab?"
      testId="ai-tab-info"
    />
  )
}

// ──────────────────────────────────────────────
// Paste-mode resolution hook (PSY-823 URL resolve + PSY-845 plain-text)
// ──────────────────────────────────────────────

/**
 * Resolve a `name` from a plain-text search hit into the preview-row item
 * shape. Mirrors the resolved-item mapping the URL path uses.
 */
function toPreviewItem(r: EntitySearchResult): StagedCollectionItem {
  return {
    entityType: r.entityType as CollectionEntityType,
    entityId: r.id,
    name: r.name,
    subtitle: r.subtitle ?? null,
  }
}

/**
 * Run `task` over `items` with at most `limit` in flight at once (PSY-845).
 * A bounded worker pool: `limit` workers each pull the next index off a
 * shared cursor until the list is drained. Keeps a 200-line paste from
 * firing 200 simultaneous searches at the backend.
 */
async function runWithConcurrency<T>(
  items: T[],
  limit: number,
  task: (item: T, index: number) => Promise<void>
): Promise<void> {
  let cursor = 0
  const worker = async () => {
    while (cursor < items.length) {
      const index = cursor++
      await task(items[index], index)
    }
  }
  const workers = Array.from(
    { length: Math.min(limit, items.length) },
    () => worker()
  )
  await Promise.all(workers)
}

/**
 * Owns the Paste-mode preview lifecycle: parses the textarea, batch-resolves
 * canonical PH URL lines (PSY-823), and auto-searches plain-text lines with
 * bounded parallelism (PSY-845). Returns the ordered preview rows plus the
 * row-level actions (pick a candidate for an AMBIGUOUS line; retry a failed
 * queue POST).
 *
 * Isolated as a hook so the component body stays declarative and the volatile
 * concurrency / dual-resolution machinery lives behind one narrow interface
 * (Code Complete: information hiding + isolate-likely-to-change).
 *
 * Stale-response guard: each debounced change bumps `generationRef`; every
 * async continuation (URL resolve onSuccess, per-line search, queue POST)
 * checks its captured generation before committing, so an older paste's
 * in-flight responses can never overwrite a newer paste's preview.
 */
function usePastePreview(pasteText: string): {
  previewRows: PreviewRow[]
  pickCandidate: (rowIndex: number, candidate: PreviewCandidate) => void
  retryQueue: (rowIndex: number) => void
} {
  const [debouncedPaste] = useDebounce(pasteText, 400)
  const resolveMutation = useResolveCollectionItems()
  const [previewRows, setPreviewRows] = useState<PreviewRow[]>([])
  const generationRef = useRef(0)

  // Commit a single row's update IFF the captured generation is still current.
  // Used by every async continuation so a stale paste can't clobber the list.
  const updateRow = useCallback(
    (generation: number, index: number, next: Partial<PreviewRow>) => {
      setPreviewRows((rows) => {
        if (generationRef.current !== generation) return rows
        if (index < 0 || index >= rows.length) return rows
        const copy = rows.slice()
        copy[index] = { ...copy[index], ...next }
        return copy
      })
    },
    []
  )

  // File a queue-for-review request for a zero-result plain-text line.
  // Extracted so the initial pass AND retryQueue share one code path. Each
  // call is an independent POST (see queueEntityRequest) so concurrent
  // zero-result lines each get their own request + row update. Returns the
  // settle promise so the bounded worker pool can AWAIT it — that keeps a
  // 200-junk-line paste from firing 200 simultaneous POSTs (the same
  // "don't hammer the backend" bound the search side gets). retryQueue, a
  // single user-triggered call, ignores the return (fire-and-forget).
  const fileQueueRequest = useCallback(
    (generation: number, index: number, raw: string): Promise<void> => {
      updateRow(generation, index, { status: 'queuing' })
      return queueEntityRequest(raw).then(
        () => updateRow(generation, index, { status: 'queued' }),
        () => updateRow(generation, index, { status: 'queue_failed' })
      )
    },
    [updateRow]
  )

  useEffect(() => {
    const lines = debouncedPaste
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l.length > 0)
    if (lines.length === 0) {
      generationRef.current += 1 // invalidate any in-flight continuations
      setPreviewRows([])
      return
    }

    generationRef.current += 1
    const generation = generationRef.current

    const parsed = lines.map((l) => parsePasteLine(l))

    // Initial states: URL lines → loading (batch resolve); plain-text → searching.
    setPreviewRows(
      parsed.map((p) => ({
        raw: p.raw,
        status: p.url ? 'loading' : 'searching',
        item: null,
      }))
    )

    // ── URL lines: one batch round-trip (PSY-823 path, unchanged semantics) ──
    const urlEntries = parsed
      .map((p, i) => ({ parsed: p, index: i }))
      .filter((e) => e.parsed.url !== null)

    if (urlEntries.length > 0) {
      resolveMutation.mutate(
        urlEntries.map((e) => ({
          entity_type: e.parsed.url!.entityType,
          slug: e.parsed.url!.slug,
        })),
        {
          onSuccess: (data) => {
            const resolvedBySlug = new Map<string, StagedCollectionItem>()
            for (const r of data.resolved) {
              resolvedBySlug.set(`${r.entity_type}:${r.slug}`, {
                entityType: r.entity_type as CollectionEntityType,
                entityId: r.entity_id,
                name: r.name,
                subtitle: r.subtitle ?? null,
              })
            }
            for (const e of urlEntries) {
              const key = `${e.parsed.url!.entityType}:${e.parsed.url!.slug}`
              const match = resolvedBySlug.get(key)
              updateRow(
                generation,
                e.index,
                match
                  ? { status: 'matched', item: match }
                  : { status: 'unresolved', item: null }
              )
            }
          },
          onError: () => {
            // Network/server error — mark URL rows unresolved so the user
            // can retry by editing the paste.
            for (const e of urlEntries) {
              updateRow(generation, e.index, { status: 'unresolved', item: null })
            }
          },
        }
      )
    }

    // ── Plain-text lines: per-line auto-search, bounded to 5 in flight ──
    const plaintextEntries = parsed
      .map((p, i) => ({ raw: p.raw, index: i }))
      .filter((e) => parsed[e.index].url === null)

    if (plaintextEntries.length > 0) {
      void runWithConcurrency(
        plaintextEntries,
        PLAINTEXT_SEARCH_CONCURRENCY,
        async (entry) => {
          // Bail early if a newer paste superseded this one.
          if (generationRef.current !== generation) return
          let candidates: EntitySearchResult[]
          try {
            const { results } = await fetchEntitySearch(entry.raw)
            candidates = flattenEntitySearchResults(results)
          } catch {
            // Search outage for this line — mark unresolved (retryable by
            // editing the paste). Don't queue: a transient failure isn't a
            // confirmed zero-result.
            updateRow(generation, entry.index, {
              status: 'unresolved',
              item: null,
            })
            return
          }
          if (generationRef.current !== generation) return

          if (candidates.length === 1) {
            updateRow(generation, entry.index, {
              status: 'matched',
              item: toPreviewItem(candidates[0]),
            })
          } else if (candidates.length > 1) {
            updateRow(generation, entry.index, {
              status: 'ambiguous',
              item: null,
              candidates: candidates
                .slice(0, MAX_PICK_CANDIDATES)
                .map(toPreviewItem),
            })
          } else {
            // Zero results ⇒ queue for admin review. Await the POST so it
            // counts against the concurrency budget — without this, a paste
            // of N all-junk lines would fire N POSTs at once (the search
            // bound would be moot for the queue side).
            await fileQueueRequest(generation, entry.index, entry.raw)
          }
        }
      )
    }
    // Only re-resolve when the debounced paste text changes. alreadyStaged is
    // computed at render time off the current stagedItems prop (PastePreviewRow),
    // so staged/existing changes don't need to re-fire resolution.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedPaste])

  // Promote an AMBIGUOUS row to MATCH with the user's chosen candidate.
  const pickCandidate = useCallback(
    (rowIndex: number, candidate: PreviewCandidate) => {
      setPreviewRows((rows) => {
        if (rowIndex < 0 || rowIndex >= rows.length) return rows
        const copy = rows.slice()
        copy[rowIndex] = {
          ...copy[rowIndex],
          status: 'matched',
          item: candidate,
          candidates: undefined,
        }
        return copy
      })
    },
    []
  )

  // Retry a failed queue POST for a zero-result line (against the CURRENT
  // generation so it survives the row-bounds check).
  const retryQueue = useCallback(
    (rowIndex: number) => {
      const row = previewRows[rowIndex]
      if (!row || row.status !== 'queue_failed') return
      fileQueueRequest(generationRef.current, rowIndex, row.raw)
    },
    [previewRows, fileQueueRequest]
  )

  return { previewRows, pickCandidate, retryQueue }
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

  // ─── Paste mode state (resolution lives in usePastePreview) ───
  const [pasteText, setPasteText] = useState('')
  const { previewRows, pickCandidate, retryQueue } = usePastePreview(pasteText)

  // Flattened search results for the active query. Mirrors the existing
  // AddItemsSection shape so users get a familiar list. The flatten order
  // is single-sourced in flattenEntitySearchResults so the interactive
  // search list and the plain-text auto-match (usePastePreview) agree.
  const searchRows: EntitySearchResult[] = useMemo(
    () => (searchResults ? flattenEntitySearchResults(searchResults) : []),
    [searchResults]
  )

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
            onPick={pickCandidate}
            onRetryQueue={retryQueue}
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
  onPick,
  onRetryQueue,
}: {
  text: string
  onTextChange: (v: string) => void
  previewRows: PreviewRow[]
  existingItems: ExistingItemKey[]
  stagedItems: StagedCollectionItem[]
  onStage: (item: StagedCollectionItem) => void
  onStageBatch: (items: StagedCollectionItem[]) => void
  onPick: (rowIndex: number, candidate: PreviewCandidate) => void
  onRetryQueue: (rowIndex: number) => void
}) {
  const matchedCount = previewRows.filter((r) => r.status === 'matched').length
  const unresolvedCount = previewRows.filter((r) => r.status === 'unresolved').length
  // In-flight rows (URL batch resolve + plain-text per-line search) share one
  // "resolving" tally — both are transient, both end at a terminal state.
  const loadingCount = previewRows.filter(
    (r) => r.status === 'loading' || r.status === 'searching'
  ).length
  const ambiguousCount = previewRows.filter((r) => r.status === 'ambiguous').length
  // Queued + queuing + queue_failed all count toward "for review" — the line
  // had no match and is (or will be) an admin-reviewable request.
  const queuedCount = previewRows.filter(
    (r) =>
      r.status === 'queued' ||
      r.status === 'queuing' ||
      r.status === 'queue_failed'
  ).length

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
          'One item per line — a PH link or just a name:\n' +
          'https://psychichomily.com/artists/kendrick-lamar\n' +
          '/releases/to-pimp-a-butterfly\n' +
          'Frank Ocean'
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
          {/* Counts joined with " · " via a parts array so adding a status to
              the tally doesn't grow the brittle pairwise-separator chain. */}
          <p className="text-xs text-muted-foreground">
            {[
              matchedCount > 0 && `${matchedCount} matched`,
              loadingCount > 0 && `${loadingCount} resolving`,
              ambiguousCount > 0 && `${ambiguousCount} need a pick`,
              queuedCount > 0 && `${queuedCount} for review`,
              unresolvedCount > 0 && `${unresolvedCount} unresolved`,
            ]
              .filter(Boolean)
              .join(' · ')}
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
              onPick={(candidate) => onPick(index, candidate)}
              onRetryQueue={() => onRetryQueue(index)}
            />
          ))}
        </div>
      )}

      {previewRows.length > 0 && queuedCount > 0 && (
        <p className="text-xs text-muted-foreground">
          Lines with no match are filed as creation requests for an admin to
          review — they won&apos;t appear in your collection until approved.
        </p>
      )}

      {previewRows.length > 0 && unresolvedCount > 0 && (
        <p className="text-xs text-muted-foreground">
          Unresolved lines are a canonical PH path that didn&apos;t match
          (e.g. <code className="px-1 rounded bg-muted">/artists/&lt;slug&gt;</code>),
          or search was momentarily unavailable. Re-paste to retry, or switch
          to the AI tab for an article URL or pasted prose.
        </p>
      )}
    </div>
  )
}

function PastePreviewRow({
  row,
  alreadyStaged,
  onAdd,
  onPick,
  onRetryQueue,
}: {
  row: PreviewRow
  alreadyStaged: boolean
  onAdd: () => void
  onPick: (candidate: PreviewCandidate) => void
  onRetryQueue: () => void
}) {
  const candidates = row.candidates ?? []
  return (
    <div
      className="rounded-md p-2 hover:bg-muted/50"
      data-testid="add-items-picker-paste-row"
    >
      <div className="flex items-center gap-3">
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

        {(row.status === 'loading' || row.status === 'searching') && (
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
        {row.status === 'ambiguous' && (
          // PICK uses the soft pending surface token, mirroring AICollectionFiller's
          // "did you mean" suggestion chips — an ambiguous match is a prompt for a
          // decision, not an error.
          <Badge
            variant="secondary"
            className="text-[10px] px-1.5 py-0 shrink-0 bg-pending text-pending-foreground motion-safe:animate-in motion-safe:fade-in"
          >
            <AlertTriangle className="h-3 w-3 mr-0.5" />
            PICK
          </Badge>
        )}
        {row.status === 'queuing' && (
          <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
            <Loader2 className="h-3 w-3 mr-0.5 animate-spin" />
            Queuing…
          </Badge>
        )}
        {row.status === 'queued' && (
          <Badge
            variant="secondary"
            className="text-[10px] px-1.5 py-0 shrink-0 bg-pending text-pending-foreground motion-safe:animate-in motion-safe:fade-in"
            data-testid="add-items-picker-paste-row-queued"
          >
            <Inbox className="h-3 w-3 mr-0.5" />
            FOR REVIEW
          </Badge>
        )}
        {row.status === 'queue_failed' && (
          <Button
            variant="ghost"
            size="sm"
            className="h-7 px-2 shrink-0 text-destructive"
            onClick={onRetryQueue}
            data-testid="add-items-picker-paste-row-retry-queue"
          >
            <AlertCircle className="h-3.5 w-3.5 mr-1" />
            Retry
          </Button>
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

      {/* AMBIGUOUS: inline [Pick] candidate dropdown (≤5). Picking promotes the
          row to MATCH. Mirrors AICollectionFiller's "did you mean" chip row. */}
      {row.status === 'ambiguous' && candidates.length > 0 && (
        <div
          className="ml-10 mt-1.5 flex items-center gap-1.5 flex-wrap text-xs"
          data-testid="add-items-picker-paste-row-pick"
        >
          <span className="text-pending-foreground">Did you mean:</span>
          {candidates.map((candidate) => (
            <button
              key={`${candidate.entityType}-${candidate.entityId}`}
              type="button"
              className="rounded-md border border-pending-foreground/20 bg-pending px-2 py-0.5 text-xs text-pending-foreground hover:bg-pending/80 transition-colors"
              onClick={() => onPick(candidate)}
            >
              {candidate.name}
              <span className="ml-1 opacity-70">
                {getEntityTypeLabel(candidate.entityType)}
              </span>
            </button>
          ))}
        </div>
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
