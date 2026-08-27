'use client'

import { createContext, useContext, useState, type ReactNode } from 'react'
import Link from 'next/link'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import type { VariantProps } from 'class-variance-authority'
import { useSendVerificationEmail } from '@/features/auth'
// Imported by module path, not through the `@/features/auth` barrel: the barrel
// is mocked wholesale in several suites, and the countdown is worth exercising
// for real wherever this control is tested.
import {
  VERIFICATION_RESEND_COOLDOWN_SECONDS,
  formatResendStatus,
  isVerificationResendUnauthorized,
  resendStatusAnnouncement,
  useVerificationResendCooldown,
  verificationResendThrottle,
  type ResendStatusDensity,
  type ResendWaitPrecision,
} from '../hooks/useVerificationResendCooldown'
import { Button, buttonVariants } from '@/components/ui/button'

/**
 * The one verification-resend control (PSY-1911).
 *
 * Four surfaces ask a signed-in reader to send themselves a verification email:
 * the post-signup interstitial, the /verify-email dead-link card, the
 * /shows/submit gate, and the Settings account row. They were built in parallel
 * and drifted — four handlers, two different 429 voices. What actually differs
 * between them is layout and label; the behaviour behind the button (send,
 * park, tell the reader what happened) is identical, so it lives here once.
 *
 * Composed rather than configured: `<VerificationResend>` renders no DOM of its
 * own and only publishes state, so each surface keeps its own markup and drops
 * the parts where its layout wants them. A single component with `layout` and
 * `showStatusBeside` style props would have had to grow a knob per surface.
 *
 * ONE 429 VOICE. A throttle is a wait, not a failure: the control parks and the
 * status line says so. It never turns red, never quotes the backend's own
 * wording, and never claims a second count the app cannot know (see
 * `ResendWaitPrecision`).
 */

interface VerificationResendState {
  /** A send from this surface has been confirmed. */
  sent: boolean
  /** A send failed for a reason the reader can do nothing about. */
  failed: boolean
  /** The session died between opening the surface and clicking. */
  sessionExpired: boolean
  isPending: boolean
  isCoolingDown: boolean
  secondsRemaining: number
  precision: ResendWaitPrecision
  signInHref: string
  resend: () => Promise<void>
}

const VerificationResendContext =
  createContext<VerificationResendState | null>(null)

function useVerificationResendPart(part: string): VerificationResendState {
  const state = useContext(VerificationResendContext)
  if (!state) {
    throw new Error(`<${part}> must be rendered inside <VerificationResend>`)
  }
  return state
}

interface VerificationResendProps {
  /**
   * Sentry `service` tag naming the surface, so a genuine send failure is
   * attributable to the screen it happened on.
   */
  service: string
  /** Where a reader whose session died is sent to get a new one. */
  signInHref: string
  children: ReactNode
}

export function VerificationResend({
  service,
  signInHref,
  children,
}: VerificationResendProps) {
  const sendVerificationEmail = useSendVerificationEmail()
  const cooldown = useVerificationResendCooldown()
  const [sent, setSent] = useState(false)
  const [failed, setFailed] = useState(false)
  const [sessionExpired, setSessionExpired] = useState(false)

  const isPending = sendVerificationEmail.isPending
  const isCoolingDown = cooldown.isCoolingDown

  const resend = async () => {
    if (isPending || isCoolingDown) {
      return
    }
    setFailed(false)
    try {
      await sendVerificationEmail.mutateAsync()
      setSent(true)
      cooldown.start(VERIFICATION_RESEND_COOLDOWN_SECONDS)
    } catch (error) {
      const throttle = verificationResendThrottle(error)
      if (throttle) {
        // Throttled, not broken: park the control rather than raise an alert.
        cooldown.start(throttle.seconds, throttle.precision)
        return
      }
      if (isVerificationResendUnauthorized(error)) {
        // The cookie died while the surface sat open. Give a way forward, and
        // do not page on-call for an ordinary session expiry.
        setSessionExpired(true)
        return
      }
      setFailed(true)
      Sentry.captureException(error, {
        level: 'error',
        tags: { service, error_type: 'verification_email' },
      })
    }
  }

  return (
    <VerificationResendContext.Provider
      value={{
        sent,
        failed,
        sessionExpired,
        isPending,
        isCoolingDown,
        secondsRemaining: cooldown.secondsRemaining,
        precision: cooldown.precision,
        signInHref,
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
  className?: string
  variant?: VariantProps<typeof buttonVariants>['variant']
  size?: VariantProps<typeof buttonVariants>['size']
}

/**
 * The button. Stays mounted and labelled while it waits, so the state reads as
 * a pause rather than a control that vanished.
 */
export function VerificationResendButton({
  children,
  className,
  variant,
  size,
}: VerificationResendButtonProps) {
  const { isPending, isCoolingDown, resend } = useVerificationResendPart(
    'VerificationResendButton'
  )

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
      {children}
    </Button>
  )
}

interface VerificationResendStatusProps {
  /** Per-surface styling for the visible line; the live region is always sr-only. */
  className?: string
  density?: ResendStatusDensity
}

/**
 * The confirmation-and-wait line, plus the live region that carries it to
 * assistive tech.
 *
 * The live region is mounted unconditionally: assistive tech announces changes
 * WITHIN a region already on the page, so a region inserted together with its
 * text is announced unreliably. The visible line ticks once a second and is
 * hidden from the region for the reason in `resendStatusAnnouncement`.
 */
export function VerificationResendStatus({
  className,
  density,
}: VerificationResendStatusProps) {
  const { sent, isCoolingDown, secondsRemaining, precision } =
    useVerificationResendPart('VerificationResendStatus')

  const status = formatResendStatus({
    sent,
    secondsRemaining,
    precision,
    density,
  })
  const announcement = resendStatusAnnouncement(sent, isCoolingDown)

  return (
    <>
      <p className="sr-only" role="status">
        {announcement ?? ''}
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
 * The two things that are genuinely wrong rather than merely slow.
 *
 * A dead session gets the sign-in link inline, because "your session expired"
 * with no way back is a dead end; a real send failure gets copy that says only
 * what the reader can act on, never the backend's own message.
 */
export function VerificationResendAlerts() {
  const { failed, sessionExpired, signInHref } = useVerificationResendPart(
    'VerificationResendAlerts'
  )

  return (
    <>
      {sessionExpired && (
        <p role="alert" className="text-sm text-destructive">
          Your session has expired.{' '}
          <Link href={signInHref} className="underline">
            Sign in again
          </Link>{' '}
          to send the email.
        </p>
      )}

      {failed && (
        <p role="alert" className="text-sm text-destructive">
          We could not send that email just now. Please try again in a moment.
        </p>
      )}
    </>
  )
}
