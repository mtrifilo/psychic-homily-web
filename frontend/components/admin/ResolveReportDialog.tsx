'use client'

import { useState, useEffect } from 'react'
import { CheckCircle, Loader2 } from 'lucide-react'
import { useResolveReport } from '@/lib/hooks/useAdminReports'
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
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

interface ResolveReportDialogProps {
  report: ShowReportResponse
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ResolveReportDialog({
  report,
  open,
  onOpenChange,
}: ResolveReportDialogProps) {
  const [notes, setNotes] = useState('')
  const [setShowFlag, setSetShowFlag] = useState(true)
  const resolveMutation = useResolveReport()

  // Check if this is a cancelled or sold_out report (flag can be set)
  const canSetFlag = report.report_type === 'cancelled' || report.report_type === 'sold_out'
  const flagLabel = report.report_type === 'cancelled' ? 'Mark show as Cancelled' : 'Mark show as Sold Out'

  // Reset flag state when dialog opens
  useEffect(() => {
    if (open) {
      setNotes('')
      setSetShowFlag(true)
    }
  }, [open])

  const handleResolve = () => {
    resolveMutation.mutate(
      {
        reportId: report.id,
        notes: notes.trim() || undefined,
        setShowFlag: canSetFlag ? setShowFlag : undefined,
      },
      {
        onSuccess: () => {
          setNotes('')
          setSetShowFlag(true)
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
            <CheckCircle className="h-5 w-5 text-green-500" />
            Resolve Report
          </DialogTitle>
          <DialogDescription>
            Mark this report for &quot;{showTitle}&quot; as resolved. Use this
            after taking action (e.g., updating show info, marking show as
            cancelled).
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Show flag checkbox for cancelled/sold_out reports */}
          {canSetFlag && (
            <div className="flex items-center space-x-2 p-3 rounded-md border border-border bg-muted/50">
              <Checkbox
                id="set-show-flag"
                checked={setShowFlag}
                onCheckedChange={(checked) => setSetShowFlag(checked === true)}
              />
              <Label
                htmlFor="set-show-flag"
                className="text-sm font-medium leading-none cursor-pointer"
              >
                {flagLabel}
              </Label>
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="resolve-notes">Action taken (optional)</Label>
            <Textarea
              id="resolve-notes"
              placeholder="e.g., Updated show date, marked show as cancelled, contacted venue for confirmation..."
              value={notes}
              onChange={e => setNotes(e.target.value)}
              rows={3}
              className="resize-none"
            />
            <p className="text-xs text-muted-foreground">
              Document what action was taken to address this report.
            </p>
          </div>
        </div>

        {resolveMutation.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {resolveMutation.error?.message ||
              'Failed to resolve report. Please try again.'}
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={resolveMutation.isPending}
          >
            Cancel
          </Button>
          <Button onClick={handleResolve} disabled={resolveMutation.isPending}>
            {resolveMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Resolving...
              </>
            ) : (
              <>
                <CheckCircle className="h-4 w-4 mr-2" />
                Mark as Resolved
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
