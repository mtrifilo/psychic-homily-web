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
  const fileInputRef = useRef<HTMLInputElement>(null)

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
              {extractionResult.items.map((item, idx) => (
                <ExtractedRow
                  key={`${idx}-${item.artist_name}`}
                  item={item}
                  alreadyStaged={
                    item.matched_artist_id
                      ? alreadyStaged('artist', item.matched_artist_id)
                      : false
                  }
                  onAdd={() => stageItem(item)}
                  onAcceptSuggestion={s => acceptSuggestion(idx, s)}
                  onDismissSuggestions={() => dismissSuggestions(idx)}
                />
              ))}
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
            Unmatched rows show suggestions for the closest existing entity.
            Creating new artists / releases via the AI flow lands in a
            follow-up — for V1 only matched (or picked) rows commit.
          </p>
        </div>
      )}
    </div>
  )
}

function ExtractedRow({
  item,
  alreadyStaged,
  onAdd,
  onAcceptSuggestion,
  onDismissSuggestions,
}: {
  item: ExtractedCollectionItem
  alreadyStaged: boolean
  onAdd: () => void
  onAcceptSuggestion: (suggestion: MatchSuggestion) => void
  onDismissSuggestions: () => void
}) {
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
                className="text-[10px] px-1.5 py-0 shrink-0 bg-success text-success-foreground"
              >
                <CheckCircle2 className="h-3 w-3 mr-0.5" />
                MATCH
              </Badge>
            )}
            {hasSuggestions && (
              <Badge
                variant="secondary"
                className="text-[10px] px-1.5 py-0 shrink-0 bg-pending text-pending-foreground"
              >
                <AlertTriangle className="h-3 w-3 mr-0.5" />
                PICK
              </Badge>
            )}
            {isNew && (
              <Badge
                variant="secondary"
                className="text-[10px] px-1.5 py-0 shrink-0 bg-destructive/10 text-destructive"
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
              className="rounded-md border border-pending bg-pending px-2 py-0.5 text-xs text-pending-foreground hover:bg-pending/80 transition-colors"
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
    </div>
  )
}
