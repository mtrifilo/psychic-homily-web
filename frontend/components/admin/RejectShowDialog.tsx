'use client'

import { useState } from 'react'
import { XCircle, Loader2 } from 'lucide-react'
import { useRejectShow } from '@/lib/hooks/useAdminShows'
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
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

interface RejectShowDialogProps {
  show: ShowResponse
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function RejectShowDialog({
  show,
  open,
  onOpenChange,
}: RejectShowDialogProps) {
  const [reason, setReason] = useState('')
  const rejectMutation = useRejectShow()

  const handleReject = () => {
    if (!reason.trim()) return

    rejectMutation.mutate(
      {
        showId: show.id,
        reason: reason.trim(),
      },
      {
        onSuccess: () => {
          setReason('')
          onOpenChange(false)
        },
      }
    )
  }

  const showTitle =
    show.title || show.artists.map(a => a.name).join(', ') || 'Untitled Show'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-destructive" />
            Reject Show
          </DialogTitle>
          <DialogDescription>
            Reject &quot;{showTitle}&quot;. Please provide a reason for the
            submitter.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="rejection-reason">Reason for rejection</Label>
            <Textarea
              id="rejection-reason"
              placeholder="e.g., Venue could not be verified, suspected fake listing, duplicate submission..."
              value={reason}
              onChange={e => setReason(e.target.value)}
              rows={3}
              className="resize-none"
            />
            <p className="text-xs text-muted-foreground">
              This reason will be saved with the submission for record-keeping.
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={rejectMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleReject}
            disabled={!reason.trim() || rejectMutation.isPending}
          >
            {rejectMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Rejecting...
              </>
            ) : (
              <>
                <XCircle className="h-4 w-4 mr-2" />
                Reject Show
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
