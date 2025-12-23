'use client'

import { useState } from 'react'
import { CheckCircle, Loader2, MapPin } from 'lucide-react'
import { useApproveShow } from '@/lib/hooks/useAdminShows'
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
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'

interface ApproveShowDialogProps {
  show: ShowResponse
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ApproveShowDialog({
  show,
  open,
  onOpenChange,
}: ApproveShowDialogProps) {
  const [verifyVenues, setVerifyVenues] = useState(true)
  const approveMutation = useApproveShow()

  const unverifiedVenues = show.venues.filter(v => !v.verified)
  const hasUnverifiedVenues = unverifiedVenues.length > 0

  const handleApprove = () => {
    approveMutation.mutate(
      {
        showId: show.id,
        verifyVenues: hasUnverifiedVenues ? verifyVenues : false,
      },
      {
        onSuccess: () => {
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
            <CheckCircle className="h-5 w-5 text-primary" />
            Approve Show
          </DialogTitle>
          <DialogDescription>
            Approve &quot;{showTitle}&quot; to make it visible to the public.
          </DialogDescription>
        </DialogHeader>

        {hasUnverifiedVenues && (
          <div className="space-y-4 py-4">
            <div className="rounded-lg border border-border bg-muted/50 p-3">
              <p className="text-sm font-medium mb-2">Unverified Venues</p>
              <ul className="space-y-1">
                {unverifiedVenues.map(venue => (
                  <li
                    key={venue.id}
                    className="flex items-center gap-2 text-sm text-muted-foreground"
                  >
                    <MapPin className="h-3 w-3" />
                    <span>
                      {venue.name} - {venue.city}, {venue.state}
                    </span>
                  </li>
                ))}
              </ul>
            </div>

            <div className="flex items-center space-x-2">
              <Checkbox
                id="verify-venues"
                checked={verifyVenues}
                onCheckedChange={checked => setVerifyVenues(checked === true)}
              />
              <Label
                htmlFor="verify-venues"
                className="text-sm font-normal cursor-pointer"
              >
                Also verify{' '}
                {unverifiedVenues.length === 1 ? 'this venue' : 'these venues'}{' '}
                as legitimate
              </Label>
            </div>
          </div>
        )}

        {!hasUnverifiedVenues && (
          <div className="py-4">
            <p className="text-sm text-muted-foreground">
              All venues for this show are already verified. The show will be
              immediately visible to the public after approval.
            </p>
          </div>
        )}

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
                Approve
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
