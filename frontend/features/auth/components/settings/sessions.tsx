'use client'

import { useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { useSignOutEverywhereElse } from '@/features/auth'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { AlertCircle, AlertTriangle, CheckCircle2, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { SETTINGS_ANCHOR_SCROLL_MT, useAnchorScroll } from './useAnchorScroll'

/** The Settings sessions card's fragment. */
export const SESSIONS_SETTINGS_ANCHOR = 'sessions'

/** The Settings sessions card's address, where a link about signed-in devices lands. */
export const SESSIONS_SETTINGS_HREF = `/profile?tab=settings#${SESSIONS_SETTINGS_ANCHOR}`

/**
 * Sessions card: signs out every other device and browser, and keeps this
 * one signed in, because the response that signs the others out renews this
 * browser's session in the same step.
 *
 * Calendar-feed tokens are separate credentials the backend leaves in place,
 * which is what the line under the button tells the reader.
 */
export function SessionsSettings() {
  const signOutEverywhereElse = useSignOutEverywhereElse()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const anchorRef = useAnchorScroll(SESSIONS_SETTINGS_ANCHOR)

  const handleConfirm = () => {
    signOutEverywhereElse.mutate(undefined, {
      // Closed either way, so the outcome line on the card is what the reader
      // sees next.
      onSettled: () => setConfirmOpen(false),
      onError: error => {
        Sentry.captureException(error, {
          level: 'warning',
          tags: { service: 'settings', error_type: 'sign_out_everywhere_else' },
        })
      },
    })
  }

  return (
    <Card
      ref={anchorRef}
      id={SESSIONS_SETTINGS_ANCHOR}
      tabIndex={-1}
      className={cn(SETTINGS_ANCHOR_SCROLL_MT, 'focus:outline-none')}
    >
      <CardHeader>
        <CardTitle className="text-base">Sessions</CardTitle>
        <CardDescription>
          Sign out on every other device and browser where you&apos;re signed in.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {signOutEverywhereElse.isSuccess && (
          <div
            role="status"
            className="flex items-center gap-2 rounded-md bg-success p-3 text-sm text-success-foreground"
          >
            <CheckCircle2 className="h-4 w-4 shrink-0" />
            <span>Signed out everywhere else. This device is still signed in.</span>
          </div>
        )}

        {signOutEverywhereElse.isError && (
          <div
            role="alert"
            className="flex items-center gap-2 rounded-md bg-destructive/10 p-3 text-sm text-destructive"
          >
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>Failed to sign out other devices. Please try again.</span>
          </div>
        )}

        <Button
          variant="outline"
          size="sm"
          onClick={() => setConfirmOpen(true)}
          disabled={signOutEverywhereElse.isPending}
        >
          Sign out everywhere else
        </Button>

        <p className="text-xs text-muted-foreground">
          This device stays signed in. Calendar feeds keep working.
        </p>

        <Dialog
          open={confirmOpen}
          onOpenChange={open => {
            if (!signOutEverywhereElse.isPending) setConfirmOpen(open)
          }}
        >
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2 text-destructive">
                <AlertTriangle className="h-5 w-5" />
                Sign out everywhere else?
              </DialogTitle>
              <DialogDescription>
                Every other device and browser signed in to your account will be
                signed out and will need to sign in again. This device stays
                signed in. Calendar feeds keep working.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                variant="outline"
                onClick={() => setConfirmOpen(false)}
                disabled={signOutEverywhereElse.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={handleConfirm}
                disabled={signOutEverywhereElse.isPending}
              >
                {signOutEverywhereElse.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" aria-label="Signing out" />
                ) : (
                  'Sign out everywhere else'
                )}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  )
}
