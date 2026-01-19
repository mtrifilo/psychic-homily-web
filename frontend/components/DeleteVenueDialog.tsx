'use client'

import { Trash2, Loader2 } from 'lucide-react'
import { useVenueDelete } from '@/lib/hooks/useVenueEdit'
import type { Venue, VenueWithShowCount } from '@/lib/types/venue'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

interface DeleteVenueDialogProps {
  venue: Venue | VenueWithShowCount
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function DeleteVenueDialog({
  venue,
  open,
  onOpenChange,
  onSuccess,
}: DeleteVenueDialogProps) {
  const deleteMutation = useVenueDelete()

  const handleDelete = () => {
    deleteMutation.mutate(venue.id, {
      onSuccess: () => {
        onOpenChange(false)
        onSuccess?.()
      },
    })
  }

  // Check if error is due to associated shows
  const errorMessage = deleteMutation.error?.message || ''
  const hasAssociatedShows = errorMessage.includes('associated with')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Trash2 className="h-5 w-5 text-destructive" />
            Delete Venue
          </DialogTitle>
          <DialogDescription>
            Are you sure you want to delete &quot;{venue.name}&quot;? This
            action cannot be undone.
          </DialogDescription>
        </DialogHeader>

        {deleteMutation.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {hasAssociatedShows
              ? 'This venue cannot be deleted because it has associated shows. Please delete or reassign the shows first.'
              : errorMessage || 'Failed to delete venue. Please try again.'}
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
                Delete Venue
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
