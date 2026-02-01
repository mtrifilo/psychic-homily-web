'use client'

import { Trash2, Loader2 } from 'lucide-react'
import { useShowDelete } from '@/lib/hooks/useShowDelete'
import type { ShowResponse } from '@/lib/types/show'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

interface DeleteShowDialogProps {
  show: ShowResponse
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function DeleteShowDialog({
  show,
  open,
  onOpenChange,
  onSuccess,
}: DeleteShowDialogProps) {
  const deleteMutation = useShowDelete()

  const handleDelete = () => {
    deleteMutation.mutate(show.id, {
      onSuccess: () => {
        onOpenChange(false)
        onSuccess?.()
      },
    })
  }

  const showTitle =
    show.title || show.artists.map(a => a.name).join(', ') || 'Untitled Show'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Trash2 className="h-5 w-5 text-destructive" />
            Delete Show
          </DialogTitle>
          <DialogDescription>
            Are you sure you want to delete &quot;{showTitle}&quot;? This action
            cannot be undone.
          </DialogDescription>
        </DialogHeader>

        {deleteMutation.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {deleteMutation.error?.message ||
              'Failed to delete show. Please try again.'}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
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
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Deleting...
              </>
            ) : (
              <>
                <Trash2 className="h-4 w-4 mr-2" />
                Delete Show
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
