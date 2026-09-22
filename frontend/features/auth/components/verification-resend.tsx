'use client'

import { createContext, useContext, useState, type ReactNode } from 'react'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import type { VariantProps } from 'class-variance-authority'
// Through the barrel on purpose: suites that render a resend surface mock
// `useSendVerificationEmail` on `@/features/auth`, and that mock has to reach
// the control the surface delegates to.
import { useSendVerificationEmail } from '@/features/auth'
// By module path, not through the barrel, so a suite that mocks the barrel
// still runs the real countdown.
import {
  VERIFICATION_RESEND_COOLDOWN_SECONDS,
  formatResendStatus,
  isVerificationResendUnauthorized,
  resendStatusAnnouncement,
  useVerificationResendCooldown,
  verificationResendRetryAfter,
  type ResendStatusFormat,
} from '../hooks/useVerificationResendCooldown'
import { Button, buttonVariants } from '@/components/ui/button'

/**
 * The one verification-resend control.
 *
 * `<VerificationResend>` owns the behaviour every resend surface shares: send,
 * park the control for the cooldown, treat a 429 as a wait rather than an
 * error, route a dead session to its own alert without paging Sentry, and
 * report only a genuine failure. It renders no DOM. Each surface composes the
 * parts below into its own layout and supplies its own words, because layout
 * and wording are what legitimately differ between surfaces; behaviour is not.
 */

export interface VerificationResendState {
  /** A send from this surface has been confirmed. */
  sent: boolean
  /** The last attempt failed for a reason the reader can do nothing about. */
  failed: boolean
  /** A send was refused because the session is gone. Sticky until remount. */
  sessionExpired: boolean
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
  children: ReactNode
}

export function VerificationResend({ service, children }: VerificationResendProps) {
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
      const retryAfter = verificationResendRetryAfter(error)
      if (retryAfter !== null) {
        cooldown.start(retryAfter)
        return
      }
      if (isVerificationResendUnauthorized(error)) {
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
  pendingLabel?: ReactNode
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
      {isPending && pendingLabel !== undefined ? pendingLabel : children}
    </Button>
  )
}

interface VerificationResendStatusProps {
  /** Styling for the visible line; the live region is always sr-only. */
  className?: string
  /** Wording for the visible line. Defaults to the landing-surface line. */
  format?: ResendStatusFormat
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
}: VerificationResendStatusProps) {
  const { sent, isCoolingDown, secondsRemaining } = useVerificationResendState()
  const status = format(sent, secondsRemaining)

  return (
    <>
      <p className="sr-only" role="status">
        {resendStatusAnnouncement(sent, isCoolingDown) ?? ''}
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

/**
 * The alert for a send that genuinely failed. Never quotes the backend's own
 * message: the only thing the reader can act on is "try again".
 */
export function VerificationResendFailed() {
  const { failed } = useVerificationResendState()
  if (!failed) {
    return null
  }
  return (
    <p role="alert" className="text-sm text-destructive">
      We could not send that email just now. Please try again in a moment.
    </p>
  )
}
