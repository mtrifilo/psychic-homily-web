'use client'

import { useMemo, useState, type FormEvent } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { X } from 'lucide-react'
// MarkdownEditor is lazily loaded (dynamic ssr:false) so its `marked` +
// `dompurify` deps stay out of the global shared client chunk — see
// MarkdownEditorLazy / PSY-951.
import { MarkdownEditor } from './MarkdownEditorLazy'
import { AddItemsPicker, type StagedCollectionItem } from './AddItemsPicker'
import {
  MAX_COLLECTION_MARKDOWN_LENGTH,
  MAX_COVER_IMAGE_URL_LENGTH,
  validateCoverImageUrl,
} from '../types'
import {
  useCreateCollection,
  useBulkAddCollectionItems,
  useMyCollections,
} from '../hooks'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  COLLECTION_UNLIMITED,
  TIERS_HELP_PATH,
  getCollectionLimitForTier,
  getTierInfo,
} from '@/lib/tiers'
import { cn } from '@/lib/utils'

/**
 * The create-collection form. Extracted from CollectionList (PSY-961) so it can
 * be mounted both by the /collections "Create Collection" button and by the
 * app-level CreateCollectionDrawer (reachable from the AddToCollectionButton
 * popover's "Create … with {entity}" CTA). `initialStagedItems` seeds the
 * staged list — the create-from-entity flow passes the current entity as item 1.
 */
export function CreateCollectionForm({
  onSuccess,
  onCancel,
  initialStagedItems,
}: {
  onSuccess: (slug?: string) => void
  /** PSY-823: cancel affordance for the Sheet footer. Optional so legacy
   *  Dialog callers can still mount the form without a cancel button. */
  onCancel?: () => void
  /** PSY-961: pre-seed the staged list (create-from-entity passes the current
   *  entity as item 1). The form is re-mounted per drawer-open, so this only
   *  needs to be read on mount. */
  initialStagedItems?: StagedCollectionItem[]
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
  // the bulk endpoint is keyed on the new collection's slug. PSY-961:
  // pre-seeded from `initialStagedItems` for the create-from-entity flow.
  const [stagedItems, setStagedItems] = useState<StagedCollectionItem[]>(
    initialStagedItems ?? []
  )
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

  const handleSubmit = async (e: FormEvent) => {
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
          onChange={(e) => setTitle(e.target.value)}
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
            onChange={(e) => {
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
            onChange={(e) => setIsPublic(e.target.checked)}
            className="rounded border-border"
          />
          Public
        </label>

        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={collaborative}
            onChange={(e) => setCollaborative(e.target.checked)}
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
