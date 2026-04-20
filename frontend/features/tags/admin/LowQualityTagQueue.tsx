'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Loader2,
  Inbox,
  Trash2,
  GitMerge,
  Sparkles,
  EyeOff,
  ThumbsDown,
  ThumbsUp,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  useBulkLowQualityAction,
  useDeleteTag,
  useLowQualityTagQueue,
  useMarkTagOfficial,
  useSnoozeTag,
} from './useAdminTags'
import { MergeTagDialog } from './MergeTagDialog'
import {
  LOW_QUALITY_REASON_LABELS,
  LOW_QUALITY_SIGNAL_CHIPS,
  getCategoryColor,
  getCategoryLabel,
  type BulkLowQualityAction,
  type LowQualityReason,
  type LowQualityTagQueueItem,
} from '../types'

const PAGE_SIZE = 20

// Selecting more than this triggers a "Type 'delete N tags' to confirm"
// keystroke gate on the bulk delete path. Smaller selections still get a
// regular confirm dialog (any destructive action does), just no typing required.
const BULK_DELETE_TYPED_CONFIRM_THRESHOLD = 5

type ActiveDialog =
  | 'merge'
  | 'delete'
  | 'bulk-snooze'
  | 'bulk-mark-official'
  | 'bulk-delete'
  | null

export function LowQualityTagQueue() {
  const [offset, setOffset] = useState(0)
  const { data, isLoading, error } = useLowQualityTagQueue({
    limit: PAGE_SIZE,
    offset,
  })

  const snoozeMutation = useSnoozeTag()
  const markOfficialMutation = useMarkTagOfficial()
  const deleteMutation = useDeleteTag()
  const bulkActionMutation = useBulkLowQualityAction()

  const [activeDialog, setActiveDialog] = useState<ActiveDialog>(null)
  const [selectedTagId, setSelectedTagId] = useState<number | null>(null)
  const [selectedTagName, setSelectedTagName] = useState('')

  // Bulk-select state — keyed by tag id so the set survives pagination
  // (admin selects 5 on page 1, paginates, comes back, still selected).
  // We DO NOT auto-clear on pagination; clear only on explicit "Clear" /
  // successful bulk action.
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [bulkError, setBulkError] = useState<string | null>(null)
  const [bulkDeleteConfirmText, setBulkDeleteConfirmText] = useState('')

  // Multi-select signal-type filter chips (PSY-487).
  // `null` ID-set means "All"; an empty filter set means "All" too.
  const [activeSignalIds, setActiveSignalIds] = useState<Set<string>>(new Set())

  const openMerge = useCallback((id: number, name: string) => {
    setSelectedTagId(id)
    setSelectedTagName(name)
    setActiveDialog('merge')
  }, [])

  const openDelete = useCallback((id: number, name: string) => {
    setSelectedTagId(id)
    setSelectedTagName(name)
    setActiveDialog('delete')
  }, [])

  const closeDialog = useCallback(() => {
    setActiveDialog(null)
    setSelectedTagId(null)
    setSelectedTagName('')
    setBulkError(null)
    setBulkDeleteConfirmText('')
  }, [])

  const handleSnooze = useCallback(
    (id: number) => {
      snoozeMutation.mutate(id)
    },
    [snoozeMutation]
  )

  const handleMarkOfficial = useCallback(
    (id: number) => {
      markOfficialMutation.mutate(id)
    },
    [markOfficialMutation]
  )

  const handleDelete = useCallback(() => {
    if (selectedTagId == null) return
    deleteMutation.mutate(selectedTagId, {
      onSuccess: () => closeDialog(),
    })
  }, [selectedTagId, deleteMutation, closeDialog])

  const tags = data?.tags ?? []
  const total = data?.total ?? 0
  const hasPrev = offset > 0
  const hasNext = offset + PAGE_SIZE < total

  // Filter the visible page by active signal chips. Filtering is client-side
  // — the backend returns the union, and admins can narrow within the page
  // they're already looking at. (A wider chip filter that drives the server
  // query is a future ticket; this matches the spec's "filter to 'Aging
  // unused' only" example.)
  const visibleTags = useMemo<LowQualityTagQueueItem[]>(() => {
    if (activeSignalIds.size === 0) return tags
    const allowedReasons = new Set<LowQualityReason>()
    for (const chip of LOW_QUALITY_SIGNAL_CHIPS) {
      if (activeSignalIds.has(chip.id)) {
        for (const r of chip.reasons) allowedReasons.add(r)
      }
    }
    return tags.filter((t) => t.reasons.some((r) => allowedReasons.has(r)))
  }, [tags, activeSignalIds])

  // Drop selections that are no longer in the visible page. Without this,
  // admins selecting on filtered view, then changing filter, can wind up
  // with bulk actions targeting "invisible" tags (still legal — the IDs are
  // valid — but confusing).
  const visibleIdSet = useMemo(() => {
    const s = new Set<number>()
    for (const t of visibleTags) s.add(t.id)
    return s
  }, [visibleTags])

  const visibleSelectedIds = useMemo(() => {
    const out: number[] = []
    for (const id of selectedIds) {
      if (visibleIdSet.has(id)) out.push(id)
    }
    return out
  }, [selectedIds, visibleIdSet])

  const selectionCount = visibleSelectedIds.length
  const allVisibleSelected =
    visibleTags.length > 0 && selectionCount === visibleTags.length

  const toggleSignal = useCallback((id: string) => {
    setActiveSignalIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const toggleRow = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const toggleSelectAllVisible = useCallback(() => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (allVisibleSelected) {
        for (const t of visibleTags) next.delete(t.id)
      } else {
        for (const t of visibleTags) next.add(t.id)
      }
      return next
    })
  }, [allVisibleSelected, visibleTags])

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set())
  }, [])

  // Available signal chips on the current page — the spec says "build the
  // chip set from the union of distinct signals returned by the backend".
  // We always render the canonical ordered set, but disable chips that have
  // no matching tags on the current page so the chip row reflects reality.
  const availableSignalIds = useMemo(() => {
    const seen = new Set<LowQualityReason>()
    for (const t of tags) for (const r of t.reasons) seen.add(r)
    const ids = new Set<string>()
    for (const chip of LOW_QUALITY_SIGNAL_CHIPS) {
      if (chip.reasons.some((r) => seen.has(r))) ids.add(chip.id)
    }
    return ids
  }, [tags])

  const requireTypedConfirm =
    selectionCount > BULK_DELETE_TYPED_CONFIRM_THRESHOLD
  const expectedBulkDeletePhrase = `delete ${selectionCount} tags`
  const typedConfirmMatches =
    !requireTypedConfirm ||
    bulkDeleteConfirmText.trim() === expectedBulkDeletePhrase

  const runBulkAction = useCallback(
    (action: BulkLowQualityAction) => {
      if (visibleSelectedIds.length === 0) return
      setBulkError(null)
      bulkActionMutation.mutate(
        { action, tagIds: visibleSelectedIds },
        {
          onSuccess: () => {
            clearSelection()
            closeDialog()
          },
          onError: (err) => {
            setBulkError(
              err instanceof Error ? err.message : 'Bulk action failed.'
            )
          },
        }
      )
    },
    [bulkActionMutation, clearSelection, closeDialog, visibleSelectedIds]
  )

  // Reset typed-confirm text any time the selection size changes — the
  // expected phrase depends on the count.
  useEffect(() => {
    if (activeDialog === 'bulk-delete') return
    setBulkDeleteConfirmText('')
  }, [selectionCount, activeDialog])

  return (
    <div className="space-y-4">
      <div>
        <p className="text-sm text-muted-foreground">
          Non-official tags flagged by at least one low-quality signal —
          orphaned, aging unused, downvoted, or unusual name length. Snoozed
          tags hide for 30 days.
        </p>
      </div>

      {!isLoading && !error && tags.length > 0 && (
        <div
          className="flex flex-wrap items-center gap-2"
          data-testid="signal-filter-chips"
          role="group"
          aria-label="Filter by signal type"
        >
          <Button
            variant={activeSignalIds.size === 0 ? 'default' : 'outline'}
            size="sm"
            onClick={() => setActiveSignalIds(new Set())}
            className="h-7 px-2 text-xs"
            aria-pressed={activeSignalIds.size === 0}
          >
            All
          </Button>
          {LOW_QUALITY_SIGNAL_CHIPS.map((chip) => {
            const isActive = activeSignalIds.has(chip.id)
            const isAvailable = availableSignalIds.has(chip.id)
            return (
              <Button
                key={chip.id}
                variant={isActive ? 'default' : 'outline'}
                size="sm"
                onClick={() => toggleSignal(chip.id)}
                disabled={!isAvailable && !isActive}
                className="h-7 px-2 text-xs"
                aria-pressed={isActive}
                data-testid={`signal-chip-${chip.id}`}
              >
                {chip.label}
              </Button>
            )
          })}
        </div>
      )}

      {selectionCount > 0 && (
        <div
          className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-background/95 p-3 shadow-sm backdrop-blur"
          data-testid="bulk-action-toolbar"
          role="region"
          aria-label="Bulk actions"
        >
          <div className="flex items-center gap-3">
            <span className="text-sm font-medium">
              {selectionCount} selected
            </span>
            <Button
              variant="ghost"
              size="sm"
              onClick={clearSelection}
              className="h-7 text-xs"
            >
              Clear
            </Button>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setActiveDialog('bulk-mark-official')}
              disabled={bulkActionMutation.isPending}
              data-testid="bulk-action-mark-official"
            >
              <Sparkles className="h-3.5 w-3.5 mr-1" />
              Bulk Mark Official
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setActiveDialog('bulk-snooze')}
              disabled={bulkActionMutation.isPending}
              data-testid="bulk-action-snooze"
            >
              <EyeOff className="h-3.5 w-3.5 mr-1" />
              Bulk Ignore
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setActiveDialog('bulk-delete')}
              disabled={bulkActionMutation.isPending}
              data-testid="bulk-action-delete"
            >
              <Trash2 className="h-3.5 w-3.5 mr-1" />
              Bulk Delete
            </Button>
          </div>
        </div>
      )}

      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center">
          <p className="text-destructive">
            {error instanceof Error
              ? error.message
              : 'Failed to load review queue.'}
          </p>
        </div>
      )}

      {!isLoading && !error && tags.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted mb-4">
            <Inbox className="h-8 w-8 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">Nothing to review</h3>
          <p className="text-sm text-muted-foreground max-w-sm">
            No tags match the low-quality criteria. Community tag hygiene is on
            track.
          </p>
        </div>
      )}

      {!isLoading && !error && tags.length > 0 && (
        <>
          <div className="flex items-center justify-between gap-2 text-sm text-muted-foreground">
            <div className="flex items-center gap-2">
              <Checkbox
                id="select-all-visible"
                checked={allVisibleSelected}
                onCheckedChange={toggleSelectAllVisible}
                aria-label="Select all visible tags"
                data-testid="select-all-visible"
              />
              <label
                htmlFor="select-all-visible"
                className="cursor-pointer text-xs"
              >
                Select all visible
              </label>
            </div>
            <span>
              Showing {offset + 1}-{Math.min(offset + tags.length, total)} of{' '}
              {total}
              {activeSignalIds.size > 0 &&
                ` (${visibleTags.length} after filter)`}
            </span>
          </div>

          {visibleTags.length === 0 && (
            <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
              No tags on this page match the active signal filters.
            </div>
          )}

          <div className="space-y-2">
            {visibleTags.map((tag) => {
              const isBusy =
                (snoozeMutation.isPending &&
                  snoozeMutation.variables === tag.id) ||
                (markOfficialMutation.isPending &&
                  markOfficialMutation.variables === tag.id)
              const isSelected = selectedIds.has(tag.id)

              return (
                <div
                  key={tag.id}
                  className="flex flex-col gap-3 rounded-lg border p-3 hover:bg-muted/50 transition-colors data-[selected=true]:border-primary/50 data-[selected=true]:bg-primary/5"
                  data-selected={isSelected}
                  data-testid={`low-quality-tag-${tag.id}`}
                >
                  <div className="flex items-start gap-3">
                    <Checkbox
                      checked={isSelected}
                      onCheckedChange={() => toggleRow(tag.id)}
                      aria-label={`Select ${tag.name}`}
                      className="mt-0.5"
                      data-testid={`row-checkbox-${tag.id}`}
                    />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="font-medium text-sm truncate">
                          {tag.name}
                        </span>
                        <Badge
                          variant="outline"
                          className={`text-xs flex-shrink-0 ${getCategoryColor(tag.category)}`}
                        >
                          {getCategoryLabel(tag.category)}
                        </Badge>
                      </div>
                      <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                        <span>
                          {tag.usage_count}{' '}
                          {tag.usage_count === 1 ? 'use' : 'uses'}
                        </span>
                        <span className="text-muted-foreground/50">
                          /{tag.slug}
                        </span>
                        {(tag.upvotes > 0 || tag.downvotes > 0) && (
                          <>
                            <span className="inline-flex items-center gap-1">
                              <ThumbsUp className="h-3 w-3" />
                              {tag.upvotes}
                            </span>
                            <span className="inline-flex items-center gap-1">
                              <ThumbsDown className="h-3 w-3" />
                              {tag.downvotes}
                            </span>
                          </>
                        )}
                      </div>
                    </div>

                    <div className="flex items-center gap-1 flex-shrink-0">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleMarkOfficial(tag.id)}
                        disabled={isBusy}
                        aria-label={`Mark ${tag.name} official`}
                        title="Promote to official"
                      >
                        <Sparkles className="h-3.5 w-3.5 mr-1" />
                        Official
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => openMerge(tag.id, tag.name)}
                        aria-label={`Merge ${tag.name}`}
                        title="Merge into another tag"
                      >
                        <GitMerge className="h-3.5 w-3.5 mr-1" />
                        Merge
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleSnooze(tag.id)}
                        disabled={isBusy}
                        aria-label={`Ignore ${tag.name} for 30 days`}
                        title="Ignore for 30 days"
                      >
                        <EyeOff className="h-3.5 w-3.5 mr-1" />
                        Ignore
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => openDelete(tag.id, tag.name)}
                        className="text-muted-foreground hover:text-destructive"
                        aria-label={`Delete ${tag.name}`}
                        title="Delete tag"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>

                  {tag.reasons.length > 0 && (
                    <div
                      className="flex flex-wrap gap-1"
                      data-testid={`reasons-${tag.id}`}
                    >
                      {tag.reasons.map((r: LowQualityReason) => (
                        <Badge
                          key={r}
                          variant="secondary"
                          className="text-xs"
                        >
                          {LOW_QUALITY_REASON_LABELS[r] ?? r}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>

          <div className="flex items-center justify-between pt-2">
            <Button
              variant="outline"
              size="sm"
              disabled={!hasPrev}
              onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
            >
              Previous
            </Button>
            <span className="text-xs text-muted-foreground">
              Page {Math.floor(offset / PAGE_SIZE) + 1} of{' '}
              {Math.max(1, Math.ceil(total / PAGE_SIZE))}
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={!hasNext}
              onClick={() => setOffset((o) => o + PAGE_SIZE)}
            >
              Next
            </Button>
          </div>
        </>
      )}

      {/* Merge dialog — reuses the existing PSY-306 flow */}
      <MergeTagDialog
        open={activeDialog === 'merge'}
        sourceTagId={activeDialog === 'merge' ? selectedTagId : null}
        sourceTagName={selectedTagName}
        onClose={closeDialog}
      />

      {/* Single-row delete confirmation */}
      <Dialog
        open={activeDialog === 'delete'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Tag</DialogTitle>
            <DialogDescription>
              This action is permanent and cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">
              Are you sure you want to delete{' '}
              <span className="font-semibold text-foreground">
                &quot;{selectedTagName}&quot;
              </span>
              ? This will remove it from all entities and delete all associated
              aliases and votes.
            </p>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={closeDialog}
                disabled={deleteMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleDelete}
                disabled={deleteMutation.isPending}
              >
                {deleteMutation.isPending ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Deleting...
                  </>
                ) : (
                  'Delete Tag'
                )}
              </Button>
            </DialogFooter>
          </div>
        </DialogContent>
      </Dialog>

      {/* Bulk Snooze (Ignore) confirmation */}
      <Dialog
        open={activeDialog === 'bulk-snooze'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Ignore {selectionCount} tag{selectionCount === 1 ? '' : 's'}</DialogTitle>
            <DialogDescription>
              These tags will be hidden from the queue for 30 days. They&apos;ll
              re-surface if they still match the low-quality criteria after the
              snooze window.
            </DialogDescription>
          </DialogHeader>
          {bulkError && (
            <div
              role="alert"
              className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
            >
              {bulkError}
            </div>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={closeDialog}
              disabled={bulkActionMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={() => runBulkAction('snooze')}
              disabled={bulkActionMutation.isPending}
              data-testid="bulk-snooze-confirm"
            >
              {bulkActionMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Working...
                </>
              ) : (
                `Ignore ${selectionCount}`
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Bulk Mark Official confirmation */}
      <Dialog
        open={activeDialog === 'bulk-mark-official'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>
              Mark {selectionCount} tag{selectionCount === 1 ? '' : 's'} official
            </DialogTitle>
            <DialogDescription>
              Promoting tags to official excludes them from this queue
              permanently and surfaces them as canonical taxonomy values.
            </DialogDescription>
          </DialogHeader>
          {bulkError && (
            <div
              role="alert"
              className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
            >
              {bulkError}
            </div>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={closeDialog}
              disabled={bulkActionMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={() => runBulkAction('mark_official')}
              disabled={bulkActionMutation.isPending}
              data-testid="bulk-mark-official-confirm"
            >
              {bulkActionMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Working...
                </>
              ) : (
                `Mark ${selectionCount} official`
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Bulk Delete — typed confirmation when selection is large */}
      <Dialog
        open={activeDialog === 'bulk-delete'}
        onOpenChange={(open) => !open && closeDialog()}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>
              Delete {selectionCount} tag{selectionCount === 1 ? '' : 's'}
            </DialogTitle>
            <DialogDescription>
              This is permanent. Entity tags, votes, and aliases attached to
              these tags will also be deleted via FK cascade.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            {bulkError && (
              <div
                role="alert"
                className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
              >
                {bulkError}
              </div>
            )}
            {requireTypedConfirm && (
              <div className="space-y-2">
                <label
                  htmlFor="bulk-delete-confirm-input"
                  className="text-sm font-medium"
                >
                  Type{' '}
                  <code className="rounded bg-muted px-1 py-0.5 text-xs">
                    {expectedBulkDeletePhrase}
                  </code>{' '}
                  to confirm
                </label>
                <Input
                  id="bulk-delete-confirm-input"
                  data-testid="bulk-delete-confirm-input"
                  value={bulkDeleteConfirmText}
                  onChange={(e) => setBulkDeleteConfirmText(e.target.value)}
                  autoComplete="off"
                  autoFocus
                  placeholder={expectedBulkDeletePhrase}
                />
              </div>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={closeDialog}
              disabled={bulkActionMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => runBulkAction('delete')}
              disabled={
                bulkActionMutation.isPending || !typedConfirmMatches
              }
              data-testid="bulk-delete-confirm"
            >
              {bulkActionMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Deleting...
                </>
              ) : (
                `Delete ${selectionCount}`
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default LowQualityTagQueue
