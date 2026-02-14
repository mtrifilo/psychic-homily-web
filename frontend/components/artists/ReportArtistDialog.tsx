'use client'

import { useState } from 'react'
import { Flag, Loader2, AlertCircle, UserX } from 'lucide-react'
import { useReportArtist } from '@/lib/hooks/useArtistReports'
import type { ArtistReportType } from '@/lib/types/artist'
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
  value: ArtistReportType
  label: string
  description: string
  icon: React.ReactNode
}[] = [
  {
    value: 'inaccurate',
    label: 'Inaccurate Info',
    description: 'Some information on this page is incorrect',
    icon: <AlertCircle className="h-5 w-5" />,
  },
  {
    value: 'removal_request',
    label: 'Removal Request',
    description: "I'm the artist and want this page removed",
    icon: <UserX className="h-5 w-5" />,
  },
]

interface ReportArtistDialogProps {
  artistId: number
  artistName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function ReportArtistDialog({
  artistId,
  artistName,
  open,
  onOpenChange,
  onSuccess,
}: ReportArtistDialogProps) {
  const [selectedType, setSelectedType] = useState<ArtistReportType | null>(
    null
  )
  const [details, setDetails] = useState('')
  const reportMutation = useReportArtist()

  const handleSubmit = () => {
    if (!selectedType) return

    reportMutation.mutate(
      {
        artistId,
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
            Report an issue with &quot;{artistName}&quot;. Our team will review
            your report.
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

          {/* Details */}
          {selectedType && (
            <div className="space-y-2">
              <Label htmlFor="details">
                Additional details{' '}
                {selectedType === 'inaccurate'
                  ? '(recommended)'
                  : '(optional)'}
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
