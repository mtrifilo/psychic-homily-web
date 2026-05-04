'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Library, Check, Plus, Loader2, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  useMyCollections,
  useAddCollectionItem,
  useUserCollectionsContaining,
} from '@/features/collections/hooks'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { CollectionEntityType } from '@/features/collections/types'

interface AddToCollectionButtonProps {
  entityType: CollectionEntityType
  entityId: number
  entityName: string
  variant?: 'default' | 'ghost' | 'outline'
  size?: 'sm' | 'default' | 'icon'
}

/**
 * Per-collection error from the most recent Add submission. Cleared on
 * retry. The id keys collection.id so successive retries replace the
 * previous attempt's row instead of stacking.
 */
type SubmitError = { collectionId: number; message: string }

/**
 * Single failure-message extraction so the empty-message and Error-message
 * cases share one fallback string. Keeping it module-scope avoids reallocating
 * the closure on every render.
 */
function describeError(reason: unknown): string {
  if (reason instanceof Error && reason.message) return reason.message
  if (typeof reason === 'string' && reason.length > 0) return reason
  return 'Failed to add to this collection'
}

export function AddToCollectionButton({
  entityType,
  entityId,
  entityName,
  variant = 'ghost',
  size = 'sm',
}: AddToCollectionButtonProps) {
  // NOTE: every hook below this comment must be called UNCONDITIONALLY,
  // BEFORE any early return. Earlier versions placed `useState` after the
  // `!isAuthenticated` early return, which produced a Rules-of-Hooks
  // violation once the auth profile resolved (PSY-466 regression). The
  // existing test suite (`renders without hook-order errors during the
  // auth loading → authenticated transition`) covers the regression.
  const { isAuthenticated } = useAuthContext()
  const [open, setOpen] = useState(false)

  // User's editable collections + the subset that already contain this entity.
  // The contains query is gated on `open` so we don't fetch on every entity
  // page render — only when the user actually opens the popover.
  const { data: myCollectionsData, isLoading: collectionsLoading } =
    useMyCollections()
  const { data: containingIds, isLoading: containingLoading } =
    useUserCollectionsContaining(entityType, entityId, { enabled: open })
  const addMutation = useAddCollectionItem()

  // Selected = the set the user wants the entity to be in after Submit.
  // Seeded from `containingIds` once it resolves, mutated by checkbox toggles.
  const [selected, setSelected] = useState<Set<number>>(new Set())
  // Server's last-known truth: the IDs the entity is already in. Diff against
  // `selected` to compute "newly checked" rows for submission, and to disable
  // Submit when nothing changed.
  const [savedIds, setSavedIds] = useState<Set<number>>(new Set())
  // Per-collection feedback after the most recent Submit. Successes flip the
  // row to a green check; failures show an inline error + retry affordance.
  const [submitErrors, setSubmitErrors] = useState<SubmitError[]>([])
  const [submitting, setSubmitting] = useState(false)
  // IDs successfully added during the current popover session. Clears on
  // close so a re-open of the popover starts with no green-check chips
  // (server-known checked state takes over). Persists through `containingIds`
  // refetches so the chip doesn't flicker off when the cache invalidates.
  const [justSaved, setJustSaved] = useState<Set<number>>(new Set())

  // Sync the popover state from server data each time we get a fresh answer
  // (popover open, mutation success → cache invalidation, etc.). Without
  // this, toggling a row, closing the popover, and reopening would show
  // stale local state.
  useEffect(() => {
    if (!containingIds) return
    setSelected(new Set(containingIds))
    setSavedIds(new Set(containingIds))
    // Stale errors from a previous open should not persist into a fresh open.
    setSubmitErrors([])
  }, [containingIds])

  // Reset session-only state when the popover closes — re-opening should
  // start fresh, with checked state coming from the server (containingIds).
  useEffect(() => {
    if (open) return
    setJustSaved(new Set())
    setSubmitErrors([])
  }, [open])

  const collections = myCollectionsData?.collections ?? []

  // Newly-checked rows are the ones the Submit handler will fan out to.
  // Memoized so the disabled-state check + button label don't recompute on
  // every keypress unrelated to this set.
  const newlyChecked = useMemo(() => {
    const result: number[] = []
    for (const id of selected) {
      if (!savedIds.has(id)) result.push(id)
    }
    return result
  }, [selected, savedIds])

  if (!isAuthenticated) return null

  const errorByCollection = new Map(
    submitErrors.map((e) => [e.collectionId, e.message])
  )

  const handleToggle = (collectionId: number) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(collectionId)) {
        next.delete(collectionId)
      } else {
        next.add(collectionId)
      }
      return next
    })
    // Toggling clears the error for that row so the user sees a clean retry.
    setSubmitErrors((prev) => prev.filter((e) => e.collectionId !== collectionId))
  }

  /**
   * Fan out N parallel AddItem requests via Promise.allSettled so one failure
   * doesn't kill the rest. Successes update the local "saved" set so they
   * flip to checked-and-green; failures collect into `submitErrors` for
   * inline display + retry.
   */
  const handleSubmit = async () => {
    if (newlyChecked.length === 0 || submitting) return

    setSubmitting(true)
    setSubmitErrors([])

    // Resolve slugs + titles up front so the closure body is just the call.
    const targets = newlyChecked
      .map((id) => collections.find((c) => c.id === id))
      .filter((c): c is NonNullable<typeof c> => Boolean(c))

    const results = await Promise.allSettled(
      targets.map((collection) =>
        addMutation
          .mutateAsync({
            slug: collection.slug,
            entityType,
            entityId,
          })
          // Wrap so .allSettled gets the collection.id alongside the error.
          .then(
            () => ({ kind: 'ok' as const, id: collection.id }),
            (err: unknown) => {
              throw { id: collection.id, error: err }
            }
          )
      )
    )

    const newlySaved = new Set(savedIds)
    const sessionSavedIds = new Set(justSaved)
    const errors: SubmitError[] = []

    for (const result of results) {
      if (result.status === 'fulfilled') {
        newlySaved.add(result.value.id)
        sessionSavedIds.add(result.value.id)
      } else {
        const reason = result.reason as { id: number; error: unknown }
        errors.push({ collectionId: reason.id, message: describeError(reason.error) })
      }
    }

    setSavedIds(newlySaved)
    setJustSaved(sessionSavedIds)
    // Successes are still checked from earlier `selected` mutation — only
    // failures need to drop back out so the row's pre-mutation state is
    // visually accurate (unchecked + error message).
    if (errors.length > 0) {
      setSelected((prev) => {
        const next = new Set(prev)
        for (const e of errors) next.delete(e.collectionId)
        return next
      })
    }
    setSubmitErrors(errors)
    setSubmitting(false)
  }

  const isLoading = collectionsLoading || (open && containingLoading)
  const submitDisabled = submitting || newlyChecked.length === 0
  const submitLabel = submitting
    ? 'Adding…'
    : newlyChecked.length > 0
      ? `Add to ${newlyChecked.length} ${newlyChecked.length === 1 ? 'collection' : 'collections'}`
      : 'Add'

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
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
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0" align="end">
        <div className="p-3 border-b border-border">
          <h4 className="text-sm font-semibold">Add to Collection</h4>
          <p className="text-xs text-muted-foreground mt-0.5 truncate">
            {entityName}
          </p>
        </div>

        <div
          className="max-h-64 overflow-y-auto p-1"
          role="group"
          aria-label="Your collections"
        >
          {isLoading ? (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          ) : collections.length === 0 ? (
            <div className="py-3 px-2 text-center">
              <p className="text-sm text-muted-foreground">No collections yet</p>
            </div>
          ) : (
            collections.map((collection) => {
              const isChecked = selected.has(collection.id)
              const wasJustSaved = justSaved.has(collection.id)
              const errorMessage = errorByCollection.get(collection.id)
              const checkboxId = `collection-checkbox-${collection.id}`

              return (
                <div key={collection.id} className="px-1">
                  <label
                    htmlFor={checkboxId}
                    className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted/50 transition-colors cursor-pointer"
                  >
                    <Checkbox
                      id={checkboxId}
                      checked={isChecked}
                      onCheckedChange={() => handleToggle(collection.id)}
                      disabled={submitting}
                      aria-describedby={
                        errorMessage ? `${checkboxId}-error` : undefined
                      }
                    />
                    <Library className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                    <span className="flex-1 truncate">{collection.title}</span>
                    {wasJustSaved && (
                      <Check className="h-3.5 w-3.5 text-green-600 dark:text-green-400 shrink-0" />
                    )}
                  </label>
                  {errorMessage && (
                    <div
                      id={`${checkboxId}-error`}
                      className="ml-8 mr-2 mb-1 flex items-start gap-1 text-xs text-destructive"
                    >
                      <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
                      <span className="flex-1">{errorMessage}</span>
                    </div>
                  )}
                </div>
              )
            })
          )}
        </div>

        {/* Submit row */}
        {collections.length > 0 && (
          <div className="p-2 border-t border-border flex items-center gap-2">
            <Button
              type="button"
              size="sm"
              className="flex-1"
              onClick={handleSubmit}
              disabled={submitDisabled}
            >
              {submitting && <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />}
              {submitLabel}
            </Button>
          </div>
        )}

        {/* Create new link */}
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
      </PopoverContent>
    </Popover>
  )
}
