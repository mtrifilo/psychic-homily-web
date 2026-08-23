'use client'

import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import { useSendVerificationEmail } from '@/features/auth'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import type { ApiError } from '@/lib/api'

/**
 * Post-signup interstitial: signup already sent a verification link (PSY-1871),
 * so this is where the new account learns that, what is open in the meantime,
 * and how to get another copy of the email.
 *
 * It renders in place on /auth rather than at its own route so the address
 * never travels through a URL, and so a reload cannot strand anyone on a page
 * with no context.
 *
 * Deliberately local to `app/auth`. PSY-1901 adds a resend control to the
 * verify-email landing; the two are the same three lines of logic but sit on
 * surfaces whose copy is still moving, so they stay separate until one of them
 * settles.
 */

/** Matches the verification token TTL in backend `jwt.go: CreateVerificationToken`. */
const VERIFICATION_LINK_HOURS = 24

/** Where the Browse CTA goes when signup did not start from a returnTo flow. */
const BROWSE_HREF = '/shows'

/** The account-email fold, which is a tab on the profile page, not `/settings`. */
const ACCOUNT_SETTINGS_HREF = '/profile?tab=settings'

/**
 * Turns a failed resend into something actionable. A raw 429 body reads as
 * "the button is broken"; `Retry-After` is the only part of it a user can act
 * on, so it becomes the message.
 */
function resendFailureMessage(error: unknown): string {
  const apiError = error as ApiError | null

  if (apiError?.status === 429) {
    const seconds = apiError.retryAfter
    if (typeof seconds === 'number' && Number.isFinite(seconds) && seconds > 0) {
      return `That is a lot of resends. Try again in ${seconds}s.`
    }
    return 'That is a lot of resends. Try again in a minute.'
  }

  if (error instanceof Error && error.message) {
    return error.message
  }

  return 'Could not send the email. Try again in a moment.'
}

interface CheckInboxInterstitialProps {
  /** The address the account was created under. */
  email: string
  /**
   * The sanitized destination the signup started from. `/` means the user
   * arrived at /auth on their own rather than being sent there mid-task.
   */
  returnTo: string
}

export function CheckInboxInterstitial({
  email,
  returnTo,
}: CheckInboxInterstitialProps) {
  const resend = useSendVerificationEmail()

  // returnTo decision (PSY-1878): a user sent to /auth mid-task still needs to
  // be told an email is waiting, so the interstitial always shows. What changes
  // is the primary CTA, which returns them to what they were doing instead of
  // dropping them on a generic listing.
  const startedFromTask = returnTo !== '/'
  const primaryHref = startedFromTask ? returnTo : BROWSE_HREF
  const primaryLabel = startedFromTask
    ? 'Continue where you left off'
    : 'Browse upcoming shows'

  return (
    <Card className="gap-4 px-10 py-9">
      <p className="font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground">
        One email sent · one click to finish
      </p>

      <h1 className="font-display text-[26px] font-bold leading-tight text-foreground">
        Check your inbox.
      </h1>

      <p className="text-sm leading-[22px] text-foreground">
        We sent a verification link to{' '}
        <span className="font-mono break-words">{email}</span>. It expires in{' '}
        {VERIFICATION_LINK_HOURS} hours.
      </p>

      <div className="space-y-1.5 border border-border bg-background px-4 py-3">
        <p className="font-mono text-[11px] uppercase tracking-[0.55px] text-muted-foreground">
          Meanwhile, everything else is open
        </p>
        {/*
          Kept to what the code actually does today: browsing, saving, and
          following are authenticated-only, and email verification gates show
          submission. Follow-driven alert delivery is PSY-1896 and is still in
          the backlog, so this does not promise that verifying switches alerts on.
        */}
        <p className="text-[13px] leading-5 text-foreground">
          Browse shows, save what you like, follow artists and venues: all of
          that works right now. Verifying unlocks show submission.
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Button asChild>
          <Link href={primaryHref}>{primaryLabel}</Link>
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() => resend.mutate()}
          disabled={resend.isPending}
        >
          {resend.isPending ? (
            <>
              <Loader2 className="h-4 w-4 animate-spin" />
              Sending...
            </>
          ) : (
            'Resend email'
          )}
        </Button>
      </div>

      {resend.isSuccess && (
        <p className="text-sm text-success-foreground">
          Sent again. Give it a minute to arrive.
        </p>
      )}
      {resend.isError && (
        <p role="alert" className="text-sm text-destructive">
          {resendFailureMessage(resend.error)}
        </p>
      )}

      <p className="text-xs text-muted-foreground">
        Wrong address? Check it in{' '}
        <Link
          href={ACCOUNT_SETTINGS_HREF}
          className="underline underline-offset-4 hover:text-primary"
        >
          Settings
        </Link>
        .
      </p>
    </Card>
  )
}
