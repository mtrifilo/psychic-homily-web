'use client'

import { useState } from 'react'
import { XCircle, Loader2 } from 'lucide-react'
import { useBatchRejectShows } from '@/lib/hooks/useAdminShows'
import type { RejectionCategory } from '@/lib/types/show'
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

const REJECTION_CATEGORIES: { value: RejectionCategory; label: string }[] = [
  { value: 'non_music', label: 'Not a music event' },
  { value: 'duplicate', label: 'Duplicate listing' },
  { value: 'bad_data', label: 'Bad / inaccurate data' },
  { value: 'past_event', label: 'Past event' },
  { value: 'other', label: 'Other' },
]

interface BatchRejectDialogProps {
  showIds: number[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
  /** Pre-selected category for quick-reject flows */
  defaultCategory?: RejectionCategory
  /** Pre-filled reason for quick-reject flows */
  defaultReason?: string
}

export function BatchRejectDialog({
  showIds,
  open,
  onOpenChange,
  onSuccess,
  defaultCategory,
  defaultReason,
}: BatchRejectDialogProps) {
  const [category, setCategory] = useState<RejectionCategory | ''>(defaultCategory ?? '')
  const [reason, setReason] = useState(defaultReason ?? '')
  const batchRejectMutation = useBatchRejectShows()

  const handleReject = () => {
    if (!reason.trim()) return

    batchRejectMutation.mutate(
      {
        showIds,
        reason: reason.trim(),
        category: category || undefined,
      },
      {
        onSuccess: () => {
          setCategory('')
          setReason('')
          onOpenChange(false)
          onSuccess?.()
        },
      }
    )
  }

  // Reset state when dialog opens with new defaults
  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setCategory(defaultCategory ?? '')
      setReason(defaultReason ?? '')
    }
    onOpenChange(nextOpen)
  }

  const count = showIds.length

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-destructive" />
            Reject {count} {count === 1 ? 'Show' : 'Shows'}
          </DialogTitle>
          <DialogDescription>
            This will reject {count} selected {count === 1 ? 'show' : 'shows'}. Provide a reason
            and optional category.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="rejection-category">Category</Label>
            <select
              id="rejection-category"
              value={category}
              onChange={e => setCategory(e.target.value as RejectionCategory | '')}
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              <option value="">Select a category...</option>
              {REJECTION_CATEGORIES.map(cat => (
                <option key={cat.value} value={cat.value}>
                  {cat.label}
                </option>
              ))}
            </select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="batch-rejection-reason">Reason</Label>
            <Textarea
              id="batch-rejection-reason"
              placeholder="e.g., Not a music event, duplicate listing..."
              value={reason}
              onChange={e => setReason(e.target.value)}
              rows={3}
              className="resize-none"
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={batchRejectMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleReject}
            disabled={!reason.trim() || batchRejectMutation.isPending}
          >
            {batchRejectMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Rejecting...
              </>
            ) : (
              <>
                <XCircle className="h-4 w-4 mr-2" />
                Reject {count} {count === 1 ? 'Show' : 'Shows'}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
