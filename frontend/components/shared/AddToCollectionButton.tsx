'use client'

import { useId, useMemo, useRef, useState, type ReactNode } from 'react'
import Link from 'next/link'
import { useRouter, usePathname } from 'next/navigation'
import { useQueryClient } from '@tanstack/react-query'
import { Library, Check, Plus, Loader2, AlertCircle, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from './BracketLink'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { CollectionCoverImage } from '@/features/collections/components/CollectionCoverImage'
import {
  useMyCollections,
  useAddCollectionItem,
  useRemoveCollectionItem,
  useUserCollectionsContaining,
} from '@/features/collections/hooks'
import { queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { CollectionEntityType } from '@/features/collections/types'
import {
  readCollectionAddRecency,
  recordCollectionAdd,
} from '@/features/collections/collectionAddRecency'

interface AddToCollectionButtonProps {
  entityType: CollectionEntityType
  entityId: number
  entityName: string
  /**
   * Trigger style. `default`/`ghost`/`outline` render a shadcn Button.
   * `bracket` renders a `<BracketLink>` for dense entity-page header
   * linkboxes (PSY-641) — `[Add to collection]`.
   */
  variant?: 'default' | 'ghost' | 'outline' | 'bracket'
  size?: 'sm' | 'default' | 'icon'
}

type SubmitError = { collectionId: number; message: string }

// Sentinel for the adjust-during-render contains-sync below: a value
// guaranteed distinct from any real `containing` Map, so the guard also fires
// on the FIRST render (the prior effect always ran on mount and seeded).
const UNSET = Symbol('unset')

// PSY-829: the client-side filter input only earns its space once the list is
// long enough to scroll. Below this count the rows fit at a glance, so the
// input would be noise. This matches the locked PSY-893 design, whose
// default-open mock (4 rows) has NO search box while its "search active" mock
// depicts a larger library — i.e. the input is conditional, not always-on. The
// exact cutoff is a judgment call (the design doesn't specify a number).
const SEARCH_THRESHOLD = 8

// PSY-960 / PSY-893 D3: the popover promotes up to this many recently-used
// collections above a separator ("up to 5" per the locked design; tunable 3–5).
const MAX_PROMOTED = 5

// Grouping (RECENTLY USED / ALL COLLECTIONS) only earns its visual overhead
// once the library is large enough that surfacing recents saves scanning. The
// locked PSY-893 design draws the default-open mock (4 collections) FLAT and
// the recently-used mock (5 collections) GROUPED — so the cutoff is 5.
const RECENTLY_USED_MIN_COLLECTIONS = 5

function describeError(reason: unknown): string {
  if (reason instanceof Error && reason.message) return reason.message
  if (typeof reason === 'string' && reason.length > 0) return reason
  return 'Failed to add to this collection'
}

/** Uppercase section label for the RECENTLY USED / ALL COLLECTIONS groups.
 *  `id` lets each section's group reference it via aria-labelledby. */
function SectionLabel({ id, children }: { id?: string; children: ReactNode }) {
  return (
    <p
      id={id}
      className="px-2 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground"
    >
      {children}
    </p>
  )
}

export function AddToCollectionButton({
  entityType,
  entityId,
  entityName,
  variant = 'ghost',
  size = 'sm',
}: AddToCollectionButtonProps) {
  // Every hook below this comment must be called UNCONDITIONALLY, BEFORE
  // any early return. Placing `useState` after the `!isAuthenticated`
  // early return triggered a Rules-of-Hooks violation once the auth
  // profile resolved (PSY-466).
  const { isAuthenticated } = useAuthContext()
  const router = useRouter()
  const pathname = usePathname()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)

  // The contains query is gated on `open` so we don't fetch on every
  // entity page render — only when the user actually opens the popover.
  const { data: myCollectionsData, isLoading: collectionsLoading } =
    useMyCollections()
  // collectionId → collection_item id (PSY-829): the Map keys drive the
  // pre-check; the values give the item id to DELETE on uncheck→remove.
  const { data: containing, isLoading: containingLoading } =
    useUserCollectionsContaining(entityType, entityId, { enabled: open })
  const addMutation = useAddCollectionItem()
  const removeMutation = useRemoveCollectionItem()

  // `pendingAdds` — rows the user has checked that are NOT yet on the server
  // (the Submit batch). `savedIds` — the server's last-known truth (seeded
  // from `containing`, plus session adds, minus session removes). A row is
  // checked when it's in either set; "✓ Added" shows for `savedIds`.
  const [pendingAdds, setPendingAdds] = useState<Set<number>>(new Set())
  const [savedIds, setSavedIds] = useState<Set<number>>(new Set())
  // collectionId → collection_item id captured from this session's adds. The
  // contains query refetch (triggered by addMutation) eventually supplies the
  // same ids, but until it lands `containing` has no entry for a just-added
  // row — this lets a same-session uncheck→remove of that row still DELETE
  // the correct item instead of silently no-op'ing.
  const [sessionItemIds, setSessionItemIds] = useState<Map<number, number>>(
    new Map()
  )
  const [submitErrors, setSubmitErrors] = useState<SubmitError[]>([])
  const [submitting, setSubmitting] = useState(false)
  const [search, setSearch] = useState('')
  // D1 remove-with-confirm: the already-in row currently showing its inline
  // "Remove from this collection?" confirm, the in-flight removal, and any
  // remove error to surface on that row.
  const [removeConfirmId, setRemoveConfirmId] = useState<number | null>(null)
  const [removingId, setRemovingId] = useState<number | null>(null)
  const [removeError, setRemoveError] = useState<SubmitError | null>(null)
  // Synchronous in-flight guards: the `submitting`/`removingId` state disables
  // the buttons, but that only takes effect after React commits — two clicks in
  // the same tick both read the stale state and fire duplicate mutations. Refs
  // flip synchronously so the second click bails immediately.
  const submitInFlightRef = useRef(false)
  const removeInFlightRef = useRef(false)

  // Seed `savedIds` from the server's contains-truth whenever the query
  // resolves (or returns a fresh reference — e.g. after an add/remove
  // invalidation). React 19.2: adjust state during render via the canonical
  // previous-value-guard idiom instead of a cascading effect. The tracker
  // starts at a sentinel so the guard also fires on the FIRST render when
  // `containing` is already resolved (matching the prior effect, which always
  // ran on mount). Pending adds not yet saved are PRESERVED across the
  // re-seed (a containing refetch triggered by removing one row must not wipe
  // a different row the user just checked but hasn't submitted).
  const [prevContaining, setPrevContaining] = useState<
    typeof containing | typeof UNSET
  >(UNSET)
  if (containing !== prevContaining) {
    setPrevContaining(containing)
    if (containing) {
      const serverKeys = new Set(containing.keys())
      setSavedIds(serverKeys)
      setPendingAdds((prev) => {
        const next = new Set<number>()
        for (const id of prev) if (!serverKeys.has(id)) next.add(id)
        return next
      })
      // NOTE: do NOT clear submitErrors here. A successful add inside a mixed
      // batch invalidates the contains query → this guard fires on the
      // refetch; clearing here would wipe the still-relevant error for a
      // SIBLING row whose add failed. submitErrors are cleared on close, on
      // row toggle, and at the start of the next submit instead.
    }
  }

  // Clear per-session state when the popover closes. React 19.2: adjust state
  // during render on the open→close transition instead of a cascading effect
  // (covers both close paths — onOpenChange and any setOpen(false)).
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (!open) {
      // Clear ALL session-scoped intent so a never-submitted check (or stale
      // session item id) can't leak into the next open — the contains query
      // stays cached within its staleTime, so the re-seed guard won't fire on
      // reopen to reconcile it.
      setPendingAdds(new Set())
      setSessionItemIds(new Map())
      setSubmitErrors([])
      setSearch('')
      setRemoveConfirmId(null)
      setRemoveError(null)
    }
  }

  const collections = useMemo(
    () => myCollectionsData?.collections ?? [],
    [myCollectionsData]
  )

  const pendingCount = pendingAdds.size

  const filteredCollections = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return collections
    return collections.filter((c) => c.title.toLowerCase().includes(q))
  }, [collections, search])

  // Stable ids so each grouped section can label its own role="group".
  const recentlyUsedLabelId = useId()
  const allCollectionsLabelId = useId()

  // Snapshot the add-recency signal when the popover OPENS. Reading per-open
  // (not per-render) keeps the order stable while open — recording an add must
  // not make the list jump under the user; the new order shows on next open.
  // `{}` while closed avoids touching localStorage on SSR / on every
  // entity-page render of this high-traffic affordance.
  const recencySnapshot = useMemo<Record<string, number>>(
    () => (open ? readCollectionAddRecency() : {}),
    [open]
  )

  // Partition the (unfiltered) list into a promoted "recently used" group and
  // the rest. Promoted = collections with a recency stamp, newest first,
  // capped at MAX_PROMOTED; rest = everything else in the server's order. The
  // two are disjoint (a promoted row is not repeated below), matching the
  // locked design. Suppressed while filtering and for small libraries.
  const { promotedCollections, restCollections, showRecentlyUsed } =
    useMemo(() => {
      const searching = search.trim().length > 0
      if (searching || collections.length < RECENTLY_USED_MIN_COLLECTIONS) {
        return {
          promotedCollections: [],
          restCollections: [],
          showRecentlyUsed: false,
        }
      }
      const promoted = collections
        .filter((c) => recencySnapshot[String(c.id)] != null)
        .sort(
          (a, b) =>
            recencySnapshot[String(b.id)] - recencySnapshot[String(a.id)]
        )
        .slice(0, MAX_PROMOTED)
      const promotedIds = new Set(promoted.map((c) => c.id))
      const rest = collections.filter((c) => !promotedIds.has(c.id))
      // Need both a promoted item AND a remainder for grouping to add meaning.
      return {
        promotedCollections: promoted,
        restCollections: rest,
        showRecentlyUsed: promoted.length >= 1 && rest.length >= 1,
      }
    }, [collections, recencySnapshot, search])

  // Unauthenticated bracket variant — render the public [Add to collection]
  // affordance and redirect to /auth on click, mirroring FollowButton /
  // NotifyMeButton (which both render their bracket for unauth viewers).
  // ReleaseDetail renders neither a [Follow] nor a [Notify me] bracket, so
  // [Add to collection] is the ONLY public header bracket on a release —
  // returning null here left an empty linkbox for anonymous visitors (PSY-663).
  // Non-bracket variants still return null below — they have no public surface.
  if (!isAuthenticated && variant === 'bracket') {
    return (
      <BracketLink
        label="Add to collection"
        title={`Add "${entityName}" to a collection`}
        ariaLabel="Add to Collection"
        onClick={() =>
          router.push(`/auth?returnTo=${encodeURIComponent(pathname)}`)
        }
      />
    )
  }
  if (!isAuthenticated) return null

  const errorByCollection = new Map(
    submitErrors.map((e) => [e.collectionId, e.message])
  )

  const isChecked = (id: number) => savedIds.has(id) || pendingAdds.has(id)

  // A checkbox flip. Unchecking a SAVED row routes to the remove-confirm (D1)
  // rather than silently no-op'ing — the pre-PSY-829 popover only fanned out
  // newly-checked IDs, so unchecking an already-in row did nothing (dead
  // affordance). Unchecking a not-yet-saved row just drops it from the batch.
  const handleCheckedChange = (collectionId: number, checked: boolean) => {
    if (checked) {
      if (!savedIds.has(collectionId)) {
        setPendingAdds((prev) => new Set(prev).add(collectionId))
      }
      // Re-checking a row that just failed to add clears its stale error so the
      // checked row doesn't keep displaying "it failed" until the next submit.
      setSubmitErrors((prev) =>
        prev.filter((e) => e.collectionId !== collectionId)
      )
      return
    }
    // checked === false
    if (savedIds.has(collectionId)) {
      setRemoveConfirmId(collectionId)
      setRemoveError(null)
      return
    }
    setPendingAdds((prev) => {
      const next = new Set(prev)
      next.delete(collectionId)
      return next
    })
    setSubmitErrors((prev) =>
      prev.filter((e) => e.collectionId !== collectionId)
    )
  }

  const cancelRemove = () => {
    setRemoveConfirmId(null)
    setRemoveError(null)
  }

  const confirmRemove = async (collectionId: number) => {
    if (removeInFlightRef.current) return
    const collection = collections.find((c) => c.id === collectionId)
    // Prefer the server-confirmed item id; fall back to one captured from a
    // same-session add whose contains-refetch hasn't landed yet.
    const itemId = containing?.get(collectionId) ?? sessionItemIds.get(collectionId)
    if (!collection || itemId === undefined) {
      // Shouldn't happen — the confirm only opens for a saved row, and a saved
      // row's item id comes from either `containing` or `sessionItemIds`.
      // Surface an error rather than silently closing, so the user isn't left
      // with a checked-but-unremovable row (the dead-affordance class D1 fixes).
      setRemoveError({
        collectionId,
        message: 'Could not resolve this item — reopen and try again.',
      })
      return
    }
    removeInFlightRef.current = true
    setRemovingId(collectionId)
    setRemoveError(null)
    try {
      await removeMutation.mutateAsync({ slug: collection.slug, itemId })
      setSavedIds((prev) => {
        const next = new Set(prev)
        next.delete(collectionId)
        return next
      })
      setPendingAdds((prev) => {
        const next = new Set(prev)
        next.delete(collectionId)
        return next
      })
      setSessionItemIds((prev) => {
        const next = new Map(prev)
        next.delete(collectionId)
        return next
      })
      setRemoveConfirmId(null)
      // Cancel any in-flight contains refetch first: a just-submitted add fires
      // its own contains refetch, and if that GET (sent before this DELETE
      // committed) lands after the delete it would briefly re-seed this row as
      // "Added". Cancelling it before we invalidate avoids that flicker.
      queryClient.cancelQueries({
        queryKey: queryKeys.collections.containing(entityType, entityId),
      })
      // Refresh the contains-check + public backlinks for this entity so a
      // reopen (and the entity page's "appears in" list) reflect the removal.
      // The re-seed guard above preserves any un-submitted pending adds, so
      // this refetch can't clobber the user's in-progress selection.
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.containing(entityType, entityId),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.collections.entity(entityType, entityId),
      })
    } catch (err) {
      setRemoveError({ collectionId, message: describeError(err) })
    } finally {
      removeInFlightRef.current = false
      setRemovingId(null)
    }
  }

  // Fan out via Promise.allSettled so one failure doesn't kill the rest.
  const handleSubmit = async () => {
    if (pendingAdds.size === 0 || submitting || submitInFlightRef.current) return

    submitInFlightRef.current = true
    setSubmitting(true)
    setSubmitErrors([])

    const targets = [...pendingAdds]
      .map((id) => collections.find((c) => c.id === id))
      .filter((c) => c !== undefined)

    const results = await Promise.allSettled(
      targets.map((collection) =>
        addMutation
          .mutateAsync({
            slug: collection.slug,
            entityType,
            entityId,
          })
          .then(
            (item) => ({ id: collection.id, itemId: item.id }),
            (err: unknown) => {
              throw { id: collection.id, error: err }
            }
          )
      )
    )

    const nextSaved = new Set(savedIds)
    const nextPending = new Set(pendingAdds)
    const nextSessionItemIds = new Map(sessionItemIds)
    const errors: SubmitError[] = []

    // PSY-960: stamp this batch's adds with strictly-increasing timestamps so
    // "Recently used" keeps a stable newest-first order across a multi-select
    // submit — a bare Date.now() per add would tie within the same millisecond
    // and collapse to server order. `results` is in checked-row order, so the
    // last-checked collection gets the newest stamp.
    const recordedAt = Date.now()
    let addedRank = 0

    for (const result of results) {
      if (result.status === 'fulfilled') {
        nextSaved.add(result.value.id)
        nextPending.delete(result.value.id)
        nextSessionItemIds.set(result.value.id, result.value.itemId)
        // Record the add client-side so this collection promotes to "Recently
        // used" on the next open of the popover.
        recordCollectionAdd(result.value.id, recordedAt + addedRank)
        addedRank += 1
      } else {
        const reason = result.reason as { id: number; error: unknown }
        // Drop the failed row out of the add batch so it unchecks (matching
        // the row state to reality); the inline error explains why.
        nextPending.delete(reason.id)
        errors.push({
          collectionId: reason.id,
          message: describeError(reason.error),
        })
      }
    }

    setSavedIds(nextSaved)
    setPendingAdds(nextPending)
    setSessionItemIds(nextSessionItemIds)
    setSubmitErrors(errors)
    setSubmitting(false)
    submitInFlightRef.current = false
    // addMutation.onSuccess also invalidates the contains query → the re-seed
    // guard re-syncs savedIds with the server's item ids; sessionItemIds
    // bridges the gap until that refetch lands.
  }

  const isLoading = collectionsLoading || (open && containingLoading)
  const submitDisabled = submitting || pendingCount === 0
  const submitLabel = submitting
    ? 'Adding…'
    : pendingCount > 0
      ? `Add to ${pendingCount} ${pendingCount === 1 ? 'collection' : 'collections'}`
      : 'Add'
  const showSearch = collections.length > SEARCH_THRESHOLD
  const hasCollections = collections.length > 0

  // A single collection row, shared by the flat list and the grouped
  // (RECENTLY USED / ALL COLLECTIONS) layout so both render identically.
  const renderRow = (collection: (typeof collections)[number]) => {
    const checked = isChecked(collection.id)
    const added = savedIds.has(collection.id)
    const errorMessage = errorByCollection.get(collection.id)
    const rowRemoveError =
      removeError?.collectionId === collection.id
        ? removeError.message
        : undefined
    const checkboxId = `collection-checkbox-${collection.id}`
    const isConfirming = removeConfirmId === collection.id
    const isRemoving = removingId === collection.id
    const subtitle = `${collection.item_count} ${collection.item_count === 1 ? 'item' : 'items'} · ${collection.is_public ? 'Public' : 'Private'}`

    return (
      <div key={collection.id} className="px-1">
        <label
          htmlFor={checkboxId}
          className="flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm hover:bg-muted/50 transition-colors cursor-pointer"
        >
          <Checkbox
            id={checkboxId}
            checked={checked}
            onCheckedChange={(value) =>
              handleCheckedChange(collection.id, value === true)
            }
            disabled={submitting || isRemoving}
            aria-describedby={
              errorMessage || rowRemoveError
                ? `${checkboxId}-error`
                : undefined
            }
          />
          <CollectionCoverImage
            url={collection.cover_image_url}
            alt=""
            className="h-7 w-7 shrink-0 rounded bg-muted/50"
            fallback={
              <div className="flex h-full w-full items-center justify-center rounded bg-muted/50">
                <Library className="h-3.5 w-3.5 text-muted-foreground" />
              </div>
            }
          />
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="truncate font-medium leading-tight">
              {collection.title}
            </span>
            <span className="text-xs text-muted-foreground leading-tight">
              {subtitle}
            </span>
          </span>
          {added && !isConfirming && (
            <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400 shrink-0">
              <Check className="h-3.5 w-3.5" />
              Added
            </span>
          )}
        </label>

        {/* D1: inline remove-with-confirm for an already-in row. */}
        {isConfirming && (
          <div className="ml-8 mr-2 mb-1.5 flex items-center justify-between gap-2">
            <span className="text-xs text-muted-foreground">
              Remove from this collection?
            </span>
            <span className="flex items-center gap-1.5 shrink-0">
              <Button
                type="button"
                variant="destructive"
                size="sm"
                className="h-6 px-2 text-xs"
                onClick={() => confirmRemove(collection.id)}
                disabled={isRemoving}
              >
                {isRemoving && (
                  <Loader2 className="h-3 w-3 animate-spin mr-1" />
                )}
                Remove
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-xs"
                onClick={cancelRemove}
                disabled={isRemoving}
              >
                Cancel
              </Button>
            </span>
          </div>
        )}

        {(errorMessage || rowRemoveError) && (
          <div
            id={`${checkboxId}-error`}
            className="ml-8 mr-2 mb-1 flex items-start gap-1 text-xs text-destructive"
          >
            <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
            <span className="flex-1">{errorMessage ?? rowRemoveError}</span>
          </div>
        )}
      </div>
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        {variant === 'bracket' ? (
          <BracketLink
            label="Add to collection"
            title={`Add "${entityName}" to a collection`}
            aria-label="Add to Collection"
          />
        ) : (
          <Button
            variant={variant}
            size={size}
            className={size === 'icon' ? 'h-8 w-8 p-0' : ''}
            title={`Add "${entityName}" to a collection`}
            aria-label="Add to Collection"
          >
            <Library className="h-4 w-4" />
            {size !== 'icon' && <span className="ml-1.5">Collect</span>}
          </Button>
        )}
      </PopoverTrigger>
      <PopoverContent className="w-[360px] p-0" align="end">
        <div className="p-3 border-b border-border">
          <h4 className="text-sm font-display font-semibold">
            Add to Collection
          </h4>
          <p className="text-xs text-muted-foreground mt-0.5 truncate">
            {entityName}
          </p>
        </div>

        {showSearch && (
          <div className="p-2 border-b border-border">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
              <Input
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value)
                  // Filtering can hide the row whose remove-confirm is open,
                  // orphaning it (no Cancel until the filter clears). Dismiss
                  // the confirm whenever the visible set changes.
                  if (removeConfirmId !== null) {
                    setRemoveConfirmId(null)
                    setRemoveError(null)
                  }
                }}
                placeholder="Filter collections…"
                aria-label="Filter collections"
                className="h-8 pl-8 text-sm"
              />
            </div>
          </div>
        )}

        <div
          className="max-h-72 overflow-y-auto p-1"
          role="group"
          aria-label="Your collections"
        >
          {isLoading ? (
            <div className="flex items-center justify-center py-6">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          ) : !hasCollections ? (
            // D5: empty state promotes Create as the primary action.
            <div className="px-3 py-5 text-center">
              <p className="text-sm text-muted-foreground mb-3">
                No collections yet — start one.
              </p>
              <Button asChild size="sm" className="w-full">
                <Link href="/collections" onClick={() => setOpen(false)}>
                  <Plus className="h-3.5 w-3.5 mr-1.5" />
                  Create new collection
                </Link>
              </Button>
            </div>
          ) : filteredCollections.length === 0 ? (
            <div className="py-4 px-2 text-center">
              <p className="text-sm text-muted-foreground">
                No collections match “{search.trim()}”
              </p>
            </div>
          ) : showRecentlyUsed ? (
            <>
              <div role="group" aria-labelledby={recentlyUsedLabelId}>
                <SectionLabel id={recentlyUsedLabelId}>
                  Recently used
                </SectionLabel>
                {promotedCollections.map(renderRow)}
              </div>
              <div className="mx-2 my-1 h-px bg-border" aria-hidden="true" />
              <div role="group" aria-labelledby={allCollectionsLabelId}>
                <SectionLabel id={allCollectionsLabelId}>
                  All collections
                </SectionLabel>
                {restCollections.map(renderRow)}
              </div>
            </>
          ) : (
            filteredCollections.map(renderRow)
          )}
        </div>

        {/* Submit row: always present when the user has collections; the
            button is disabled (label "Add") until ≥1 new row is checked. */}
        {hasCollections && (
          <div className="p-2 border-t border-border">
            <Button
              type="button"
              size="sm"
              className="w-full"
              onClick={handleSubmit}
              disabled={submitDisabled}
            >
              {submitting && (
                <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />
              )}
              {submitLabel}
            </Button>
          </div>
        )}

        {/* Create new link. D3 (recently-used promotion) shipped in PSY-960
            via a client-side add-recency signal (see the grouped list above).
            D4 (create-from-entity pre-fill) is still deferred to PSY-961 —
            needs the Create drawer reachable from entity pages; until then
            this stays a plain link to the collections page rather than
            "Create … with {entity}". */}
        {hasCollections && (
          <div className="p-2 border-t border-border">
            <Link
              href="/collections"
              className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:text-foreground hover:bg-muted/50 transition-colors"
              onClick={() => setOpen(false)}
            >
              <Plus className="h-3.5 w-3.5" />
              Create new collection
            </Link>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
