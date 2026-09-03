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
import { useVirtualizer } from '@tanstack/react-virtual'
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
  Undo2,
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
  type SensorDescriptor,
  type SensorOptions,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
  arrayMove,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  ENTITY_ICONS,
  REPLACED_REQUEST_EXPLANATION,
  WITHDRAW_REFUSED_MESSAGE,
} from './collectionDetailShared'
import type { components } from '@/types/api'

/**
 * The batch queue-create response, aliased from the generated OpenAPI types so a
 * backend rename fails the build here instead of silently reporting every
 * submission as a first filing.
 */
type EntityRequestBatchResponse = components['schemas']['CreateEntityRequestBatchResponseBody']
type EntityRequestBatchResult = components['schemas']['EntityRequestBatchResult']

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
 *   - `queue_refused` — zero results ⇒ the server refused THIS line's content
 *                       (e.g. a name past the 255-character cap). Distinct from
 *                       `queue_failed` because re-sending the same line is
 *                       refused the same way: the line has to change first, so
 *                       the row offers the reason instead of a Retry.
 *   - `withdrawing`   — the requester is retracting the request this line filed
 *   - `withdrawn`     — retracted; no admin will see it
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
  | 'queue_refused'
  | 'withdrawing'
  | 'withdrawn'

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
  /**
   * `queued` rows only: the request corrected the requester's own earlier
   * queued request under this name rather than filing a new one. A field and
   * not a status, because the row is queued for review either way and every
   * count that reads `status` still means what it meant.
   */
  replaced?: boolean
  /**
   * `queue_refused` rows only: the server's own words for why this line was
   * refused. The reason is per line, so it is on the row rather than on a
   * surface shared by the whole paste.
   */
  refusal?: string
  /**
   * The stored request this line filed. Present on `queued` and the two withdraw
   * states; it is what the withdraw affordance names, so a row without it offers
   * none. Lines that collapsed onto one request all carry the SAME id, so
   * withdrawing from any of them retracts the one request they share.
   */
  requestId?: number
  /**
   * A failed withdrawal's message. It is set on every row naming the request the
   * withdrawal was for, which is more than one when identical lines collapsed
   * onto it: they are one request, so they carry one verdict. The rows stay
   * `queued` on a failure, because the request they name still is.
   */
  withdrawError?: string
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
 * LOCAL queue-create call (PSY-845, batched by PSY-2005). Posts the zero-result
 * lines of one paste to `POST /entity-requests/batch`, so a plain-text line with
 * no matches becomes an admin-reviewable artist-creation request rather than
 * being silently dropped. One request per QUEUE_BATCH_MAX_ITEMS names, which for
 * every paste at or under that size is one request.
 *
 * Deliberately LOCAL (not a shared exported hook): AICollectionFiller posts to
 * the same endpoint from a different surface, one item at a time as the user
 * acts on a row, and the two consumers share the endpoint rather than a hook.
 *
 * A plain async function, NOT a `useMutation` hook: usePastePreview owns the
 * per-row status, so the hook's data/isPending/error would be unused, and a
 * retry of one row runs beside a paste's own batch.
 *
 * Entity type is `artist`: a bare plain-text line in a music collection picker
 * is overwhelmingly an artist name, and `artist` is the only entity_request
 * payload whose sole required field is `name` (releases need a title, shows an
 * event_date, venues city+state, etc.) — so the line text is sufficient to
 * file a well-formed request. The admin reviewing the queue retypes /
 * reclassifies if it was actually a release or venue.
 *
 * Results come back one per item, and each carries the index of the name in the
 * CALLER's list: a chunk indexes from zero on the wire, and the offset is added
 * back before the result is returned, so the caller never sees the split.
 *
 * `replaced` says the request landed on the requester's own queued row under
 * this name rather than filing a new one, and `refused` says nothing was stored
 * for that line and why. A name with NO result is one nothing answered for; see
 * the chunk-failure note in the body.
 */
async function queueEntityRequestBatch(
  names: string[]
): Promise<EntityRequestBatchResult[]> {
  const results: EntityRequestBatchResult[] = []
  for (let sent = 0; sent < names.length; sent += QUEUE_BATCH_MAX_ITEMS) {
    const chunk = names.slice(sent, sent + QUEUE_BATCH_MAX_ITEMS)
    let res: EntityRequestBatchResponse
    try {
      res = await apiRequest<EntityRequestBatchResponse>(
        API_ENDPOINTS.COLLECTIONS.ENTITY_REQUESTS_BATCH,
        {
          method: 'POST',
          body: JSON.stringify({
            items: chunk.map(name => ({
              entity_type: 'artist',
              payload: { name },
              source_context: 'paste_mode',
            })),
          }),
        }
      )
    } catch {
      // A failed chunk RESOLVES with what the earlier ones answered rather than
      // rejecting. Rejecting would mark every row failed, including the ones
      // already filed, and retrying those would file replacements the
      // contributor never made. Items with no result are the caller's
      // queue_failed, which is exactly the rows this chunk and the ones after it
      // cover. Stopping is deliberate: the next chunk is one more request at the
      // same server the last one just failed against.
      break
    }
    // Each chunk indexes from zero, so the offset restores the caller's own
    // positions and the results read as one list.
    for (const result of res?.results ?? []) {
      results.push({ ...result, index: result.index + sent })
    }
  }
  return results
}

/**
 * The identity two paste lines have to share before this component treats them
 * as ONE request: the trimmed line, byte for byte.
 *
 * It is deliberately NOT the server's dedup key. That key is case-folded in
 * Postgres, and a client that guessed at the folding could fold two names the
 * server keeps apart, which would silently never file the second one. Two
 * identical lines are one request under any folding, so this collapse can only
 * ever be too conservative. Lines the SERVER then merges are caught by their
 * shared request id in the results, so the collapse is an optimisation and not
 * the thing correctness rests on.
 */
function pasteQueueKey(raw: string): string {
  return raw.trim()
}

/**
 * How many names this component puts in one batch request. It is chosen to be at
 * most the endpoint's own cap, which nothing here can read: the cap lives in a
 * Go struct tag and does not reach the generated types.
 *
 * A paste of more zero-result lines than this is SPLIT rather than sent as one
 * body, because a body over the endpoint's cap is refused whole. Should the
 * endpoint's cap ever drop below this number, a full chunk is refused whole and
 * its rows land on the retryable failed state with a Retry, rather than
 * vanishing - which is what the chunk-failure handling below buys.
 */
const QUEUE_BATCH_MAX_ITEMS = 200

/**
 * Retract a request this paste filed (PSY-1992). Resolves when the request is
 * withdrawn; rejects with the server's message otherwise, which is what the row
 * surfaces.
 */
function withdrawEntityRequest(requestId: number): Promise<void> {
  return apiRequest(API_ENDPOINTS.COLLECTIONS.ENTITY_REQUEST_WITHDRAW(requestId), {
    method: 'POST',
  }).then(() => undefined)
}

/**
 * What a refused withdrawal says. The server's own message names the reason
 * (already reviewed, no longer the caller's), which is more use than the generic
 * line; the constant is the fallback for a failure that carries no message at
 * all, such as a dropped connection.
 */
function withdrawFailureMessage(err: unknown): string {
  const message = err instanceof Error ? err.message.trim() : ''
  return message || WITHDRAW_REFUSED_MESSAGE
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

/**
 * Above this many staged items, the list windows its rows with
 * `@tanstack/react-virtual` (PSY-994) instead of mounting every row. Below it,
 * the full non-virtual render stays — drag-reorder is the common case at small
 * N and a non-windowed list is simpler + needs no dnd-kit coordination.
 *
 * 30 is the ticket's locked threshold: comfortably above the 10-row visible
 * window (so the scroll-but-no-window band — 11..30 — keeps the cheap,
 * fully-mounted render the design was built against) and well below the
 * 200-item scale where unbounded DOM actually bites.
 */
const STAGED_LIST_VIRTUALIZE_THRESHOLD = 30

/**
 * Estimated row height (px) fed to the virtualizer. Rows are a single line of
 * text in a `py-1.5` (6px top + 6px bottom) padded box with a 1px border and a
 * `space-y-0.5` (2px) gap → ~40px. The virtualizer measures each real row via
 * `measureElement` after mount, so this estimate only sets the initial window
 * size + scrollbar extent; an off-by-a-few estimate self-corrects on measure.
 */
const STAGED_ROW_ESTIMATED_HEIGHT = 40

/**
 * Extra rows rendered above + below the visible window — purely to keep a fast
 * idle scroll from flashing blank rows before the virtualizer catches up.
 * Overscan plays NO role in drag/keyboard reorder: a drag (pointer OR keyboard,
 * which lifts via `onDragStart`) flips to the full-flow render where EVERY row
 * is mounted, so reorder reaches any index regardless of this value. It is an
 * idle-scroll smoothing knob only.
 */
const STAGED_LIST_OVERSCAN = 8

/** Fixed max height (px) of the staged-list scroll viewport — the single
 *  source for both render modes (windowed + full flow), applied as an inline
 *  style on the shared scroll container so the two modes stay visually
 *  interchangeable across the threshold. (Was the PSY-823 `max-h-[420px]`.) */
const STAGED_LIST_VIEWPORT_HEIGHT = 420

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
  withdrawQueued: (rowIndex: number) => void
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

  // Commit many rows at once IFF the captured generation is still current. One
  // clone of the list for a whole batch, rather than one per row: a 200-line
  // paste stamps and then settles every row in two passes.
  const updateRows = useCallback(
    (generation: number, next: Map<number, Partial<PreviewRow>>) => {
      setPreviewRows((rows) => {
        if (generationRef.current !== generation) return rows
        if (next.size === 0) return rows
        return rows.map((row, index) => {
          const patch = next.get(index)
          return patch ? { ...row, ...patch } : row
        })
      })
    },
    []
  )

  // File a queue-for-review request for every zero-result plain-text line of one
  // paste, in as few round trips as the endpoint's cap allows. Extracted so the
  // initial pass AND retryQueue share one code path.
  //
  // Identical lines are sent ONCE and every row sharing the line reads that one
  // result. Lines the SERVER merges instead (a case difference, say) come back
  // with the SAME request id, and the second one is reported as whatever the
  // first said rather than as a replacement: it corrects nothing, it is the same
  // line twice. Without that, a paste tells the contributor it updated a request
  // they never filed.
  //
  // A 4xx item is terminal for that line, so it lands on queue_refused with the
  // server's reason and no Retry. A 5xx item stays on the retryable queue_failed,
  // and so does a name with no result at all, which is what a chunk that failed
  // to send leaves behind.
  const fileQueueBatch = useCallback(
    (generation: number, entries: { raw: string; index: number }[]): Promise<void> => {
      if (entries.length === 0) return Promise.resolve()

      const order: string[] = []
      const rowsByKey = new Map<string, number[]>()
      for (const entry of entries) {
        const key = pasteQueueKey(entry.raw)
        const rows = rowsByKey.get(key)
        if (rows) {
          rows.push(entry.index)
          continue
        }
        rowsByKey.set(key, [entry.index])
        order.push(entry.raw)
      }
      const rowsForItem = [...rowsByKey.values()]

      updateRows(
        generation,
        new Map(entries.map((e) => [e.index, { status: 'queuing' as const }]))
      )

      const settleAll = (next: Partial<PreviewRow>) =>
        new Map(entries.map((e) => [e.index, next]))

      return queueEntityRequestBatch(order).then(
        (results) => {
          const byIndex = new Map(results.map((r) => [r.index, r]))
          // The verdict already reported for a request id, so a second item that
          // landed on the same row repeats it instead of claiming a correction.
          const verdictByRequest = new Map<number, Partial<PreviewRow>>()
          const patches = new Map<number, Partial<PreviewRow>>()

          rowsForItem.forEach((rowIndexes, itemIndex) => {
            const result = byIndex.get(itemIndex)
            let patch: Partial<PreviewRow>
            if (!result) {
              // An item with no result was neither filed nor refused, so the row
              // says the request did not land and stays retryable.
              patch = { status: 'queue_failed' }
            } else if (result.status === 'refused') {
              patch =
                result.error_status !== undefined && result.error_status >= 500
                  ? { status: 'queue_failed' }
                  : {
                      status: 'queue_refused',
                      refusal: result.error ?? undefined,
                    }
            } else {
              const id = result.id ?? undefined
              const seen = id !== undefined ? verdictByRequest.get(id) : undefined
              patch = seen ?? {
                status: 'queued',
                replaced: result.status === 'replaced',
                requestId: id,
                withdrawError: undefined,
              }
              if (id !== undefined && seen === undefined) {
                verdictByRequest.set(id, patch)
              }
            }
            for (const rowIndex of rowIndexes) patches.set(rowIndex, patch)
          })

          updateRows(generation, patches)
        },
        // queueEntityRequestBatch resolves with what it has rather than
        // rejecting, so reaching here means it threw for a reason it does not
        // handle. Every row settles retryable, which is the safe reading of an
        // answer nobody got.
        () => updateRows(generation, settleAll({ status: 'queue_failed' }))
      )
    },
    [updateRows]
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
      // Zero-result lines collected across the whole search pass and filed as ONE
      // batch when it settles. The pass is concurrent, so this is appended to
      // from several workers; JS runs them on one thread, and each push happens
      // in a single synchronous continuation, so the array needs no lock.
      const toQueue: { raw: string; index: number }[] = []
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
            // Zero results ⇒ queue for admin review, once the whole pass has
            // settled. Collected rather than filed here so a paste of N
            // zero-result lines is ONE request instead of N: the tail of a large
            // paste is what a per-line POST puts at the mercy of any per-request
            // ceiling.
            toQueue.push({ raw: entry.raw, index: entry.index })
          }
        }
      ).then(() => {
        if (generationRef.current !== generation) return
        void fileQueueBatch(generation, toQueue)
      })
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

  // Retry a failed queue request for a zero-result line (against the CURRENT
  // generation so it survives the row-bounds check). A refused row is not
  // retryable and has no Retry to press.
  //
  // The retry carries EVERY failed row sharing this one's queue key, not just
  // the row that was pressed: they are one request, so retrying them
  // separately would file the first and have the rest land on it and report
  // themselves as corrections.
  const retryQueue = useCallback(
    (rowIndex: number) => {
      const row = previewRows[rowIndex]
      if (!row || row.status !== 'queue_failed') return
      const key = pasteQueueKey(row.raw)
      const entries = previewRows
        .map((r, index) => ({ raw: r.raw, index, status: r.status }))
        .filter(
          (r) => r.status === 'queue_failed' && pasteQueueKey(r.raw) === key
        )
        .map(({ raw, index }) => ({ raw, index }))
      void fileQueueBatch(generationRef.current, entries)
    },
    [previewRows, fileQueueBatch]
  )

  // Retract the request a queued row filed (PSY-1992). Every row that collapsed
  // onto the same request moves together, because they name ONE request and a
  // row still reading "for review" beside a withdrawn twin would be false.
  const withdrawQueued = useCallback(
    (rowIndex: number) => {
      const row = previewRows[rowIndex]
      if (!row || row.status !== 'queued' || row.requestId === undefined) return
      const requestId = row.requestId
      const generation = generationRef.current

      const applyToRequest = (next: Partial<PreviewRow>) => {
        setPreviewRows((rows) => {
          if (generationRef.current !== generation) return rows
          return rows.map((r) =>
            r.requestId === requestId ? { ...r, ...next } : r
          )
        })
      }

      applyToRequest({ status: 'withdrawing', withdrawError: undefined })
      void withdrawEntityRequest(requestId).then(
        () => applyToRequest({ status: 'withdrawn' }),
        (err: unknown) =>
          applyToRequest({
            status: 'queued',
            withdrawError: withdrawFailureMessage(err),
          })
      )
    },
    [previewRows]
  )

  return { previewRows, pickCandidate, retryQueue, withdrawQueued }
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
  const { previewRows, pickCandidate, retryQueue, withdrawQueued } =
    usePastePreview(pasteText)

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
            onWithdrawQueued={withdrawQueued}
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

      {/* Staged list (PSY-962: overview strip + drag-reorderable detail list;
          PSY-994: windows the detail list above STAGED_LIST_VIRTUALIZE_THRESHOLD) */}
      {stagedCount > 0 && (
        <div className="mt-3 border-t border-border/50 pt-3 space-y-2">
          <StagedOverviewStrip items={stagedItems} />
          <StagedItemsList
            items={stagedItems}
            stagedIds={stagedIds}
            sensors={sensors}
            onReorder={handleReorder}
            onRemove={unstageItem}
          />
        </div>
      )}
    </div>
  )
}

// ──────────────────────────────────────────────
// Staged list (PSY-962 drag-reorder + PSY-994 virtualization)
// ──────────────────────────────────────────────

/**
 * The drag-reorderable staged-item list. Owns the @dnd-kit `DndContext` and,
 * above {@link STAGED_LIST_VIRTUALIZE_THRESHOLD} items, windows its rows with
 * `@tanstack/react-virtual` so a 200-item staging session doesn't mount 200
 * rows (PSY-994).
 *
 * ── dnd-kit ↔ virtualization coordination: VIRTUALIZE-WHEN-IDLE, ──
 * ──                                          ONE SCROLL CONTAINER  ──
 *
 * `@dnd-kit/sortable` hit-tests and computes keyboard-reorder targets against
 * the rows currently MOUNTED inside its `SortableContext`. A naively windowed
 * list only mounts ~12 rows, so a drag could never reach an off-window target
 * and keyboard reorder would stop at the window edge.
 *
 * The chosen strategy is the simplest that satisfies the AC (Code Complete:
 * pick the simplest design that works; isolate the volatile machinery here):
 *
 *   - IDLE (not dragging): window the rows. Only ~12 rows + overscan are in the
 *     DOM — the bounded-DOM goal, and the state the user inspects "at rest".
 *   - ACTIVE DRAG: render EVERY row (full flow list). `onDragStart` flips
 *     `isDragging`, which swaps the inner content to the full list for the
 *     duration of the drag, then `onDragEnd`/`onDragCancel` flip it back. While
 *     dragging, every sortable is mounted, so the dragged row never unmounts
 *     mid-drag, pointer drags reach any target (dnd-kit auto-scroll drives the
 *     container), and keyboard reorder crosses the whole list.
 *
 *   - CRITICAL: both modes render INTO THE SAME, always-mounted scroll
 *     container `<div ref={scrollRef}>` — only its *inner* content swaps. The
 *     container is never unmounted across the idle↔drag switch, so its
 *     `scrollTop` is preserved. (An earlier two-container version remounted a
 *     fresh `scrollTop:0` div on drag-start, so grabbing a below-fold row in a
 *     200-item list reset the scroll to top and the drag jumped off-screen —
 *     PSY-994 adversarial review.) A preserved scrollTop maps to the SAME row
 *     across the switch: to reach a below-fold row the user scrolls PAST every
 *     row above it, so the virtualizer has already `measureElement`-measured
 *     them — the current scroll offset reflects real measured heights, not the
 *     estimate. Estimate-vs-actual drift lives only BELOW the viewport, where
 *     it can't shift the row under the pointer.
 *
 * This sidesteps the fragile alternative (keep a window-around-the-dragged-row
 * mounted + hand-rolled auto-scroll + measured-position juggling) that the
 * ticket flags as the hard part.
 *
 * `prefers-reduced-motion` is honored throughout: each row's enter animation
 * stays gated on Tailwind's `motion-safe:` variant. Virtualization adds no new
 * animation CLASS, but the windowed path mounts/unmounts rows on scroll — which
 * would re-fire the mount-triggered `animate-in` on every scroll-in. To avoid
 * that flashing wave, windowed rows are rendered with `animateEnter={false}`
 * (the enter fade-in plays only in the full-flow render, for genuinely new rows).
 */
function StagedItemsList({
  items,
  stagedIds,
  sensors,
  onReorder,
  onRemove,
}: {
  items: StagedCollectionItem[]
  stagedIds: string[]
  sensors: SensorDescriptor<SensorOptions>[]
  onReorder: (event: DragEndEvent) => void
  onRemove: (entityType: string, entityId: number) => void
}) {
  const [isDragging, setIsDragging] = useState(false)
  const canReorder = items.length > 1
  const scrollRef = useRef<HTMLDivElement>(null)
  // Window the rows ONLY when idle and above the threshold. During an active
  // drag we render every row INTO THE SAME scroll container (see the strategy
  // note above) so dnd-kit can hit-test / keyboard-move across the whole list
  // AND the container's scrollTop is preserved across the idle↔drag switch
  // (no remount → grabbing a below-fold row no longer resets scroll to top).
  const windowed =
    items.length > STAGED_LIST_VIRTUALIZE_THRESHOLD && !isDragging
  // The list scrolls (and caps its height) once it passes the visible-row
  // window — true across the whole virtualized range and the 11..30 band.
  const scrollable = items.length > STAGED_LIST_MAX_VISIBLE

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => STAGED_ROW_ESTIMATED_HEIGHT,
    overscan: STAGED_LIST_OVERSCAN,
    // Stable key per item so the measurement cache survives a reorder (keyed by
    // identity, not index) — mirrors the React key + dnd-kit sortable id.
    getItemKey: (index) => stagedKey(items[index]),
  })

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      setIsDragging(false)
      onReorder(event)
    },
    [onReorder]
  )

  const virtualRows = virtualizer.getVirtualItems()

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={() => setIsDragging(true)}
      onDragEnd={handleDragEnd}
      onDragCancel={() => setIsDragging(false)}
    >
      <SortableContext items={stagedIds} strategy={verticalListSortingStrategy}>
        {/* ONE scroll container for both render modes — it stays mounted across
            the idle↔drag switch so scrollTop is preserved; only the inner
            content swaps (windowed absolute rows ↔ full flow list). maxHeight is
            an inline style (not a Tailwind arbitrary value) so the windowed and
            flow paths share the one STAGED_LIST_VIEWPORT_HEIGHT source. */}
        <div
          ref={scrollRef}
          className={cn(scrollable && 'overflow-y-auto')}
          style={
            scrollable ? { maxHeight: STAGED_LIST_VIEWPORT_HEIGHT } : undefined
          }
          data-testid="add-items-picker-staged-list"
          // Marks the windowed render path. Load-bearing test/inspection hook:
          // the threshold tests assert its presence (>threshold, idle) +
          // absence (≤threshold, or the mid-drag full render). Don't drop it on
          // a refactor without updating those tests.
          data-virtualized={windowed ? 'true' : undefined}
        >
          {windowed ? (
            <div
              className="relative w-full"
              style={{ height: virtualizer.getTotalSize() }}
            >
              {virtualRows.map((vr) => {
                const item = items[vr.index]
                return (
                  <div
                    key={vr.key}
                    data-index={vr.index}
                    ref={virtualizer.measureElement}
                    className="absolute left-0 top-0 w-full"
                    style={{ transform: `translateY(${vr.start}px)` }}
                  >
                    {/* pb wrapper reproduces the flow `space-y-0.5` gap so the
                        two render modes look identical across the threshold. */}
                    <div className="pb-0.5">
                      <StagedRow
                        index={vr.index}
                        item={item}
                        canReorder={canReorder}
                        onRemove={() => onRemove(item.entityType, item.entityId)}
                        // Scroll-recycled rows must not re-fire the enter fade-in.
                        animateEnter={false}
                      />
                    </div>
                  </div>
                )
              })}
            </div>
          ) : (
            <div className="space-y-0.5">
              {items.map((item, index) => (
                <StagedRow
                  key={stagedKey(item)}
                  index={index}
                  item={item}
                  canReorder={canReorder}
                  onRemove={() => onRemove(item.entityType, item.entityId)}
                />
              ))}
            </div>
          )}
        </div>
      </SortableContext>
    </DndContext>
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
  onWithdrawQueued,
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
  onWithdrawQueued: (rowIndex: number) => void
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
  // A withdrawn line has left the review queue, so it leaves that tally too;
  // counting it there would promise an admin will see something nobody will.
  const withdrawnCount = previewRows.filter(
    (r) => r.status === 'withdrawn' || r.status === 'withdrawing'
  ).length
  // A refused line has its own tally: it is NOT for review, and counting it
  // there would tell the user an admin will see a line nothing filed.
  const refusedCount = previewRows.filter(
    (r) => r.status === 'queue_refused'
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
              refusedCount > 0 && `${refusedCount} refused`,
              withdrawnCount > 0 && `${withdrawnCount} withdrawn`,
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
              onWithdrawQueued={() => onWithdrawQueued(index)}
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
  onWithdrawQueued,
}: {
  row: PreviewRow
  alreadyStaged: boolean
  onAdd: () => void
  onPick: (candidate: PreviewCandidate) => void
  onRetryQueue: () => void
  onWithdrawQueued: () => void
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
        {/* A replaced row is still queued for review, so it keeps the pending
            register and only its word changes. Both spellings of the
            explanation are required; see the constant's own contract. */}
        {row.status === 'queued' && (
          <>
            <Badge
              variant="secondary"
              className="text-[10px] px-1.5 py-0 shrink-0 bg-pending text-pending-foreground motion-safe:animate-in motion-safe:fade-in"
              data-testid="add-items-picker-paste-row-queued"
              title={row.replaced ? REPLACED_REQUEST_EXPLANATION : undefined}
            >
              <Inbox className="h-3 w-3 mr-0.5" />
              {row.replaced ? 'UPDATED' : 'FOR REVIEW'}
              {row.replaced && (
                <span className="sr-only"> {REPLACED_REQUEST_EXPLANATION}</span>
              )}
            </Badge>
            {/* PSY-1992: retract the request this line filed, beside the chip
                that says it is queued. Offered only when the response named the
                request, since the call has to name it. */}
            {row.requestId !== undefined && (
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 shrink-0 text-muted-foreground"
                onClick={onWithdrawQueued}
                aria-label={`Withdraw the request for ${row.raw}`}
                data-testid="add-items-picker-paste-row-withdraw"
              >
                <Undo2 className="h-3.5 w-3.5 mr-1" />
                Withdraw
              </Button>
            )}
          </>
        )}
        {row.status === 'withdrawing' && (
          <Badge variant="secondary" className="text-[10px] px-1.5 py-0 shrink-0">
            <Loader2 className="h-3 w-3 mr-0.5 animate-spin" />
            Withdrawing…
          </Badge>
        )}
        {/* A withdrawn line is not for review and not an error: no admin will
            see it, and nothing went wrong. */}
        {row.status === 'withdrawn' && (
          <Badge
            variant="secondary"
            className="text-[10px] px-1.5 py-0 shrink-0 motion-safe:animate-in motion-safe:fade-in"
            data-testid="add-items-picker-paste-row-withdrawn"
          >
            <Undo2 className="h-3 w-3 mr-0.5" />
            WITHDRAWN
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
        {/* A refused line offers no Retry: the server refused this line's own
            content, so re-sending it unchanged is refused the same way. */}
        {row.status === 'queue_refused' && (
          <Badge
            variant="secondary"
            className="text-[10px] px-1.5 py-0 shrink-0 bg-destructive/10 text-destructive motion-safe:animate-in motion-safe:fade-in"
            data-testid="add-items-picker-paste-row-refused"
          >
            <AlertCircle className="h-3 w-3 mr-0.5" />
            REFUSED
          </Badge>
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

      {/* A failed withdrawal, on every row naming the request it was for. The
          rows are still queued, so the message is the only thing that changed. */}
      {row.withdrawError && (
        <p
          className="ml-10 mt-1.5 text-xs text-destructive"
          data-testid="add-items-picker-paste-row-withdraw-error"
        >
          {row.withdrawError}
        </p>
      )}

      {/* REFUSED: the server's own reason, on the row it belongs to. Nothing was
          filed for this line, and the reason is what says which edit would make
          it filable. */}
      {row.status === 'queue_refused' && row.refusal && (
        <p
          className="ml-10 mt-1.5 text-xs text-destructive"
          data-testid="add-items-picker-paste-row-refusal"
        >
          {row.refusal}
        </p>
      )}

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
  animateEnter = true,
}: {
  index: number
  item: StagedCollectionItem
  canReorder: boolean
  onRemove: () => void
  /**
   * Whether the row plays its enter fade-in on mount. TRUE in the full-flow
   * render (a newly-staged item entering is meaningful). FALSE in the windowed
   * render: there, scrolling continuously mounts/unmounts rows, so the
   * mount-triggered `animate-in` would re-fire on every scroll-in — a flashing
   * wave on exactly the large-N path PSY-994 targets (adversarial review:
   * Saboteur + Future-Maintainer + Completeness).
   */
  animateEnter?: boolean
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
      className={cn(
        'flex items-center gap-2 rounded-md px-2 py-1.5 border border-border/40 bg-popover',
        // Enter animation only when this row is genuinely entering (full-flow
        // render); suppressed for scroll-recycled windowed rows — see the
        // animateEnter prop doc.
        animateEnter && 'motion-safe:animate-in motion-safe:fade-in'
      )}
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
