'use client'

import { EyeOff, Loader2 } from 'lucide-react'
import { useShowMakePrivate } from '@/lib/hooks/useShowMakePrivate'
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

interface MakePrivateDialogProps {
  show: ShowResponse
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function MakePrivateDialog({
  show,
  open,
  onOpenChange,
  onSuccess,
}: MakePrivateDialogProps) {
  const makePrivateMutation = useShowMakePrivate()

  const handleMakePrivate = () => {
    makePrivateMutation.mutate(show.id, {
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
            <EyeOff className="h-5 w-5 text-slate-500" />
            Make Private
          </DialogTitle>
          <DialogDescription>
            Make &quot;{showTitle}&quot; private? It will only be visible to you
            and won&apos;t appear in public listings.
          </DialogDescription>
        </DialogHeader>

        {makePrivateMutation.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {makePrivateMutation.error?.message ||
              'Failed to make show private. Please try again.'}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={makePrivateMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="secondary"
            className="bg-slate-500/10 text-slate-600 hover:bg-slate-500/20 hover:text-slate-600 dark:text-slate-400 dark:hover:text-slate-400"
            onClick={handleMakePrivate}
            disabled={makePrivateMutation.isPending}
          >
            {makePrivateMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Making Private...
              </>
            ) : (
              <>
                <EyeOff className="h-4 w-4 mr-2" />
                Make Private
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
