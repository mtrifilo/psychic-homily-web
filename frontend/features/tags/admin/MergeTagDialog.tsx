'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Loader2, GitMerge, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import { useSearchTags } from '../hooks'
import {
  useMergeTags,
  useMergeTagsPreview,
  useTagAliases,
} from './useAdminTags'
import type { TagListItem } from '../types'

interface MergeTagDialogProps {
  open: boolean
  sourceTagId: number | null
  sourceTagName: string
  onClose: () => void
  onSuccess?: () => void
}

// Re-exported separately so useTagAliases lives alongside the rest of the hooks
// and the dialog imports what it needs without creating a circular barrel.
function useSourceAliasIds(sourceTagId: number | null): number[] {
  const { data } = useTagAliases(sourceTagId ?? 0, {
    enabled: sourceTagId != null && sourceTagId > 0,
  })
  return useMemo(() => (data?.aliases ?? []).map(a => a.id), [data])
}

export function MergeTagDialog({
  open,
  sourceTagId,
  sourceTagName,
  onClose,
  onSuccess,
}: MergeTagDialogProps) {
  const [search, setSearch] = useState('')
  const [debounced, setDebounced] = useState('')
  const [selectedTarget, setSelectedTarget] = useState<TagListItem | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search), 250)
    return () => clearTimeout(t)
  }, [search])

  // Reset state whenever the dialog opens for a new source tag.
  useEffect(() => {
    if (!open) return
    setSearch('')
    setDebounced('')
    setSelectedTarget(null)
    setError(null)
  }, [open, sourceTagId])

  const { data: searchData, isLoading: searching } = useSearchTags(
    debounced,
    10
  )
  const sourceAliasIds = useSourceAliasIds(sourceTagId)

  // Exclude the source itself + its existing aliases from the candidate list.
  const candidates = useMemo(() => {
    const results = searchData?.tags ?? []
    return results.filter(
      t =>
        t.id !== sourceTagId && !sourceAliasIds.includes(t.id)
    )
  }, [searchData, sourceTagId, sourceAliasIds])

  const {
    data: preview,
    isLoading: previewLoading,
    error: previewError,
  } = useMergeTagsPreview(sourceTagId, selectedTarget?.id ?? null, {
    enabled: open,
  })

  const mergeMutation = useMergeTags()

  const handleConfirm = useCallback(() => {
    if (!sourceTagId || !selectedTarget) return
    setError(null)
    mergeMutation.mutate(
      { sourceId: sourceTagId, targetId: selectedTarget.id },
      {
        onSuccess: () => {
          onSuccess?.()
          onClose()
        },
        onError: err => {
          setError(err instanceof Error ? err.message : 'Failed to merge tags')
        },
      }
    )
  }, [sourceTagId, selectedTarget, mergeMutation, onSuccess, onClose])

  const totalMoves = preview
    ? preview.moved_entity_tags + preview.moved_votes
    : 0
  const totalSkips = preview
    ? preview.skipped_entity_tags + preview.skipped_votes
    : 0

  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitMerge className="h-5 w-5" />
            Merge &quot;{sourceTagName}&quot; into...
          </DialogTitle>
          <DialogDescription>
            Pick a target tag. All entity applications, votes, and aliases move
            to the target. &quot;{sourceTagName}&quot; becomes an alias and is
            then deleted.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {error && (
            <div
              role="alert"
              className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
            >
              {error}
            </div>
          )}

          {selectedTarget ? (
            <div className="space-y-3">
              <div className="rounded-lg border p-3">
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{selectedTarget.name}</span>
                      <Badge variant="outline" className="text-xs">
                        {selectedTarget.category}
                      </Badge>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {selectedTarget.usage_count} uses / {selectedTarget.slug}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setSelectedTarget(null)}
                    disabled={mergeMutation.isPending}
                  >
                    Change
                  </Button>
                </div>
              </div>

              {previewLoading && (
                <div className="flex items-center justify-center py-4 text-sm text-muted-foreground">
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Loading preview...
                </div>
              )}
              {previewError && (
                <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
                  {previewError instanceof Error
                    ? previewError.message
                    : 'Failed to load preview.'}
                </div>
              )}
              {preview && (
                <div
                  data-testid="merge-preview"
                  className="rounded-lg border bg-muted/30 p-3 text-sm space-y-1"
                >
                  <p>
                    This will move{' '}
                    <strong>
                      {preview.moved_entity_tags} entity tag
                      {preview.moved_entity_tags === 1 ? '' : 's'}
                    </strong>{' '}
                    and{' '}
                    <strong>
                      {preview.moved_votes} vote
                      {preview.moved_votes === 1 ? '' : 's'}
                    </strong>{' '}
                    to &quot;{preview.target_name}&quot;.
                  </p>
                  {totalSkips > 0 && (
                    <p className="text-muted-foreground">
                      {preview.skipped_entity_tags} duplicate entity tag
                      {preview.skipped_entity_tags === 1 ? '' : 's'} and{' '}
                      {preview.skipped_votes} duplicate vote
                      {preview.skipped_votes === 1 ? '' : 's'} will be dropped
                      (target already has them).
                    </p>
                  )}
                  {preview.source_aliases_count > 0 && (
                    <p className="text-muted-foreground">
                      {preview.source_aliases_count} existing alias
                      {preview.source_aliases_count === 1 ? '' : 'es'} on
                      &quot;{preview.source_name}&quot; will be re-pointed to
                      &quot;{preview.target_name}&quot;.
                    </p>
                  )}
                  <p className="text-muted-foreground">
                    &quot;{preview.source_name}&quot; will become an alias of
                    &quot;{preview.target_name}&quot;.
                  </p>
                </div>
              )}
            </div>
          ) : (
            <div className="space-y-2">
              <label className="text-sm font-medium">
                Search for target tag
              </label>
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  autoFocus
                  placeholder="Type at least 2 characters..."
                  value={search}
                  onChange={e => setSearch(e.target.value)}
                  className="pl-9"
                />
              </div>

              {debounced.length >= 2 && (
                <div className="rounded-md border max-h-64 overflow-y-auto">
                  {searching && (
                    <div className="flex items-center justify-center py-3 text-sm text-muted-foreground">
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Searching...
                    </div>
                  )}
                  {!searching && candidates.length === 0 && (
                    <div className="p-3 text-sm text-muted-foreground">
                      No matching tags.
                    </div>
                  )}
                  {!searching &&
                    candidates.map(tag => (
                      <button
                        key={tag.id}
                        type="button"
                        onClick={() => setSelectedTarget(tag)}
                        className="flex w-full items-center justify-between gap-2 p-2 text-left text-sm hover:bg-muted/60"
                      >
                        <div className="min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="font-medium">{tag.name}</span>
                            <Badge variant="outline" className="text-xs">
                              {tag.category}
                            </Badge>
                          </div>
                          <p className="text-xs text-muted-foreground">
                            {tag.usage_count} uses
                          </p>
                        </div>
                      </button>
                    ))}
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={onClose}
            disabled={mergeMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={
              !selectedTarget ||
              mergeMutation.isPending ||
              previewLoading ||
              !!previewError
            }
          >
            {mergeMutation.isPending ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Merging...
              </>
            ) : (
              <>
                Merge
                {preview && totalMoves > 0 ? ` (${totalMoves})` : ''}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export default MergeTagDialog
