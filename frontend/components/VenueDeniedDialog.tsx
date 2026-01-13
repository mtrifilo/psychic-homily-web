'use client'

import { ShieldX, CheckCircle2 } from 'lucide-react'
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

interface VenueDeniedDialogProps {
  show: ShowResponse
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function VenueDeniedDialog({
  show,
  open,
  onOpenChange,
}: VenueDeniedDialogProps) {
  const showTitle =
    show.title || show.artists.map(a => a.name).join(', ') || 'Untitled Show'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader className="text-center sm:text-center">
          <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
            <ShieldX className="h-6 w-6 text-destructive" />
          </div>
          <DialogTitle className="text-xl">Cannot Publish Show</DialogTitle>
          <DialogDescription className="text-center">
            &quot;{showTitle}&quot; cannot be published because the venue could
            not be verified. Venues may be denied for reasons such as being
            unverifiable, unlicensed, or operating illegally.
          </DialogDescription>
        </DialogHeader>

        {show.rejection_reason && (
          <div className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
            <span className="font-medium">Admin note:</span>{' '}
            {show.rejection_reason}
          </div>
        )}

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
