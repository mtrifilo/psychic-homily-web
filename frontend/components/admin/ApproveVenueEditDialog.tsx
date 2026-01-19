'use client'

import { CheckCircle, Loader2 } from 'lucide-react'
import { useApproveVenueEdit } from '@/lib/hooks/useAdminVenueEdits'
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

interface ApproveVenueEditDialogProps {
  edit: PendingVenueEdit
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ApproveVenueEditDialog({
  edit,
  open,
  onOpenChange,
}: ApproveVenueEditDialogProps) {
  const approveMutation = useApproveVenueEdit()

  const handleApprove = () => {
    approveMutation.mutate(edit.id, {
      onSuccess: () => {
        onOpenChange(false)
      },
    })
  }

  const venueName = edit.venue?.name || `Venue #${edit.venue_id}`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <CheckCircle className="h-5 w-5 text-primary" />
            Approve Venue Edit
          </DialogTitle>
          <DialogDescription>
            Approve the proposed changes to &quot;{venueName}&quot;. The changes
            will be applied immediately.
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          <p className="text-sm text-muted-foreground">
            This will apply all proposed changes submitted by{' '}
            <span className="font-medium text-foreground">
              {edit.submitter_name || `User #${edit.submitted_by}`}
            </span>{' '}
            to the venue.
          </p>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={approveMutation.isPending}
          >
            Cancel
          </Button>
          <Button onClick={handleApprove} disabled={approveMutation.isPending}>
            {approveMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Approving...
              </>
            ) : (
              <>
                <CheckCircle className="h-4 w-4 mr-2" />
                Approve Changes
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
