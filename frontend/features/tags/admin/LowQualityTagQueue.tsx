'use client'

import { useCallback, useState } from 'react'
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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  useDeleteTag,
  useLowQualityTagQueue,
  useMarkTagOfficial,
  useSnoozeTag,
} from './useAdminTags'
import { MergeTagDialog } from './MergeTagDialog'
import {
  LOW_QUALITY_REASON_LABELS,
  getCategoryColor,
  getCategoryLabel,
  type LowQualityReason,
} from '../types'

const PAGE_SIZE = 20

type ActiveDialog = 'merge' | 'delete' | null

export function LowQualityTagQueue() {
  const [offset, setOffset] = useState(0)
  const { data, isLoading, error } = useLowQualityTagQueue({
    limit: PAGE_SIZE,
    offset,
  })

  const snoozeMutation = useSnoozeTag()
  const markOfficialMutation = useMarkTagOfficial()
  const deleteMutation = useDeleteTag()

  const [activeDialog, setActiveDialog] = useState<ActiveDialog>(null)
  const [selectedTagId, setSelectedTagId] = useState<number | null>(null)
  const [selectedTagName, setSelectedTagName] = useState('')

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

  return (
    <div className="space-y-4">
      <div>
        <p className="text-sm text-muted-foreground">
          Non-official tags flagged by at least one low-quality signal —
          orphaned, aging unused, downvoted, or unusual name length. Snoozed
          tags hide for 30 days.
        </p>
      </div>

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
          <div className="text-sm text-muted-foreground">
            Showing {offset + 1}-{Math.min(offset + tags.length, total)} of{' '}
            {total}
          </div>

          <div className="space-y-2">
            {tags.map((tag) => {
              const isBusy =
                (snoozeMutation.isPending &&
                  snoozeMutation.variables === tag.id) ||
                (markOfficialMutation.isPending &&
                  markOfficialMutation.variables === tag.id)

              return (
                <div
                  key={tag.id}
                  className="flex flex-col gap-3 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
                  data-testid={`low-quality-tag-${tag.id}`}
                >
                  <div className="flex items-start gap-3">
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

      {/* Delete confirmation */}
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
    </div>
  )
}

export default LowQualityTagQueue
