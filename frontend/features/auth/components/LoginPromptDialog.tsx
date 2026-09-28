'use client'

import Link from 'next/link'
import { LogIn, UserPlus } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { AuthGateHrefs } from '@/lib/hooks/common/useAuthGatedAction'

interface LoginPromptDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title?: string
  description?: string
  /**
   * Both sign-in destinations, already built. Required, and hrefs rather than
   * a bare `returnTo`, because the caller resolves them at click time from the
   * browser's own location (`useAuthGatedAction`). A default here could only
   * be a destination that discards where the reader was, which is the bug the
   * required prop exists to make unwritable.
   */
  hrefs: AuthGateHrefs
}

export function LoginPromptDialog({
  open,
  onOpenChange,
  title = 'Sign in required',
  description = 'You need to be signed in to perform this action.',
  hrefs,
}: LoginPromptDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-3 pt-4">
          <Button asChild>
            <Link href={hrefs.signInHref}>
              <LogIn className="h-4 w-4 mr-2" />
              Sign in
            </Link>
          </Button>

          <Button variant="outline" asChild>
            <Link href={hrefs.createAccountHref}>
              <UserPlus className="h-4 w-4 mr-2" />
              Create account
            </Link>
          </Button>

          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            className="text-muted-foreground"
          >
            Cancel
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
