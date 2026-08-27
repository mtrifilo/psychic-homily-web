'use client'

import { useEffect, useRef } from 'react'
import Link from 'next/link'
// Imported by module path, not through the `@/features/auth` barrel, so a suite
// that mocks the barrel still runs the real resend control.
import {
  VerificationResend,
  VerificationResendAlerts,
  VerificationResendButton,
  VerificationResendStatus,
} from '@/features/auth/components/verification-resend'
import { buildAuthHref } from '@/lib/auth-href'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'

/**
 * Post-signup interstitial: signup already sent a verification link (PSY-1871),
 * so this is where the new account learns that, what is open in the meantime,
 * and how to get another copy of the email.
 *
 * It renders in place on /auth rather than at its own route so the address
 * never travels through a URL, and so a reload cannot strand anyone on a page
 * with no context.
 *
 * The resend button is the shared `<VerificationResend>` control (PSY-1911).
 * This surface used to carry its own handler and its own 429 wording, which is
 * exactly the drift that control exists to remove.
 */

/** Matches the verification token TTL in backend `jwt.go: CreateVerificationToken`. */
const VERIFICATION_LINK_HOURS = 24

/** Where the Browse CTA goes when signup did not start from a returnTo flow. */
const BROWSE_HREF = '/shows'

/** The account-email fold, which is a tab on the profile page, not `/settings`. */
const ACCOUNT_SETTINGS_HREF = '/profile?tab=settings'

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
  const headingRef = useRef<HTMLHeadingElement>(null)

  // This surface replaces the signup card in place rather than navigating, so
  // it inherits none of the App Router's route announcement, and the submit
  // button that had focus unmounts underneath the user. Without this a screen
  // reader user hears nothing and a keyboard user is dropped at the top of the
  // document with no sign that the account was created.
  useEffect(() => {
    headingRef.current?.focus()
  }, [])

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

      <h1
        ref={headingRef}
        tabIndex={-1}
        className="font-display text-[26px] font-bold leading-tight text-foreground focus:outline-none"
      >
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

      <VerificationResend
        service="auth_signup"
        signInHref={buildAuthHref(primaryHref)}
      >
        <div className="flex flex-wrap items-center gap-3">
          <Button asChild>
            <Link href={primaryHref}>{primaryLabel}</Link>
          </Button>
          <VerificationResendButton variant="outline">
            Resend email
          </VerificationResendButton>
        </div>

        <VerificationResendStatus className="font-mono text-[11px] uppercase tracking-[0.55px] text-primary" />
        <VerificationResendAlerts />
      </VerificationResend>

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
