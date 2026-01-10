'use client'

import { Clock, EyeOff, CheckCircle2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

type SubmissionStatus = 'pending' | 'private' | null

interface SubmissionSuccessDialogProps {
  status: SubmissionStatus
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SubmissionSuccessDialog({
  status,
  open,
  onOpenChange,
}: SubmissionSuccessDialogProps) {
  if (!status) return null

  const isPending = status === 'pending'
  const isPrivate = status === 'private'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader className="text-center sm:text-center">
          {isPending && (
            <>
              <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-amber-500/10">
                <Clock className="h-6 w-6 text-amber-500" />
              </div>
              <DialogTitle className="text-xl">Pending Review</DialogTitle>
              <DialogDescription className="text-center">
                Your show includes a new venue that needs admin verification.
                It will appear publicly once approved. In the meantime, you can
                find it in your list below.
              </DialogDescription>
            </>
          )}

          {isPrivate && (
            <>
              <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-slate-500/10">
                <EyeOff className="h-6 w-6 text-slate-500" />
              </div>
              <DialogTitle className="text-xl">Private Show Added</DialogTitle>
              <DialogDescription className="text-center">
                Your show has been saved to your personal list. It will only be
                visible to you and won&apos;t appear in public listings.
              </DialogDescription>
            </>
          )}
        </DialogHeader>

        <DialogFooter className="sm:justify-center">
          <Button
            onClick={() => onOpenChange(false)}
            className="w-full sm:w-auto"
          >
            <CheckCircle2 className="h-4 w-4 mr-2" />
            Got it
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
