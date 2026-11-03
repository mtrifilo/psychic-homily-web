'use client'

import { createContext, useContext, useState, type ReactNode } from 'react'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import type { VariantProps } from 'class-variance-authority'
// Through the barrel on purpose: suites that render a resend surface mock
// `useSendVerificationEmail` on `@/features/auth`, and that mock has to reach
// the control the surface delegates to.
import { useSendVerificationEmail } from '@/features/auth'
import {
  VERIFICATION_RESEND_COOLDOWN_SECONDS,
  formatResendStatus,
  isVerificationResendAlreadyVerified,
  isVerificationResendUnauthorized,
  resendStatusAnnouncement,
  useVerificationResendCooldown,
  verificationResendRetryAfter,
} from '../hooks/useVerificationResendCooldown'
import { Button, buttonVariants } from '@/components/ui/button'

/**
 * The one verification-resend control, and through `EmailResend` the one
 * resend control for any other email a reader can ask for again.
 *
 * `<VerificationResend>` owns the behaviour every resend surface shares: send,
 * park the control for the cooldown, treat a 429 as a wait rather than an
 * error, route a dead session to its own alert without paging Sentry, and
 * report only a genuine failure. It renders no DOM. Each surface composes the
 * parts below into its own layout and supplies its own words, because layout
 * and wording are what legitimately differ between surfaces; behaviour is not.
 *
 * Every surface must render all four parts. The provider only records an
 * outcome; the parts are what show it. Leave one out and that outcome goes
 * silent: without `VerificationResendStatus` a throttle parks the button with
 * no reason given and nothing is announced, without
 * `VerificationResendSessionExpired` a dead session fails every click unseen
 * (and, by design, unreported), and without `VerificationResendFailed` a real
 * failure reaches Sentry but never the reader.
 */

export interface VerificationResendState {
  /**
   * A send from this surface has been confirmed at some point. Sticky, so the
   * shared status line and announcement stay put once the wait runs out.
   */
  sent: boolean
  /**
   * The most recent settled attempt was a confirmed send: false while a send is
   * in flight, and after any later throttle, failure, or expired session. For a
   * surface whose confirmation must not outlive the attempt that earned it.
   */
  latestAttemptSent: boolean
  /** The last attempt failed for a reason other than a throttle or an expired session. */
  failed: boolean
  /**
   * Narrows `failed`: the backend refused because the address is already
   * verified. Only ever true while `failed` is.
   */
  alreadyVerified: boolean
  /** A send was refused because the session is gone. Sticky until remount. */
  sessionExpired: boolean
  /**
   * The most recent settled attempt was refused because the session is gone.
   * Held while a retry is in flight, and cleared by any other settled outcome:
   * a send, a throttle, or a refusal all mean the server accepted the session,
   * since the resend route authenticates before it throttles or answers.
   */
  latestSettledSessionExpired: boolean
  /**
   * Counts expired-session refusals, so a surface can remount its alert on a
   * repeated one and have assistive tech announce it again.
   */
  sessionExpiredRefusals: number
  isPending: boolean
  isCoolingDown: boolean
  /** Whole seconds left on the cooldown; 0 when the control is usable. */
  secondsRemaining: number
  /** A no-op while a send is in flight or the control is parked. */
  resend: () => Promise<void>
}

const VerificationResendContext =
  createContext<VerificationResendState | null>(null)

/** The shared state, for a surface whose own markup branches on it. */
export function useVerificationResendState(): VerificationResendState {
  const state = useContext(VerificationResendContext)
  if (!state) {
    throw new Error(
      'useVerificationResendState must be used inside <VerificationResend>'
    )
  }
  return state
}

interface VerificationResendProps {
  /**
   * Sentry `service` tag naming the surface, so a genuine send failure is
   * attributable to the screen it happened on.
   */
  service: string
  /**
   * Whether an already-verified refusal is reported to Sentry like any other
   * failure. Defaults to true; a surface that names the state to the reader
   * (`VerificationResendFailed`'s `alreadyVerifiedMessage`) treats it as an
   * outcome rather than a fault and passes false.
   */
  reportAlreadyVerified?: boolean
  children: ReactNode
}

/** The verification email's resend: `EmailResend` over the verification send. */
export function VerificationResend({
  service,
  reportAlreadyVerified = true,
  children,
}: VerificationResendProps) {
  const sendVerificationEmail = useSendVerificationEmail()
  return (
    <EmailResend
      service={service}
      errorType="verification_email"
      send={sendVerificationEmail.mutateAsync}
      isPending={sendVerificationEmail.isPending}
      reportAlreadyVerified={reportAlreadyVerified}
    >
      {children}
    </EmailResend>
  )
}

interface EmailResendProps {
  /** Sentry `service` tag naming the surface. */
  service: string
  /** Sentry `error_type` tag naming the email. */
  errorType: string
  /**
   * Sends the email. Resolves once the server accepted the send; rejects with
   * the request's error otherwise, which this control sorts into a throttle,
   * a dead session, or a failure.
   */
  send: () => Promise<unknown>
  /** A send is in flight. */
  isPending: boolean
  /** As `VerificationResend`'s; only a verification send can be refused so. */
  reportAlreadyVerified?: boolean
  children: ReactNode
}

/**
 * The provider behind every resend control, for any email the reader can ask
 * for again. The parts below (`VerificationResendButton`, `...Status`,
 * `...SessionExpired`, `...Failed`) read whichever provider wraps them, so a
 * surface that resends some other email keeps the shared look, cooldown and
 * outcome handling by wrapping them in this with its own `send`.
 */
export function EmailResend({
  service,
  errorType,
  send,
  isPending,
  reportAlreadyVerified = true,
  children,
}: EmailResendProps) {
  const cooldown = useVerificationResendCooldown()
  const [sent, setSent] = useState(false)
  const [latestAttemptSent, setLatestAttemptSent] = useState(false)
  const [failed, setFailed] = useState(false)
  const [alreadyVerified, setAlreadyVerified] = useState(false)
  const [sessionExpired, setSessionExpired] = useState(false)
  const [latestSettledSessionExpired, setLatestSettledSessionExpired] =
    useState(false)
  const [sessionExpiredRefusals, setSessionExpiredRefusals] = useState(0)

  const isCoolingDown = cooldown.isCoolingDown

  const resend = async () => {
    if (isPending || isCoolingDown) {
      return
    }
    setLatestAttemptSent(false)
    setFailed(false)
    setAlreadyVerified(false)
    try {
      await send()
      setLatestSettledSessionExpired(false)
      setSent(true)
      setLatestAttemptSent(true)
      cooldown.start(VERIFICATION_RESEND_COOLDOWN_SECONDS)
    } catch (error) {
      const sessionGone = isVerificationResendUnauthorized(error)
      setLatestSettledSessionExpired(sessionGone)
      const retryAfter = verificationResendRetryAfter(error)
      if (retryAfter !== null) {
        cooldown.start(retryAfter)
        return
      }
      if (sessionGone) {
        setSessionExpired(true)
        setSessionExpiredRefusals(count => count + 1)
        return
      }
      setFailed(true)
      const alreadyVerifiedRefusal = isVerificationResendAlreadyVerified(error)
      setAlreadyVerified(alreadyVerifiedRefusal)
      if (alreadyVerifiedRefusal && !reportAlreadyVerified) {
        return
      }
      Sentry.captureException(error, {
        level: 'error',
        tags: { service, error_type: errorType },
      })
    }
  }

  return (
    <VerificationResendContext.Provider
      value={{
        sent,
        latestAttemptSent,
        failed,
        alreadyVerified,
        sessionExpired,
        latestSettledSessionExpired,
        sessionExpiredRefusals,
        isPending,
        isCoolingDown,
        secondsRemaining: cooldown.secondsRemaining,
        resend,
      }}
    >
      {children}
    </VerificationResendContext.Provider>
  )
}

interface VerificationResendButtonProps {
  /** The label at rest. Each surface names the action in its own terms. */
  children: ReactNode
  /** Replaces the label while a send is in flight; the spinner shows either way. */
  pendingLabel?: string
  className?: string
  variant?: VariantProps<typeof buttonVariants>['variant']
  size?: VariantProps<typeof buttonVariants>['size']
}

/**
 * Stays mounted and labelled while parked, so the wait reads as a pause rather
 * than a control that vanished.
 */
export function VerificationResendButton({
  children,
  pendingLabel,
  className,
  variant,
  size,
}: VerificationResendButtonProps) {
  const { isPending, isCoolingDown, resend } = useVerificationResendState()

  return (
    <Button
      type="button"
      onClick={resend}
      disabled={isPending || isCoolingDown}
      variant={variant}
      size={size}
      className={className}
    >
      {isPending ? <Loader2 className="animate-spin" /> : null}
      {isPending && pendingLabel ? pendingLabel : children}
    </Button>
  )
}

/**
 * Wording for the visible status line, from the two independent halves of the
 * control's state; `null` when there is nothing to say.
 */
export type ResendStatusFormat = (
  sent: boolean,
  secondsRemaining: number
) => string | null

/**
 * What the live region says; `null` when there is nothing to say. It is not
 * handed the seconds left, so it cannot tick: see `resendStatusAnnouncement`.
 */
export type ResendAnnouncement = (
  state: Pick<
    VerificationResendState,
    'sent' | 'latestAttemptSent' | 'isCoolingDown'
  >
) => string | null

const sharedAnnouncement: ResendAnnouncement = ({ sent, isCoolingDown }) =>
  resendStatusAnnouncement(sent, isCoolingDown)

interface VerificationResendStatusProps {
  /** Styling for the visible line; the live region is always sr-only. */
  className?: string
  /** Wording for the visible line. Defaults to the landing-surface line. */
  format?: ResendStatusFormat
  /** Wording for the live region. Defaults to the shared announcement. */
  announce?: ResendAnnouncement
}

/**
 * The live region that announces a send or a wait, plus the visible line.
 *
 * The live region is mounted unconditionally: assistive tech announces changes
 * WITHIN a region already on the page, so a region inserted together with its
 * text is announced unreliably. The visible line ticks once a second and is
 * hidden from assistive tech for the reason in `resendStatusAnnouncement`.
 */
export function VerificationResendStatus({
  className,
  format = formatResendStatus,
  announce = sharedAnnouncement,
}: VerificationResendStatusProps) {
  const { sent, latestAttemptSent, isCoolingDown, secondsRemaining } =
    useVerificationResendState()
  const status = format(sent, secondsRemaining)

  return (
    <>
      <p className="sr-only" role="status">
        {announce({ sent, latestAttemptSent, isCoolingDown }) ?? ''}
      </p>
      {status && (
        <p aria-hidden="true" className={className}>
          {status}
        </p>
      )}
    </>
  )
}

/**
 * The alert for a session that died while the surface sat open. The words are
 * the surface's, since each one offers a different way back to sign-in.
 */
export function VerificationResendSessionExpired({
  children,
}: {
  children: ReactNode
}) {
  const { sessionExpired } = useVerificationResendState()
  if (!sessionExpired) {
    return null
  }
  return (
    <p role="alert" className="text-sm text-destructive">
      {children}
    </p>
  )
}

/** The words for a send that failed, on every resend surface. */
export const EMAIL_SEND_FAILED_MESSAGE =
  'We could not send that email just now. Please try again in a moment.'

/**
 * The alert for a send that failed. Never renders the backend's message; a
 * surface may supply its own words for an already-verified refusal, which
 * otherwise gets the generic line too.
 */
export function VerificationResendFailed({
  alreadyVerifiedMessage,
}: {
  alreadyVerifiedMessage?: string
}) {
  const { failed, alreadyVerified } = useVerificationResendState()
  if (!failed) {
    return null
  }
  return (
    <p role="alert" className="text-sm text-destructive">
      {alreadyVerified && alreadyVerifiedMessage
        ? alreadyVerifiedMessage
        : EMAIL_SEND_FAILED_MESSAGE}
    </p>
  )
}

/**
 * For a surface that confirms a send in its own sentence: the live region
 * speaks `message` while the latest attempt is a confirmed send, and only the
 * wait otherwise. The region empties when the next attempt starts, so every
 * success is announced. Pair it with `SentConfirmation` showing the same
 * message, and with `formatResendWaitOnly` for the visible line.
 */
export function announceSent(message: string): ResendAnnouncement {
  return ({ latestAttemptSent, isCoolingDown }) =>
    latestAttemptSent ? message : resendStatusAnnouncement(false, isCoolingDown)
}

/**
 * A surface's confirmation of a send, shown only while that send is the latest
 * outcome: a later attempt in flight, throttled, or refused takes it down, so
 * it never claims a send that did not happen. Hidden from assistive tech,
 * because the live region speaks the same sentence (`announceSent`).
 */
export function SentConfirmation({
  className,
  children,
}: {
  className?: string
  children: ReactNode
}) {
  const { latestAttemptSent } = useVerificationResendState()
  if (!latestAttemptSent) {
    return null
  }
  return (
    <p aria-hidden="true" className={className}>
      {children}
    </p>
  )
}
