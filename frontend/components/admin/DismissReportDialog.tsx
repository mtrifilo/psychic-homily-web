'use client'

import { useState } from 'react'
import { XCircle, Loader2 } from 'lucide-react'
import { useDismissReport } from '@/lib/hooks/useAdminReports'
import type { ShowReportResponse } from '@/lib/types/show'
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

interface DismissReportDialogProps {
  report: ShowReportResponse
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function DismissReportDialog({
  report,
  open,
  onOpenChange,
}: DismissReportDialogProps) {
  const [notes, setNotes] = useState('')
  const dismissMutation = useDismissReport()

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

  const showTitle = report.show?.title || 'Unknown Show'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-muted-foreground" />
            Dismiss Report
          </DialogTitle>
          <DialogDescription>
            Dismiss this report for &quot;{showTitle}&quot;. Use this for spam,
            invalid, or duplicate reports.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="dismiss-notes">Notes (optional)</Label>
            <Textarea
              id="dismiss-notes"
              placeholder="e.g., Duplicate report, spam, report doesn't match show info..."
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
