'use client'

import { useState } from 'react'
import { CheckCircle, Loader2 } from 'lucide-react'
import { useResolveArtistReport } from '@/lib/hooks/useAdminArtistReports'
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

interface ResolveArtistReportDialogProps {
  report: ArtistReportResponse
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ResolveArtistReportDialog({
  report,
  open,
  onOpenChange,
}: ResolveArtistReportDialogProps) {
  const [notes, setNotes] = useState('')
  const resolveMutation = useResolveArtistReport()

  const handleResolve = () => {
    resolveMutation.mutate(
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
            <CheckCircle className="h-5 w-5 text-green-500" />
            Resolve Report
          </DialogTitle>
          <DialogDescription>
            Mark this report for &quot;{artistName}&quot; as resolved. Use this
            after taking action (e.g., updating artist info, removing the
            artist page).
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="resolve-artist-report-notes">
              Action taken (optional)
            </Label>
            <Textarea
              id="resolve-artist-report-notes"
              placeholder="e.g., Updated artist info, removed artist page, contacted artist..."
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
