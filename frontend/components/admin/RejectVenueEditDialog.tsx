'use client'

import { useState } from 'react'
import { XCircle, Loader2 } from 'lucide-react'
import { useRejectVenueEdit } from '@/lib/hooks/useAdminVenueEdits'
import type { PendingVenueEdit } from '@/lib/types/venue'
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

interface RejectVenueEditDialogProps {
  edit: PendingVenueEdit
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function RejectVenueEditDialog({
  edit,
  open,
  onOpenChange,
}: RejectVenueEditDialogProps) {
  const [reason, setReason] = useState('')
  const rejectMutation = useRejectVenueEdit()

  const handleReject = () => {
    if (!reason.trim()) return

    rejectMutation.mutate(
      {
        editId: edit.id,
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

  const venueName = edit.venue?.name || `Venue #${edit.venue_id}`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-destructive" />
            Reject Venue Edit
          </DialogTitle>
          <DialogDescription>
            Reject the proposed changes to &quot;{venueName}&quot;. Please
            provide a reason for the rejection.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="rejection-reason">Reason for rejection</Label>
            <Textarea
              id="rejection-reason"
              placeholder="e.g., Inaccurate information, spam submission, duplicate request..."
              value={reason}
              onChange={e => setReason(e.target.value)}
              rows={3}
              className="resize-none"
            />
            <p className="text-xs text-muted-foreground">
              This reason will be saved with the edit request for
              record-keeping.
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
                Reject Edit
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
