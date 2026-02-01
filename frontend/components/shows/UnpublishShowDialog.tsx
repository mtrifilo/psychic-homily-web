'use client'

import { EyeOff, Loader2 } from 'lucide-react'
import { useShowUnpublish } from '@/lib/hooks/useShowUnpublish'
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

interface UnpublishShowDialogProps {
  show: ShowResponse
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function UnpublishShowDialog({
  show,
  open,
  onOpenChange,
  onSuccess,
}: UnpublishShowDialogProps) {
  const unpublishMutation = useShowUnpublish()

  const handleUnpublish = () => {
    unpublishMutation.mutate(show.id, {
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
            <EyeOff className="h-5 w-5 text-amber-500" />
            Unpublish Show
          </DialogTitle>
          <DialogDescription>
            Are you sure you want to unpublish &quot;{showTitle}&quot;? It will
            become private and only visible to you.
          </DialogDescription>
        </DialogHeader>

        {unpublishMutation.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {unpublishMutation.error?.message ||
              'Failed to unpublish show. Please try again.'}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={unpublishMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="secondary"
            className="bg-amber-500/10 text-amber-600 hover:bg-amber-500/20 hover:text-amber-600 dark:text-amber-400 dark:hover:text-amber-400"
            onClick={handleUnpublish}
            disabled={unpublishMutation.isPending}
          >
            {unpublishMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Unpublishing...
              </>
            ) : (
              <>
                <EyeOff className="h-4 w-4 mr-2" />
                Unpublish
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
