'use client'

import { useState } from 'react'
import { Flag, Loader2, Check } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { useReportEntity } from '../hooks/useReportEntity'
import { REPORT_TYPES } from '../types'
import type { ReportableEntityType } from '../types'

interface ReportEntityDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  entityType: ReportableEntityType
  entityId: number
  /**
   * Display name for the entity being reported. The dialog quotes this
   * verbatim ("Report Issue with 'X'"), so callers should pass the
   * thing the viewer would recognise — collection title, artist name,
   * "Comment by Foo", etc. — not a raw ID.
   */
  entityName: string
  /**
   * Optional human-readable type label inserted before the entity name
   * (e.g. `entityTypeLabel="collection"` yields "Report Issue with
   * collection 'X'"). Omit when the entity name is already
   * self-describing (e.g. "Comment by Foo"). PSY-578.
   */
  entityTypeLabel?: string
}

export function ReportEntityDialog({
  open,
  onOpenChange,
  entityType,
  entityId,
  entityName,
  entityTypeLabel,
}: ReportEntityDialogProps) {
  const [selectedType, setSelectedType] = useState<string | null>(null)
  const [details, setDetails] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const reportMutation = useReportEntity()

  const reportOptions = REPORT_TYPES[entityType] ?? []

  const isDuplicateError =
    reportMutation.isError &&
    /already.*(pending|report)/i.test(
      (reportMutation.error as Error)?.message ?? ''
    )

  const handleSubmit = () => {
    if (!selectedType) return

    reportMutation.mutate(
      {
        entityType,
        entityId,
        reportType: selectedType,
        details: details.trim() || undefined,
      },
      {
        onSuccess: () => {
          setSubmitted(true)
        },
      }
    )
  }

  const handleClose = (newOpen: boolean) => {
    if (!newOpen) {
      // Reset state when closing
      setSelectedType(null)
      setDetails('')
      setSubmitted(false)
      reportMutation.reset()
    }
    onOpenChange(newOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Flag className="h-5 w-5 text-orange-500" />
            Report Issue
          </DialogTitle>
          <DialogDescription>
            Report an issue with {entityTypeLabel ? `${entityTypeLabel} ` : ''}
            &quot;{entityName}&quot;. Our team will review your report.
          </DialogDescription>
        </DialogHeader>

        {/* Success state */}
        {submitted && reportMutation.isSuccess && (
          <div className="rounded-md border border-green-800 bg-green-950/50 p-4">
            <div className="flex items-center gap-2 text-green-400">
              <Check className="h-4 w-4" />
              <span className="font-medium">Report submitted</span>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              Thank you for helping improve our data. An admin will review your report.
            </p>
          </div>
        )}

        {/* Duplicate report error — show message only, no form */}
        {isDuplicateError && (
          <div className="rounded-md border border-orange-800 bg-orange-950/50 p-4">
            <div className="flex items-center gap-2 text-orange-400">
              <Flag className="h-4 w-4" />
              <span className="font-medium">Already reported</span>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              You&apos;ve already reported this entity. An admin will review your
              existing report.
            </p>
          </div>
        )}

        {/* Other error state */}
        {reportMutation.isError && !isDuplicateError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {(reportMutation.error as Error)?.message ||
              'Failed to submit report. Please try again.'}
          </div>
        )}

        {/* Form */}
        {!submitted && !isDuplicateError && (
          <div className="space-y-4">
            {/* Report Type Selection */}
            <div className="space-y-2">
              <Label>What&apos;s the issue?</Label>
              <div className="grid gap-2">
                {reportOptions.map((option) => (
                  <button
                    key={option.value}
                    type="button"
                    onClick={() => setSelectedType(option.value)}
                    className={`flex items-start gap-3 p-3 rounded-lg border text-left transition-colors ${
                      selectedType === option.value
                        ? 'border-primary bg-primary/5'
                        : 'border-border hover:border-muted-foreground/50'
                    }`}
                  >
                    <div
                      className={`mt-0.5 h-4 w-4 rounded-full border-2 flex items-center justify-center shrink-0 ${
                        selectedType === option.value
                          ? 'border-primary'
                          : 'border-muted-foreground/50'
                      }`}
                    >
                      {selectedType === option.value && (
                        <div className="h-2 w-2 rounded-full bg-primary" />
                      )}
                    </div>
                    <div>
                      <div className="font-medium text-sm">{option.label}</div>
                      <div className="text-xs text-muted-foreground">
                        {option.description}
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            </div>

            {/* Details */}
            {selectedType && (
              <div className="space-y-2">
                <Label htmlFor="report-details">
                  Additional details (optional)
                </Label>
                <Textarea
                  id="report-details"
                  placeholder="Please provide any additional context..."
                  value={details}
                  onChange={(e) => setDetails(e.target.value)}
                  rows={3}
                />
              </div>
            )}
          </div>
        )}

        {!submitted && !isDuplicateError && (
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => handleClose(false)}
              disabled={reportMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={handleSubmit}
              disabled={!selectedType || reportMutation.isPending}
            >
              {reportMutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Submitting...
                </>
              ) : (
                <>
                  <Flag className="h-4 w-4 mr-2" />
                  Submit Report
                </>
              )}
            </Button>
          </DialogFooter>
        )}

        {(submitted || isDuplicateError) && (
          <DialogFooter>
            <Button onClick={() => handleClose(false)}>
              Close
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  )
}
