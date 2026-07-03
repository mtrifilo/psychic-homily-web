'use client'

import { Globe, Loader2, AlertTriangle } from 'lucide-react'
import { useShowPublish } from '../hooks/useShowPublish'
import type { ShowResponse } from '../types'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { showDisplayTitle } from '@/lib/utils/showDisplayTitle'

interface PublishShowDialogProps {
  show: ShowResponse
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function PublishShowDialog({
  show,
  open,
  onOpenChange,
  onSuccess,
}: PublishShowDialogProps) {
  const publishMutation = useShowPublish()

  const handlePublish = () => {
    publishMutation.mutate(show.id, {
      onSuccess: () => {
        onOpenChange(false)
        onSuccess?.()
      },
    })
  }

  const showTitle =
    showDisplayTitle(show.title, show.artists.map(a => a.name))

  // Check if any venue is unverified
  const hasUnverifiedVenue = show.venues?.some(v => !v.verified)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Globe className="h-5 w-5 text-success-foreground" />
            Publish Show
          </DialogTitle>
          <DialogDescription>
            Publish &quot;{showTitle}&quot; to make it visible to everyone?
          </DialogDescription>
        </DialogHeader>

        {hasUnverifiedVenue && (
          <div className="rounded-md bg-pending p-3 text-sm text-pending-foreground flex items-start gap-2">
            <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>
              This show has an unverified venue. It will be set to
              &quot;Pending&quot; for admin review before appearing publicly.
            </span>
          </div>
        )}

        {publishMutation.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {publishMutation.error?.message ||
              'Failed to publish show. Please try again.'}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={publishMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="success"
            onClick={handlePublish}
            disabled={publishMutation.isPending}
          >
            {publishMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Publishing...
              </>
            ) : (
              <>
                <Globe className="h-4 w-4 mr-2" />
                {hasUnverifiedVenue ? 'Submit for Review' : 'Publish'}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
