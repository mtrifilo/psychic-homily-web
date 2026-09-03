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
import { buildAuthHref } from '@/lib/auth-href'

interface LoginPromptDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title?: string
  description?: string
  /**
   * The sign-in destination, already built. The caller passes an href rather
   * than a bare `returnTo` because the canonical destination is resolved when
   * the prompt OPENS (`useAuthGatedAction`), while this dialog is mounted
   * closed in server markup, where there is no location to resolve it from.
   */
  authHref?: string
}

export function LoginPromptDialog({
  open,
  onOpenChange,
  title = 'Sign in required',
  description = 'You need to be signed in to perform this action.',
  authHref = buildAuthHref('/'),
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
            <Link href={authHref}>
              <LogIn className="h-4 w-4 mr-2" />
              Sign in
            </Link>
          </Button>

          <Button variant="outline" asChild>
            <Link href={`${authHref}#signup`}>
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
