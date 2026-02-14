'use client'

import { useState } from 'react'
import { XCircle, Loader2 } from 'lucide-react'
import { useDismissArtistReport } from '@/lib/hooks/useAdminArtistReports'
import type { ArtistReportResponse } from '@/lib/types/artist'
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

interface DismissArtistReportDialogProps {
  report: ArtistReportResponse
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function DismissArtistReportDialog({
  report,
  open,
  onOpenChange,
}: DismissArtistReportDialogProps) {
  const [notes, setNotes] = useState('')
  const dismissMutation = useDismissArtistReport()

  const handleDismiss = () => {
    dismissMutation.mutate(
      {
        reportId: report.id,
        notes: notes.trim() || undefined,
      },
      {
        onSuccess: () => {
          setNotes('')
          onOpenChange(false)
        },
      }
    )
  }

  const artistName = report.artist?.name || 'Unknown Artist'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-muted-foreground" />
            Dismiss Report
          </DialogTitle>
          <DialogDescription>
            Dismiss this report for &quot;{artistName}&quot;. Use this for spam,
            invalid, or duplicate reports.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="dismiss-artist-report-notes">
              Notes (optional)
            </Label>
            <Textarea
              id="dismiss-artist-report-notes"
              placeholder="e.g., Duplicate report, spam, report doesn't match artist info..."
              value={notes}
              onChange={e => setNotes(e.target.value)}
              rows={3}
              className="resize-none"
            />
            <p className="text-xs text-muted-foreground">
              These notes are for internal record-keeping only.
            </p>
          </div>
        </div>

        {dismissMutation.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {dismissMutation.error?.message ||
              'Failed to dismiss report. Please try again.'}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={dismissMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="secondary"
            onClick={handleDismiss}
            disabled={dismissMutation.isPending}
          >
            {dismissMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Dismissing...
              </>
            ) : (
              <>
                <XCircle className="h-4 w-4 mr-2" />
                Dismiss Report
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
