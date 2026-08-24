'use client'

import { Suspense, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { useConfirmVerification, useSendVerificationEmail } from '@/features/auth'
// Imported by module path, not through the `@/features/auth` barrel: the barrel
// is mocked wholesale in several suites, and the countdown is worth exercising
// for real wherever these surfaces are tested.
import {
  VERIFICATION_RESEND_COOLDOWN_SECONDS,
  formatResendStatus,
  isVerificationResendUnauthorized,
  resendStatusAnnouncement,
  useVerificationResendCooldown,
  verificationResendRetryAfter,
} from '@/features/auth/hooks/useVerificationResendCooldown'
import { useAuthContext } from '@/lib/context/AuthContext'
import { buildAuthHref } from '@/lib/auth-href'
import type { ApiError } from '@/lib/api'
import { Button } from '@/components/ui/button'

const KICKER = 'font-mono text-[11px] uppercase tracking-[0.66px]'

/** Where a reader with no usable session is sent to get one. */
const SIGN_IN_HREF = buildAuthHref('/profile?tab=settings')

/**
 * True when the backend judged the token itself bad, rather than failing to
 * judge it at all.
 *
 * `useConfirmVerification` raises an `AuthError` carrying status 400 for every
 * verdict the backend returns in a 200 body (invalid token, email mismatch,
 * unknown user). Anything else out of `apiRequest` is a network drop, a 5xx or
 * a rate limit, where the link may be perfectly good. Telling that reader
 * their link expired sends them to fetch a replacement that will fail the same
 * way.
 */
function isTokenRejected(error: unknown): boolean {
  return Boolean(error) && (error as ApiError).status === 400
}

/**
 * The landing card every /verify-email state renders into.
 *
 * Sharp-cornered bordered panel rather than <Card>: the approved mocks put the
 * state's own hairline colour on the panel edge (muted on success, destructive
 * on a dead link), which is the one thing that distinguishes the states at a
 * glance.
 */
function LandingCard({
  tone = 'default',
  children,
}: {
  tone?: 'default' | 'destructive'
  children: React.ReactNode
}) {
  return (
    <div className="min-h-[calc(100vh-64px)] px-4 py-8">
      <div className="mx-auto max-w-xl">
        <div
          className={`flex flex-col items-start gap-4 border bg-card px-6 py-8 sm:px-12 sm:py-11 ${
            tone === 'destructive' ? 'border-destructive' : 'border-border'
          }`}
        >
          {children}
        </div>
      </div>
    </div>
  )
}

/** One row of the post-verification radar: what the account can do now. */
function RadarRow({
  label,
  detail,
  highlighted = false,
}: {
  label: string
  detail: string
  highlighted?: boolean
}) {
  return (
    <div
      className={`grid w-full grid-cols-[5rem_1fr] items-center gap-4 px-4 py-2.5 sm:grid-cols-[190px_1fr] ${
        highlighted ? 'bg-background' : ''
      }`}
    >
      <p
        className={`font-mono text-xs ${
          highlighted ? 'font-bold text-primary' : 'text-muted-foreground'
        }`}
      >
        {label}
      </p>
      <p
        className={`min-w-0 text-[13px] ${
          highlighted ? 'text-foreground' : 'text-muted-foreground'
        }`}
      >
        {detail}
      </p>
    </div>
  )
}

/**
 * Post-verification landing.
 *
 * The copy deliberately anchors on submissions rather than on alerts. The mock
 * announces "EMAIL ALERTS AVAILABLE" and "in-app on now", but no send path
 * consults `email_verified` (`sendFilterEmail` pulls the address with a bare
 * `Pluck("email")`), and a plain follow drives no delivery at all: PSY-1893
 * stored the subscription and `EffectiveShowScope` still has no non-test
 * caller, with the matcher parked in PSY-1896. Submitting is the one thing
 * verification genuinely opens, and it is enforced server-side
 * (`catalog/show.go` blocks unverified non-admins with a 403), so that is what
 * the page claims. The ALERTS rung stays highlighted as the next step to take,
 * not as a switch that just flipped.
 */
function VerifiedLanding() {
  return (
    <LandingCard>
      <p className={`${KICKER} text-success-foreground`}>
        Email confirmed · Submissions open
      </p>
      <h1 className="font-display text-[28px] font-bold text-foreground">
        Welcome to the index.
      </h1>
      <p className="text-sm leading-[22px] text-foreground">
        Your email is verified, and show submissions are open to you. From here:
      </p>

      <div className="flex w-full flex-col border border-border">
        <RadarRow label="SAVE" detail="shows you plan to catch, kept in one place" />
        <RadarRow label="FOLLOW" detail="artists and venues you want to keep track of" />
        <RadarRow
          label="ALERTS"
          detail="choose what gets emailed to you in Settings"
          highlighted
        />
        <RadarRow label="SUBMIT" detail="spotted a missing show? add it any time" />
      </div>

      <div className="flex flex-wrap gap-3">
        <Button asChild>
          <Link href="/shows">Browse upcoming shows</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/artists">Explore artists</Link>
        </Button>
      </div>
    </LandingCard>
  )
}

/**
 * Dead-link card, shared by the expired/used token and the no-token cases.
 *
 * Only the headline and the kicker's second half differ: telling someone their
 * link "expired" when they landed here with no token at all would be a guess
 * dressed as a diagnosis.
 */
function DeadLinkLanding({ reason }: { reason: 'expired' | 'invalid' }) {
  const { isAuthenticated } = useAuthContext()
  const sendVerificationEmail = useSendVerificationEmail()
  const cooldown = useVerificationResendCooldown()
  const [sent, setSent] = useState(false)
  const [failed, setFailed] = useState(false)
  const [sessionExpired, setSessionExpired] = useState(false)
  const status = formatResendStatus(sent, cooldown.secondsRemaining)
  const announcement = resendStatusAnnouncement(sent, cooldown.isCoolingDown)
  const canResend = isAuthenticated && !sessionExpired

  const handleSendFreshLink = async () => {
    if (sendVerificationEmail.isPending || cooldown.isCoolingDown) {
      return
    }
    setFailed(false)
    try {
      await sendVerificationEmail.mutateAsync()
      setSent(true)
      cooldown.start(VERIFICATION_RESEND_COOLDOWN_SECONDS)
    } catch (error) {
      const retryAfter = verificationResendRetryAfter(error)
      if (retryAfter !== null) {
        // Throttled, not broken. Park the control instead of alarming anyone.
        cooldown.start(retryAfter)
        return
      }
      if (isVerificationResendUnauthorized(error)) {
        // The cookie died while this card sat open. Swap in the sign-in route
        // this component already knows how to render, and do not page on-call
        // for an expected session expiry.
        setSessionExpired(true)
        return
      }
      setFailed(true)
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'verify_email', error_type: 'verification_email' },
      })
    }
  }

  return (
    <LandingCard tone="destructive">
      <p className={`${KICKER} text-destructive`}>
        Contributor record · {reason === 'expired' ? 'Link expired' : 'Link not valid'}
      </p>
      <h1 className="font-display text-[26px] font-bold text-foreground">
        {reason === 'expired'
          ? 'That link has expired.'
          : 'That link is not valid.'}
      </h1>
      {/* Not "each one replaces the last": verification tokens are stateless
          24-hour JWTs with no revocation (CreateVerificationToken in
          services/auth/jwt.go), so every unexpired link still works. */}
      <p className="text-sm leading-[22px] text-foreground">
        Verification links last 24 hours. Send yourself a fresh one and use the
        newest email in your inbox.
      </p>

      {canResend ? (
        <Button
          onClick={handleSendFreshLink}
          disabled={sendVerificationEmail.isPending || cooldown.isCoolingDown}
        >
          {sendVerificationEmail.isPending ? (
            <Loader2 className="animate-spin" />
          ) : null}
          Send a fresh link
        </Button>
      ) : (
        // A dead link is often opened in a browser with no session, where the
        // resend endpoint would only 401. Route through sign-in instead of
        // offering a button that cannot work.
        <Button asChild>
          <Link href={SIGN_IN_HREF}>Sign in to send a fresh link</Link>
        </Button>
      )}

      {/* Mounted unconditionally: assistive tech announces changes WITHIN a
          live region that is already on the page, so a region inserted together
          with its text is announced unreliably. The visible line ticks once a
          second and is hidden from the region for the reason in
          resendStatusAnnouncement. */}
      <p className="sr-only" role="status">
        {announcement ?? ''}
      </p>

      {status && (
        <p className={`${KICKER} text-primary`} aria-hidden="true">
          {status}
        </p>
      )}

      {failed && (
        <p className="text-sm text-destructive" role="alert">
          We could not send that email just now. Please try again in a moment.
        </p>
      )}

      {sessionExpired && (
        <p className="text-sm text-destructive" role="alert">
          Your session has expired. Sign in again to send a fresh link.
        </p>
      )}
    </LandingCard>
  )
}

function CheckingLanding() {
  return (
    <LandingCard>
      <p className={`${KICKER} text-muted-foreground`}>
        Contributor record · Checking your link
      </p>
      <h1 className="flex items-center gap-3 font-display text-[26px] font-bold text-foreground">
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
        One moment.
      </h1>
    </LandingCard>
  )
}

/**
 * The check did not complete: a dropped connection, a 5xx, a rate limit.
 *
 * Kept apart from the dead-link card because the link may be fine. Sending
 * this reader off for a replacement would waste the one they already hold and
 * fail again for the same reason, so the only action offered is to retry the
 * same token.
 */
function CheckFailedLanding({ onRetry }: { onRetry: () => void }) {
  return (
    <LandingCard tone="destructive">
      <p className={`${KICKER} text-destructive`}>
        Contributor record · Check failed
      </p>
      <h1 className="font-display text-[26px] font-bold text-foreground">
        We could not check that link.
      </h1>
      <p className="text-sm leading-[22px] text-foreground">
        Something went wrong on our end, and your link may still be good. Try
        again in a moment before sending yourself a new one.
      </p>
      <Button onClick={onRetry}>Try again</Button>
    </LandingCard>
  )
}

function VerifyEmailContent() {
  const searchParams = useSearchParams()
  const token = searchParams.get('token')
  const confirmVerification = useConfirmVerification()
  const attemptedTokenRef = useRef<string | null>(null)

  // Automatically verify once per token value.
  useEffect(() => {
    if (!token || attemptedTokenRef.current === token) {
      return
    }
    attemptedTokenRef.current = token
    confirmVerification.mutate(token)
  }, [token, confirmVerification])

  if (!token) {
    return <DeadLinkLanding reason="invalid" />
  }

  if (confirmVerification.isError) {
    return isTokenRejected(confirmVerification.error) ? (
      <DeadLinkLanding reason="expired" />
    ) : (
      <CheckFailedLanding onRetry={() => confirmVerification.mutate(token)} />
    )
  }

  if (confirmVerification.isSuccess) {
    return <VerifiedLanding />
  }

  // Pending, and the tick before the mutation is dispatched.
  return <CheckingLanding />
}

export default function VerifyEmailPage() {
  return (
    <Suspense fallback={<CheckingLanding />}>
      <VerifyEmailContent />
    </Suspense>
  )
}
