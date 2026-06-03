'use client'

import { useMemo, useState } from 'react'
import Link from 'next/link'
import { useRouter, usePathname } from 'next/navigation'
import { Library, Check, Plus, Loader2, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from './BracketLink'
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
// guaranteed distinct from any real `containingIds`, so the guard also fires
// on the FIRST render (the prior effect always ran on mount and seeded).
const UNSET = Symbol('unset')

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
  // Every hook below this comment must be called UNCONDITIONALLY, BEFORE
  // any early return. Placing `useState` after the `!isAuthenticated`
  // early return triggered a Rules-of-Hooks violation once the auth
  // profile resolved (PSY-466).
  const { isAuthenticated } = useAuthContext()
  const router = useRouter()
  const pathname = usePathname()
  const [open, setOpen] = useState(false)

  // The contains query is gated on `open` so we don't fetch on every
  // entity page render — only when the user actually opens the popover.
  const { data: myCollectionsData, isLoading: collectionsLoading } =
    useMyCollections()
  const { data: containingIds, isLoading: containingLoading } =
    useUserCollectionsContaining(entityType, entityId, { enabled: open })
  const addMutation = useAddCollectionItem()

  const [selected, setSelected] = useState<Set<number>>(new Set())
  // Server's last-known truth. `selected` minus `savedIds` is the diff the
  // Submit handler needs to fan out.
  const [savedIds, setSavedIds] = useState<Set<number>>(new Set())
  const [submitErrors, setSubmitErrors] = useState<SubmitError[]>([])
  const [submitting, setSubmitting] = useState(false)
  // IDs successfully added during the current popover session. Persists
  // through `containingIds` cache invalidations so the green chip doesn't
  // flicker off mid-refetch; cleared on popover close.
  const [justSaved, setJustSaved] = useState<Set<number>>(new Set())

  // Seed `selected`/`savedIds` from the server's contains-truth whenever the
  // query resolves (or returns a fresh reference). React 19.2: adjust state
  // during render via the canonical previous-value-guard idiom instead of a
  // cascading effect. The tracker starts at a sentinel so the guard also fires
  // on the FIRST render when `containingIds` is already resolved (matching the
  // prior effect, which always ran on mount). Otherwise this preserves the
  // prior effect's `[containingIds]`-dependency semantics exactly.
  const [prevContainingIds, setPrevContainingIds] = useState<
    typeof containingIds | typeof UNSET
  >(UNSET)
  if (containingIds !== prevContainingIds) {
    setPrevContainingIds(containingIds)
    if (containingIds) {
      setSelected(new Set(containingIds))
      setSavedIds(new Set(containingIds))
      setSubmitErrors([])
    }
  }

  // Clear the per-session "just saved" chips and any submit errors when the
  // popover closes. React 19.2: adjust state during render on the open→close
  // transition instead of a cascading effect (covers both close paths —
  // onOpenChange and the "Create new collection" link's setOpen(false)).
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (!open) {
      setJustSaved(new Set())
      setSubmitErrors([])
    }
  }

  const collections = myCollectionsData?.collections ?? []

  const newlyChecked = useMemo(() => {
    const result: number[] = []
    for (const id of selected) {
      if (!savedIds.has(id)) result.push(id)
    }
    return result
  }, [selected, savedIds])

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
    setSubmitErrors((prev) => prev.filter((e) => e.collectionId !== collectionId))
  }

  // Fan out via Promise.allSettled so one failure doesn't kill the rest.
  const handleSubmit = async () => {
    if (newlyChecked.length === 0 || submitting) return

    setSubmitting(true)
    setSubmitErrors([])

    const targets = newlyChecked
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
            () => ({ id: collection.id }),
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
    // Drop failures back out of `selected` so the row state matches reality.
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
      <PopoverContent className="w-72 p-0" align="end">
        <div className="p-3 border-b border-border">
          <h4 className="text-sm font-display font-semibold">Add to Collection</h4>
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
