'use client'

import { useState } from 'react'
import Link from 'next/link'
import * as Sentry from '@sentry/nextjs'
import { Loader2, Music } from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useAuthRouteGuard } from '@/lib/hooks/common/useAuthRouteGuard'
import { useSendVerificationEmail } from '@/features/auth'
// Imported by module path, not through the `@/features/auth` barrel, so a suite
// that mocks the barrel still runs the real countdown.
import {
  VERIFICATION_RESEND_COOLDOWN_SECONDS,
  formatResendStatus,
  isVerificationResendUnauthorized,
  resendStatusAnnouncement,
  useVerificationResendCooldown,
  verificationResendRetryAfter,
} from '@/features/auth/hooks/useVerificationResendCooldown'
import { buildAuthHref } from '@/lib/auth-href'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

/** Where a reader whose session died mid-gate is sent to get a new one. */
const SIGN_IN_HREF = buildAuthHref('/shows/submit')
import { AIFormFiller, ShowForm } from '@/features/shows'
import type { ExtractedShowData } from '@/lib/types/extraction'

/**
 * Submission desk gate: shown to a signed-in but unverified user.
 *
 * The resend button is here, at the point of blockage, rather than only in
 * Settings (PSY-1871). Signup now emails the link, so the common case is "it
 * expired" or "I never got it", and sending the user two hops away to Settings
 * to fix that loses most of them.
 *
 * The framing is the desk, not spam hygiene: the earlier copy argued the
 * platform's case ("real users", "spam-free") to someone who had already
 * decided to contribute. Naming the consequence (submissions land straight on
 * the shared calendar) explains the same requirement without the lecture.
 */
function EmailVerificationRequired() {
  const sendVerificationEmail = useSendVerificationEmail()
  const cooldown = useVerificationResendCooldown()
  const [emailSent, setEmailSent] = useState(false)
  const [sendFailed, setSendFailed] = useState(false)
  const [sessionExpired, setSessionExpired] = useState(false)
  const status = formatResendStatus(emailSent, cooldown.secondsRemaining)
  const announcement = resendStatusAnnouncement(emailSent, cooldown.isCoolingDown)

  const handleResend = async () => {
    if (sendVerificationEmail.isPending || cooldown.isCoolingDown) {
      return
    }
    setSendFailed(false)
    try {
      await sendVerificationEmail.mutateAsync()
      setEmailSent(true)
      cooldown.start(VERIFICATION_RESEND_COOLDOWN_SECONDS)
    } catch (error) {
      const retryAfter = verificationResendRetryAfter(error)
      if (retryAfter !== null) {
        // Throttled, not broken: park the control rather than raise an alert.
        cooldown.start(retryAfter)
        return
      }
      if (isVerificationResendUnauthorized(error)) {
        // The session died while this gate sat open. Point at sign-in rather
        // than a generic failure, and do not page on-call for an expiry.
        setSessionExpired(true)
        return
      }
      setSendFailed(true)
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'shows_submit', error_type: 'verification_email' },
      })
    }
  }

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-xl">
        <div className="flex flex-col items-start gap-4 border border-border bg-card px-6 py-8 sm:px-11 sm:py-10">
          <p className="font-mono text-[11px] uppercase tracking-[0.66px] text-muted-foreground">
            Submission desk · Verification needed
          </p>
          <h1 className="font-display text-[26px] font-bold text-foreground">
            One step before you post.
          </h1>
          {/* Two softenings from the mock. "Straight onto" overstated it:
              lower-tier submissions land in the review queue first. And "we
              sent you a link at signup" is only true for accounts created
              after PSY-1871 shipped, which is not the backlog landing here. */}
          <p className="text-sm leading-[22px] text-foreground">
            Submissions go onto the shared calendar, so we confirm every
            submitter&rsquo;s email once. If your link is buried or expired,
            send yourself a fresh one.
          </p>

          <div className="flex w-full flex-col gap-2.5">
            <Button
              onClick={handleResend}
              disabled={sendVerificationEmail.isPending || cooldown.isCoolingDown}
              className="w-full"
            >
              {sendVerificationEmail.isPending ? (
                <Loader2 className="animate-spin" />
              ) : null}
              Send verification email
            </Button>
            <Button asChild variant="outline" className="w-full">
              <Link href="/profile?tab=settings">Manage email in Settings</Link>
            </Button>
          </div>

          {/* Mounted unconditionally: assistive tech announces changes WITHIN a
              live region already on the page, so a region inserted together
              with its text is announced unreliably. The visible line ticks once
              a second and is kept out of the region for the reason in
              resendStatusAnnouncement. */}
          <p className="sr-only" role="status">
            {announcement ?? ''}
          </p>

          {status && (
            <p
              aria-hidden="true"
              className="font-mono text-[11px] uppercase tracking-[0.44px] text-primary"
            >
              {status}
            </p>
          )}

          {sessionExpired && (
            <p role="alert" className="text-sm text-destructive">
              Your session has expired.{' '}
              <Link href={SIGN_IN_HREF} className="underline">
                Sign in again
              </Link>{' '}
              to send the email.
            </p>
          )}

          {sendFailed && (
            <p role="alert" className="text-sm text-destructive">
              We could not send that email just now. Please try again in a
              moment.
            </p>
          )}

          <p className="text-xs text-muted-foreground">
            Verify, then come back here to post your show.
          </p>
        </div>
      </div>
    </div>
  )
}

/**
 * /shows/submit — show-submission form.
 *
 * Moved here from /submissions in PSY-600 to free up that path for the
 * contributor pending-edits surface. Behaviour preserved:
 *   - settled anonymous → redirect to login (returnTo back here)
 *   - authenticated but unverified email → "Verify email" gate
 *   - authenticated + verified (or admin) → form
 */
export default function SubmitShowPage() {
  const { user } = useAuthContext()
  const gate = useAuthRouteGuard()

  const [extractedData, setExtractedData] = useState<
    ExtractedShowData | undefined
  >()
  // Bumped on each extraction so <ShowForm key={...}> remounts and re-seeds
  // its defaultValues from the new extraction (PSY-795 — replaces the prior
  // prop-derived useEffect inside ShowForm).
  const [extractionVersion, setExtractionVersion] = useState(0)

  const handleExtracted = (data: ExtractedShowData) => {
    setExtractedData(data)
    setExtractionVersion(v => v + 1)
  }

  if (gate === 'loading') {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (gate === 'blank') {
    return null
  }

  // Layered on a settled identity: the verification gate asks what this viewer
  // may do, which the guard above has to answer "who" for first.
  const canSubmit = user?.is_admin || user?.email_verified

  if (!canSubmit) {
    return <EmailVerificationRequired />
  }

  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-lg">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Music className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">Submit a Show</h1>
          {user && (
            <p className="mt-1 text-xs text-muted-foreground">
              Submitting as {user.email}
            </p>
          )}
        </div>

        <AIFormFiller onExtracted={handleExtracted} />

        <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
          <CardContent className="pt-4">
            <ShowForm
              key={extractionVersion}
              mode="create"
              initialExtraction={extractedData}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
