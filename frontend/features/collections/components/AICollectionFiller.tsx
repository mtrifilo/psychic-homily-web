'use client'

/**
 * AICollectionFiller — AI-assisted collection-item extraction.
 *
 * Sister to `frontend/components/forms/AIFormFiller.tsx` (show extraction).
 * Duplicate-not-parameterize at N=2 — refactor to a shared `AIInputCard`
 * primitive when a 3rd consumer lands. The
 * text+image+compression chrome is structurally the same; the post-extract
 * preview UI is different (per-row collection items with name-only match
 * + Pick UX, vs show-extraction's artists+venue+date+time).
 *
 * Accepts text and/or an image. On success, surfaces per-row matched /
 * with-suggestions / new states from `/api/ai/extract-collection`. User
 * stages matched (and picked-suggestion) rows into the parent
 * AddItemsPicker via the `onStageItems` callback.
 *
 * iOS Safari HEIC support: file input accepts `image/heic`,`image/heif` so
 * iOS users can upload HEIC screenshots directly. Safari's native canvas
 * decode converts them to JPEG via the compression step (the existing
 * canvas.toDataURL('image/jpeg') path). Non-Safari browsers without HEIC
 * decode will fall through the onerror path and show an inline error —
 * full polyfill via `heic2any` deferred to a follow-up if telemetry
 * shows non-Safari HEIC failures.
 */

import { useCallback, useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  Sparkles,
  X,
  Loader2,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  ImageIcon,
  Plus,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { InlineErrorBanner } from '@/components/shared'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useCollectionExtraction } from '../hooks'
import type {
  ExtractedCollectionData,
  ExtractedCollectionItem,
  MatchSuggestion,
} from '@/lib/types/extraction'
import type { StagedCollectionItem } from './AddItemsPicker'

const MAX_IMAGE_SIZE = 10 * 1024 * 1024 // 10MB
const SUPPORTED_IMAGE_TYPES = [
  'image/jpeg',
  'image/png',
  'image/gif',
  'image/webp',
  // iOS Safari HEIC paste/upload — browser-native decode converts
  // HEIC → JPEG in the canvas step below. Non-Safari fall-through is
  // graceful (inline error). heic2any polyfill is a follow-up.
  'image/heic',
  'image/heif',
]
// File input `accept` attribute. Lists extensions in addition to MIME
// types so iOS Safari's photo picker shows .heic files (Safari sometimes
// reports an empty `file.type` for clipboard-pasted HEIC).
const FILE_INPUT_ACCEPT =
  'image/jpeg,image/png,image/gif,image/webp,image/heic,image/heif,.heic,.heif'

// 1500px keeps the base64 payload comfortably under Vercel's 4.5MB body
// limit at canvas.toDataURL('image/jpeg', 0.8). Mirrors AIFormFiller.
const MAX_IMAGE_DIMENSION = 1500
const JPEG_QUALITY = 0.8

function compressImage(dataUrl: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => {
      let { width, height } = img
      if (width > MAX_IMAGE_DIMENSION || height > MAX_IMAGE_DIMENSION) {
        const scale = MAX_IMAGE_DIMENSION / Math.max(width, height)
        width = Math.round(width * scale)
        height = Math.round(height * scale)
      }
      const canvas = document.createElement('canvas')
      canvas.width = width
      canvas.height = height
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        reject(new Error('Could not get canvas context'))
        return
      }
      ctx.drawImage(img, 0, 0, width, height)
      resolve(canvas.toDataURL('image/jpeg', JPEG_QUALITY))
    }
    img.onerror = () =>
      reject(
        new Error(
          'Failed to decode image. HEIC images upload best from iOS Safari; on other browsers, convert to JPEG first.'
        )
      )
    img.src = dataUrl
  })
}

// ─── Tier-gated create / queue policy (PSY-853) ───
//
// Unmatched rows have no existing entity to stage. What the user can do with
// them depends on trust tier. The backend's tier policy lives in PSY-869's
// EntityRequestService (autoApproves): admin / local_ambassador auto-approve;
// trusted_contributor auto-approves only on a FE-confirmed request; everyone
// else queues for admin review (feedback_human_verify_ai_entity_data — no
// autonomous AI entity creation for low-trust tiers). We mirror that policy
// in the UI but DO NOT re-implement authorization client-side — the backend
// enforces it; this only picks the right affordance.
//
// IMPORTANT (architectural constraint, verified against backend on PSY-853):
// every catalog create endpoint (POST /admin/artists, /releases, …) is
// rc.Admin-gated, so non-admin trusted tiers CANNOT create entities through
// them — a 403 path. The ONLY cross-tier create mechanism is
// POST /entity-requests (PSY-997). On auto-approve that endpoint marks the
// request approved but does NOT fulfill it into a catalog row, so there is no
// new entity_id to stage into the bulk-add pipeline yet. The "Submit for creation"
// label therefore files an (auto-approved) request rather than staging the
// new entity into the collection in the same step. Closing that gap — fulfill
// on auto-approve + return created_entity_id so the row can stage — is a
// backend follow-up; it is out of this frontend-only ticket's scope.
type CreateAffordance = 'create' | 'confirm' | 'queue' | 'none'

/**
 * Maps an authenticated user's admin flag + trust tier to the create
 * affordance shown on an unmatched row. Returns 'none' for anonymous or
 * unknown-tier users (fail-closed — never offer a create action we can't
 * map to a backend-enforced policy).
 */
function createAffordanceFor(
  isAdmin: boolean | undefined,
  tier: string | undefined
): CreateAffordance {
  if (isAdmin) return 'create'
  switch (tier) {
    case 'local_ambassador':
      return 'create'
    case 'trusted_contributor':
      return 'confirm'
    case 'contributor':
    case 'new_user':
      return 'queue'
    default:
      // Anonymous, missing, or an unrecognized tier → no create action.
      return 'none'
  }
}

// Outcome of a successful entity-request POST, used to pick the per-row chip.
// 'requested' = auto-approved (admin / local_ambassador / confirmed trusted) —
// the request skipped the queue; 'queued' = pending admin review.
type RequestOutcome = 'requested' | 'queued'

interface QueueEntityRequestVars {
  /** The unmatched row this request was filed from (used for per-row state). */
  rowKey: string
  /** Always 'artist' in V1 — extraction only matches/creates artists today. */
  entityType: 'artist'
  /** User-supplied creation payload (artist name from the extracted row). */
  name: string
  /**
   * FE-side confirm step. Only meaningful for trusted_contributor (the backend
   * auto-approves a confirmed trusted request); ignored for other tiers.
   */
  confirmed: boolean
}

/**
 * Local queue-create mutation (PSY-853). Intentionally NOT a shared exported
 * hook — PSY-845 posts to the same POST /entity-requests endpoint from a
 * different component (AddItemsPicker) and a small, deliberate duplication is
 * preferred over premature coupling while both land in parallel. A follow-up
 * dedups once both have shipped (see coordination note in the ticket).
 *
 * The body carries source_context: 'ai_extraction' so the admin moderation
 * surface can see the request originated from the AI collection flow.
 */
function useQueueEntityRequest() {
  return useMutation({
    mutationFn: async (vars: QueueEntityRequestVars) => {
      const response = await fetch('/api/entity-requests', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          entity_type: vars.entityType,
          payload: { name: vars.name },
          source_context: 'ai_extraction',
          confirmed: vars.confirmed,
        }),
      })

      const data = await response.json().catch(() => null)

      if (!response.ok) {
        throw new Error(
          (data && (data.detail || data.message)) ||
            'Failed to submit entity request'
        )
      }

      // decision_state distinguishes auto-approved (skips the queue) from
      // pending (awaiting admin review) so the row chip reads correctly.
      const outcome: RequestOutcome =
        data?.decision_state === 'approved' ? 'requested' : 'queued'
      return { outcome, rowKey: vars.rowKey }
    },
  })
}

export interface AICollectionFillerProps {
  /**
   * Fires when the user stages one or more extracted items via the per-row
   * Add buttons (or "Add all matched"). Parent (AddItemsPicker) routes the
   * call through its onStageBatch so the staged list updates in a single
   * React setState (avoiding the same setState-batching race the regular
   * paste-mode addAll path guards against).
   */
  onStageItems: (items: StagedCollectionItem[]) => void
  /**
   * Predicate: is this entity already staged (or already in the
   * collection)? Used to render the "Added" chip per row.
   */
  alreadyStaged: (entityType: 'artist', entityId: number) => boolean
}

export function AICollectionFiller({
  onStageItems,
  alreadyStaged,
}: AICollectionFillerProps) {
  const [textInput, setTextInput] = useState('')
  const [imageFile, setImageFile] = useState<File | null>(null)
  const [imagePreview, setImagePreview] = useState<string | null>(null)
  const [imageError, setImageError] = useState<string | null>(null)
  const [extractionResult, setExtractionResult] =
    useState<ExtractedCollectionData | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])
  // Per-row outcome of a filed entity-request, keyed by the row's stable key.
  // Drives the "Requested" / "Queued" chip and hides the create affordance
  // once a row has been acted on (prevents double-filing).
  const [requestedRows, setRequestedRows] = useState<
    Record<string, RequestOutcome>
  >({})
  // Per-row error message for a failed entity-request POST (403 / 422 / 5xx /
  // network). Surfaced inline on the row so the action isn't a silent no-op;
  // the create/queue button stays so the user can retry.
  const [requestErrors, setRequestErrors] = useState<Record<string, string>>({})
  // Rows with an entity-request currently in flight. Tracked per-row (not via
  // the shared mutation's isPending) because the single useMutation only
  // remembers its LATEST variables — clicking row B while row A is in flight
  // would otherwise re-enable A's button mid-flight and let it double-file
  // (the backend has no create-side dedup).
  const [inFlightRows, setInFlightRows] = useState<Set<string>>(new Set())
  const fileInputRef = useRef<HTMLInputElement>(null)

  const { user } = useAuthContext()
  const affordance = createAffordanceFor(user?.is_admin, user?.user_tier)
  const queueRequest = useQueueEntityRequest()

  const { mutate, isPending, error, reset } = useCollectionExtraction()

  const handleTextChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setTextInput(e.target.value)
    // Note: extractionResult is intentionally preserved across keystrokes.
    // Each row can take multiple manual interactions (Pick suggestions,
    // Skip choices) — wiping the result on every character would force
    // the user to redo all those interactions if they ever go back to
    // touch the textarea. Clear via clearImage or a fresh Extract.
    setImageError(null)
    reset()
  }

  const handleImageSelect = useCallback(
    (file: File) => {
      // iOS Safari sometimes drops `file.type` on clipboard-pasted HEIC —
      // fall back to extension sniffing so the supported-types gate doesn't
      // erroneously reject them.
      const ext = file.name.toLowerCase().split('.').pop() || ''
      const inferredType =
        ext === 'heic' ? 'image/heic' : ext === 'heif' ? 'image/heif' : file.type

      if (!SUPPORTED_IMAGE_TYPES.includes(inferredType)) {
        setImageError(
          `Unsupported image type. Please use: JPEG, PNG, GIF, WebP, or HEIC.`
        )
        return
      }
      if (file.size > MAX_IMAGE_SIZE) {
        setImageError('Image is too large. Maximum size is 10MB.')
        return
      }

      setImageFile(file)
      // Clear the prior preview eagerly — keeping the old preview while
      // a new file is being compressed would mismatch the displayed image
      // with the file the Extract button would actually send.
      setImagePreview(null)
      setImageError(null)
      setExtractionResult(null)
      setWarnings([])
      reset()

      const reader = new FileReader()
      reader.onload = async e => {
        const rawResult = e.target?.result
        if (typeof rawResult !== 'string') {
          setImageError('Failed to read image file.')
          setImageFile(null)
          return
        }
        try {
          const compressed = await compressImage(rawResult)
          setImagePreview(compressed)
        } catch (err) {
          setImageError(
            err instanceof Error ? err.message : 'Failed to read image.'
          )
          // Clear both file + preview on decode failure so the dropzone
          // returns and the user can pick a different image without a
          // stale preview visible.
          setImageFile(null)
          setImagePreview(null)
        }
      }
      reader.onerror = () => {
        setImageError('Failed to read image file.')
        setImageFile(null)
        setImagePreview(null)
      }
      reader.readAsDataURL(file)
    },
    [reset]
  )

  const handleFileInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) handleImageSelect(file)
  }

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      e.stopPropagation()
      const file = e.dataTransfer.files[0]
      if (file) handleImageSelect(file)
    },
    [handleImageSelect]
  )

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
  }

  const clearImage = () => {
    setImageFile(null)
    setImagePreview(null)
    setImageError(null)
    setExtractionResult(null)
    setWarnings([])
    reset()
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  const handleExtract = async () => {
    const hasText = textInput.trim().length > 0
    const hasImage = imageFile !== null && imagePreview !== null
    if (!hasText && !hasImage) return

    const onSuccess = (response: { data?: ExtractedCollectionData; warnings?: string[] }) => {
      if (response.data) {
        setExtractionResult(response.data)
        setWarnings(response.warnings || [])
        // A fresh extraction replaces the row matrix, so any prior
        // Requested/Queued chips + row errors no longer correspond to a
        // visible row.
        setRequestedRows({})
        setRequestErrors({})
      }
    }

    if (hasImage) {
      const base64Data = imagePreview!.split(',')[1]
      mutate(
        {
          type: hasText ? 'both' : 'image',
          image_data: base64Data,
          media_type: 'image/jpeg',
          text: hasText ? textInput : undefined,
        },
        { onSuccess }
      )
    } else {
      mutate({ type: 'text', text: textInput }, { onSuccess })
    }
  }

  // Gate Extract on the image being fully compressed (imagePreview set),
  // not just the file being picked — there's an async window after
  // setImageFile() before FileReader.onload's compressImage completes
  // where clicking would silently drop the image and either no-op (no
  // text) or send text-only (image silently discarded). Both are bad
  // UX; require the image to be ready before allowing extract.
  const imageReady = imageFile === null || imagePreview !== null
  const canExtract =
    (textInput.trim().length > 0 || imageFile !== null) && imageReady

  // ─── Per-row actions ───
  // Subtitle uses the raw release title — StagedRow inserts its own
  // " — " separator between name and subtitle, so prepending one here
  // would render a doubled em-dash ("Frank Ocean — — Blonde").
  const stageItem = (item: ExtractedCollectionItem) => {
    if (!item.matched_artist_id) return
    onStageItems([
      {
        entityType: 'artist',
        entityId: item.matched_artist_id,
        name: item.matched_artist_name ?? item.artist_name,
        subtitle: item.release_title ?? null,
      },
    ])
  }

  // Dedup within the batch too — canon lists ("100 Best Albums") often
  // contain multiple releases by the same artist, all of which match to
  // one artist_id. Without per-batch dedup, the staged list would emit
  // duplicate React keys and the backend's UNIQUE(collection_id,
  // entity_type, entity_id) constraint would silently keep only one.
  const stageAllMatched = () => {
    if (!extractionResult) return
    const seen = new Set<number>()
    const toStage: StagedCollectionItem[] = []
    for (const i of extractionResult.items) {
      if (!i.matched_artist_id) continue
      if (alreadyStaged('artist', i.matched_artist_id)) continue
      if (seen.has(i.matched_artist_id)) continue
      seen.add(i.matched_artist_id)
      toStage.push({
        entityType: 'artist',
        entityId: i.matched_artist_id,
        name: i.matched_artist_name ?? i.artist_name,
        subtitle: i.release_title ?? null,
      })
    }
    if (toStage.length === 0) return
    onStageItems(toStage)
  }

  const acceptSuggestion = (itemIndex: number, suggestion: MatchSuggestion) => {
    if (!extractionResult) return
    const updatedItems = extractionResult.items.map((item, i) => {
      if (i !== itemIndex) return item
      return {
        ...item,
        matched_artist_id: suggestion.id,
        matched_artist_name: suggestion.name,
        matched_artist_slug: suggestion.slug,
        artist_suggestions: undefined,
      }
    })
    setExtractionResult({ ...extractionResult, items: updatedItems })
  }

  const dismissSuggestions = (itemIndex: number) => {
    if (!extractionResult) return
    const updatedItems = extractionResult.items.map((item, i) => {
      if (i !== itemIndex) return item
      return { ...item, artist_suggestions: undefined }
    })
    setExtractionResult({ ...extractionResult, items: updatedItems })
  }

  // Files an entity-request for an unmatched row. Tier policy is enforced
  // server-side; `confirmed` is only meaningful for trusted_contributor.
  // On success the row flips to a Requested/Queued chip (set from the
  // backend's decision_state, not assumed) so the affordance can't be
  // double-fired.
  const requestRow = (rowKey: string, name: string, confirmed: boolean) => {
    // Guard against a double-submit for the same row (the resolved/errored
    // chip already guards a settled row; this guards the in-flight window).
    if (inFlightRows.has(rowKey)) return
    // Clear any prior error on this row so a retry starts clean.
    setRequestErrors(prev => {
      if (!(rowKey in prev)) return prev
      const next = { ...prev }
      delete next[rowKey]
      return next
    })
    setInFlightRows(prev => new Set(prev).add(rowKey))
    const clearInFlight = () =>
      setInFlightRows(prev => {
        const next = new Set(prev)
        next.delete(rowKey)
        return next
      })
    queueRequest.mutate(
      { rowKey, entityType: 'artist', name, confirmed },
      {
        onSuccess: ({ outcome }) => {
          setRequestedRows(prev => ({ ...prev, [rowKey]: outcome }))
          clearInFlight()
        },
        onError: (err: unknown) => {
          setRequestErrors(prev => ({
            ...prev,
            [rowKey]:
              err instanceof Error
                ? err.message
                : 'Failed to submit. Please try again.',
          }))
          clearInFlight()
        },
      }
    )
  }

  const matchedReadyCount = extractionResult
    ? extractionResult.items.filter(
        i =>
          i.matched_artist_id &&
          !alreadyStaged('artist', i.matched_artist_id)
      ).length
    : 0

  return (
    <div className="mt-3 space-y-3" data-testid="ai-collection-filler">
      {/* Image drop zone */}
      {imagePreview ? (
        <div className="relative">
          <img
            src={imagePreview}
            alt="Uploaded article screenshot"
            className="max-h-48 rounded-md border border-input object-contain mx-auto"
          />
          <Button
            variant="ghost"
            size="icon"
            className="absolute top-2 right-2 h-7 w-7 bg-background/80 hover:bg-background"
            onClick={clearImage}
            disabled={isPending}
            aria-label="Remove uploaded image"
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
      ) : (
        <div
          className="border-2 border-dashed border-input rounded-md p-4 text-center hover:border-primary/50 transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onDrop={handleDrop}
          onDragOver={handleDragOver}
          onClick={() => fileInputRef.current?.click()}
          role="button"
          tabIndex={0}
          onKeyDown={e => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              fileInputRef.current?.click()
            }
          }}
          aria-label="Upload an article screenshot"
        >
          <div className="flex flex-col sm:flex-row items-center justify-center gap-2 sm:gap-3">
            <div className="flex items-center justify-center h-10 w-10 rounded-full bg-muted">
              <ImageIcon className="h-5 w-5 text-muted-foreground" />
            </div>
            <div className="text-center sm:text-left">
              <p className="text-sm text-muted-foreground">
                <span className="hidden sm:inline">Drop a screenshot here, or </span>
                <span className="sm:hidden">Tap to </span>
                <span className="text-primary">
                  <span className="hidden sm:inline">click to select</span>
                  <span className="sm:hidden">upload a screenshot</span>
                </span>
              </p>
              <p className="text-xs text-muted-foreground">
                JPEG, PNG, GIF, WebP, HEIC (max 10MB)
              </p>
            </div>
          </div>
          <input
            ref={fileInputRef}
            type="file"
            accept={FILE_INPUT_ACCEPT}
            className="hidden"
            onChange={handleFileInputChange}
            disabled={isPending}
            data-testid="ai-collection-filler-file-input"
          />
        </div>
      )}

      {imageError && (
        <InlineErrorBanner testId="ai-collection-filler-image-error">
          {imageError}
        </InlineErrorBanner>
      )}

      {/* Text input */}
      <textarea
        className="w-full min-h-[160px] rounded-md border border-input bg-background px-3 py-2 text-sm font-mono ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 resize-y"
        placeholder={
          imagePreview
            ? 'Add any details the image might be missing (list name, ranking notes, etc.)…'
            : 'Paste the article text. Best results from canon-list articles (Pitchfork best-of, AOTY top-N, weekly digests).\n\nExample:\nPitchfork — The 200 Best Albums of the 2010s\n1. Kendrick Lamar — To Pimp a Butterfly\n2. Frank Ocean — Blonde\n3. Boris — Pink\n…'
        }
        value={textInput}
        onChange={handleTextChange}
        disabled={isPending}
        data-testid="ai-collection-filler-textarea"
      />

      <Button
        onClick={handleExtract}
        disabled={!canExtract || isPending}
        className="w-full"
        data-testid="ai-collection-filler-extract"
      >
        {isPending ? (
          <>
            <Loader2 className="h-4 w-4 animate-spin mr-2" />
            Extracting…
          </>
        ) : (
          <>
            <Sparkles className="h-4 w-4 mr-2" />
            Extract items
          </>
        )}
      </Button>

      {error && (
        <InlineErrorBanner testId="ai-collection-filler-error">
          {error.message}
        </InlineErrorBanner>
      )}

      {extractionResult && (
        <div
          className="rounded-md bg-primary/5 border border-primary/20 p-3 space-y-3"
          data-testid="ai-collection-filler-result"
        >
          <div className="flex items-center justify-between gap-2 flex-wrap">
            <div className="flex items-center gap-2 text-sm font-medium text-primary">
              <CheckCircle2 className="h-4 w-4" />
              {extractionResult.source ?? 'Extraction complete'}
            </div>
            {matchedReadyCount > 0 && (
              <Button
                variant="outline"
                size="sm"
                onClick={stageAllMatched}
                data-testid="ai-collection-filler-add-all-matched"
              >
                <Plus className="h-3.5 w-3.5 mr-1" />
                Add all {matchedReadyCount} matched
              </Button>
            )}
          </div>

          {extractionResult.description && (
            <p className="text-xs text-muted-foreground italic">
              {extractionResult.description}
            </p>
          )}

          {extractionResult.items.length === 0 ? (
            <p className="text-sm text-muted-foreground py-2 text-center">
              No items were extracted. Try richer source text.
            </p>
          ) : (
            <div className="max-h-72 overflow-y-auto space-y-1">
              {extractionResult.items.map((item, idx) => {
                const rowKey = `${idx}-${item.artist_name}`
                return (
                  <ExtractedRow
                    key={rowKey}
                    item={item}
                    alreadyStaged={
                      item.matched_artist_id
                        ? alreadyStaged('artist', item.matched_artist_id)
                        : false
                    }
                    affordance={affordance}
                    requestOutcome={requestedRows[rowKey]}
                    requestError={requestErrors[rowKey]}
                    isRequesting={inFlightRows.has(rowKey)}
                    onAdd={() => stageItem(item)}
                    onAcceptSuggestion={s => acceptSuggestion(idx, s)}
                    onDismissSuggestions={() => dismissSuggestions(idx)}
                    onRequest={confirmed =>
                      requestRow(rowKey, item.artist_name, confirmed)
                    }
                  />
                )
              })}
            </div>
          )}

          {warnings.length > 0 && (
            <div className="text-xs text-pending-foreground space-y-0.5">
              {warnings.map((warning, i) => (
                <div key={i}>{warning}</div>
              ))}
            </div>
          )}

          <p className="text-xs text-muted-foreground">
            {affordance === 'queue'
              ? 'Unmatched rows show suggestions for the closest existing entity. New artists you queue go to a moderator for review before they’re created.'
              : 'Unmatched rows show suggestions for the closest existing entity. New artists you add are submitted for creation.'}
          </p>
        </div>
      )}
    </div>
  )
}

function ExtractedRow({
  item,
  alreadyStaged,
  affordance,
  requestOutcome,
  requestError,
  isRequesting,
  onAdd,
  onAcceptSuggestion,
  onDismissSuggestions,
  onRequest,
}: {
  item: ExtractedCollectionItem
  alreadyStaged: boolean
  /** Tier-derived create affordance for unmatched rows. */
  affordance: CreateAffordance
  /** Set once this row's entity-request resolves (Requested vs Queued chip). */
  requestOutcome: RequestOutcome | undefined
  /** Set if this row's entity-request POST failed (inline error). */
  requestError: string | undefined
  /** True while THIS row's request is in flight. */
  isRequesting: boolean
  onAdd: () => void
  onAcceptSuggestion: (suggestion: MatchSuggestion) => void
  onDismissSuggestions: () => void
  /** Files the entity-request. `confirmed` is the trusted_contributor step. */
  onRequest: (confirmed: boolean) => void
}) {
  // trusted_contributor confirm step is INLINE (not a Dialog) so the picker
  // context stays visible — entity creation is irreversible, so the extra
  // tap is a deliberate friction point, not a modal interruption.
  const [confirming, setConfirming] = useState(false)
  const displayName = item.matched_artist_name ?? item.artist_name
  const hasSuggestions =
    !item.matched_artist_id && (item.artist_suggestions?.length ?? 0) > 0
  const isNew = !item.matched_artist_id && !hasSuggestions

  return (
    <div
      className="rounded-md p-2 hover:bg-muted/50"
      data-testid="ai-collection-filler-row"
    >
      <div className="flex items-center gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm font-medium truncate">{displayName}</span>
            {item.matched_artist_id && (
              <Badge
                variant="secondary"
                className="text-[10px] px-1.5 py-0 shrink-0 bg-success text-success-foreground motion-safe:animate-in motion-safe:fade-in"
              >
                <CheckCircle2 className="h-3 w-3 mr-0.5" />
                MATCH
              </Badge>
            )}
            {hasSuggestions && (
              <Badge
                variant="secondary"
                className="text-[10px] px-1.5 py-0 shrink-0 bg-pending text-pending-foreground motion-safe:animate-in motion-safe:fade-in"
              >
                <AlertTriangle className="h-3 w-3 mr-0.5" />
                PICK
              </Badge>
            )}
            {/* NEW uses the SOFT destructive tint (bg-destructive/10), not the
                solid bg-destructive action token MATCH/PICK-style: "will be
                created" is an advisory outcome, not an error/failure. The DS has
                no soft destructive *surface* token, so this uses /10; success +
                pending ARE soft surface tokens, so MATCH/PICK apply them directly. */}
            {isNew && (
              <Badge
                variant="secondary"
                className="text-[10px] px-1.5 py-0 shrink-0 bg-destructive/10 text-destructive motion-safe:animate-in motion-safe:fade-in"
              >
                <AlertCircle className="h-3 w-3 mr-0.5" />
                NEW
              </Badge>
            )}
          </div>
          {item.release_title && (
            <p className="text-xs text-muted-foreground truncate">
              {item.release_title}
            </p>
          )}
        </div>
        {item.matched_artist_id &&
          (alreadyStaged ? (
            <Badge variant="secondary" className="text-xs shrink-0">
              Added
            </Badge>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 shrink-0"
              onClick={onAdd}
              data-testid="ai-collection-filler-row-add"
            >
              <Plus className="h-3.5 w-3.5 mr-1" />
              Add
            </Button>
          ))}

        {/* Tier-gated create / queue affordance for unmatched (NEW) rows.
            Suggestion (PICK) rows resolve via "Did you mean" below, not here.
            Authorization is enforced server-side by POST /entity-requests; this
            only selects the affordance and never bypasses the queue for
            low-trust tiers (a contributor only ever sees [Queue for review]). */}
        {isNew &&
          (requestOutcome ? (
            <Badge
              variant="secondary"
              className="text-xs shrink-0 motion-safe:animate-in motion-safe:fade-in"
              data-testid="ai-collection-filler-row-request-chip"
            >
              {requestOutcome === 'queued' ? 'Queued' : 'Requested'}
            </Badge>
          ) : affordance === 'none' ? null : confirming ? (
            // trusted_contributor inline confirm — irreversible creation.
            <div className="flex items-center gap-1 shrink-0">
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-destructive"
                disabled={isRequesting}
                onClick={() => {
                  setConfirming(false)
                  onRequest(true)
                }}
                data-testid="ai-collection-filler-row-confirm"
              >
                {isRequesting ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  'Confirm'
                )}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-muted-foreground"
                disabled={isRequesting}
                onClick={() => setConfirming(false)}
                data-testid="ai-collection-filler-row-cancel"
              >
                Cancel
              </Button>
            </div>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 shrink-0"
              disabled={isRequesting}
              onClick={() => {
                if (affordance === 'confirm') {
                  setConfirming(true)
                } else {
                  // admin / local_ambassador (create) and contributor /
                  // new_user (queue) both file immediately; the backend's
                  // tier policy decides auto-approve vs pending.
                  onRequest(false)
                }
              }}
              data-testid="ai-collection-filler-row-request"
            >
              {isRequesting ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" />
              ) : (
                <Plus className="h-3.5 w-3.5 mr-1" />
              )}
              {affordance === 'queue' ? 'Queue for review' : 'Submit for creation'}
            </Button>
          ))}
      </div>

      {hasSuggestions && (
        <div className="ml-0 mt-1.5 flex items-center gap-1.5 flex-wrap text-xs">
          <span className="text-pending-foreground">
            Did you mean:
          </span>
          {item.artist_suggestions!.map(s => (
            <button
              key={s.id}
              type="button"
              className="rounded-md border border-pending-foreground/20 bg-pending px-2 py-0.5 text-xs text-pending-foreground hover:bg-pending/80 transition-colors"
              onClick={() => onAcceptSuggestion(s)}
              data-testid="ai-collection-filler-row-pick"
            >
              {s.name}
            </button>
          ))}
          <button
            type="button"
            className="rounded-md border border-input px-2 py-0.5 text-xs text-muted-foreground hover:bg-muted transition-colors"
            onClick={onDismissSuggestions}
          >
            Skip
          </button>
        </div>
      )}

      {/* Inline error for a failed create/queue POST — the button stays so the
          user can retry; never a silent no-op. */}
      {requestError && (
        <p
          className="ml-0 mt-1.5 text-xs text-destructive"
          data-testid="ai-collection-filler-row-request-error"
        >
          {requestError}
        </p>
      )}
    </div>
  )
}
