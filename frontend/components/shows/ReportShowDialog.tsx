'use client'

import { useState } from 'react'
import { Flag, Loader2, AlertCircle, BanIcon, CalendarX } from 'lucide-react'
import { useReportShow } from '@/lib/hooks/useShowReports'
import type { ShowReportType } from '@/lib/types/show'
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

const REPORT_OPTIONS: {
  value: ShowReportType
  label: string
  description: string
  icon: React.ReactNode
}[] = [
  {
    value: 'cancelled',
    label: 'Cancelled',
    description: 'This show has been cancelled',
    icon: <CalendarX className="h-5 w-5" />,
  },
  {
    value: 'sold_out',
    label: 'Sold Out',
    description: 'This show is sold out',
    icon: <BanIcon className="h-5 w-5" />,
  },
  {
    value: 'inaccurate',
    label: 'Inaccurate Info',
    description: 'Some information is incorrect',
    icon: <AlertCircle className="h-5 w-5" />,
  },
]

interface ReportShowDialogProps {
  showId: number
  showTitle: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function ReportShowDialog({
  showId,
  showTitle,
  open,
  onOpenChange,
  onSuccess,
}: ReportShowDialogProps) {
  const [selectedType, setSelectedType] = useState<ShowReportType | null>(null)
  const [details, setDetails] = useState('')
  const reportMutation = useReportShow()

  const handleSubmit = () => {
    if (!selectedType) return

    reportMutation.mutate(
      {
        showId,
        reportType: selectedType,
        details: details.trim() || undefined,
      },
      {
        onSuccess: () => {
          onOpenChange(false)
          setSelectedType(null)
          setDetails('')
          onSuccess?.()
        },
      }
    )
  }

  const handleClose = (newOpen: boolean) => {
    if (!newOpen) {
      setSelectedType(null)
      setDetails('')
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
            Report an issue with &quot;{showTitle}&quot;. Our team will review your
            report.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* Report Type Selection */}
          <div className="space-y-2">
            <Label>What&apos;s the issue?</Label>
            <div className="grid gap-2">
              {REPORT_OPTIONS.map(option => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => setSelectedType(option.value)}
                  className={`flex items-center gap-3 p-3 rounded-lg border text-left transition-colors ${
                    selectedType === option.value
                      ? 'border-primary bg-primary/5'
                      : 'border-border hover:border-muted-foreground/50'
                  }`}
                >
                  <div
                    className={`${
                      selectedType === option.value
                        ? 'text-primary'
                        : 'text-muted-foreground'
                    }`}
                  >
                    {option.icon}
                  </div>
                  <div>
                    <div className="font-medium">{option.label}</div>
                    <div className="text-sm text-muted-foreground">
                      {option.description}
                    </div>
                  </div>
                </button>
              ))}
            </div>
          </div>

          {/* Details (optional, shown for all types but emphasized for inaccurate) */}
          {selectedType && (
            <div className="space-y-2">
              <Label htmlFor="details">
                Additional details{' '}
                {selectedType === 'inaccurate' ? '(recommended)' : '(optional)'}
              </Label>
              <Textarea
                id="details"
                placeholder={
                  selectedType === 'inaccurate'
                    ? 'Please describe what information is incorrect...'
                    : 'Any additional information...'
                }
                value={details}
                onChange={e => setDetails(e.target.value)}
                rows={3}
              />
            </div>
          )}
        </div>

        {reportMutation.isError && (
          <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
            {reportMutation.error?.message ||
              'Failed to submit report. Please try again.'}
          </div>
        )}

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
      </DialogContent>
    </Dialog>
  )
}
