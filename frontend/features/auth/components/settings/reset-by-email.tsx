'use client'

import type { ReactNode } from 'react'
import { CheckCircle2 } from 'lucide-react'
import { useSendOwnPasswordReset } from '@/features/auth'
import {
  EmailResend,
  VerificationResendButton,
  VerificationResendFailed,
  VerificationResendSessionExpired,
  VerificationResendStatus,
  useVerificationResendState,
  type ResendAnnouncement,
} from '@/features/auth/components/verification-resend'
import {
  formatResendWaitOnly,
  resendStatusAnnouncement,
} from '@/features/auth/hooks/useVerificationResendCooldown'
import { PASSWORD_RESET_EXPIRY_MINUTES } from '@/lib/password-reset'

/** The confirmation of a sent reset link, shown and announced in these words. */
export function resetLinkSentMessage(email: string): string {
  return `Reset link sent to ${email}. It expires in ${PASSWORD_RESET_EXPIRY_MINUTES} minutes. Nothing changes until you open it.`
}

/**
 * The Change password card's emailed reset, for an owner who does not know
 * the current password. Sending goes to the account's own address and changes
 * nothing, so it asks for no recent sign-in. It runs on the shared resend
 * control, so a second send waits out the shared cooldown.
 *
 * `children` is the card's form; the confirmation renders above it (State 6S)
 * and the controls below it come from `ResetByEmailButton` and
 * `ResetByEmailNote`, which must sit inside.
 */
export function ResetByEmail({
  email,
  signInAgainHref,
  children,
}: {
  /** The account's own address, where the link goes. */
  email: string
  /** Where "Sign in again" leads when the session died while the card sat open. */
  signInAgainHref: string
  children: ReactNode
}) {
  const sendOwnReset = useSendOwnPasswordReset()
  const announce: ResendAnnouncement = ({ latestAttemptSent, isCoolingDown }) =>
    latestAttemptSent ? resetLinkSentMessage(email) : resendStatusAnnouncement(false, isCoolingDown)

  return (
    <EmailResend
      service="settings_password_reset"
      errorType="password_reset_email"
      send={sendOwnReset.mutateAsync}
      isPending={sendOwnReset.isPending}
    >
      <ResetLinkSent email={email} />
      {children}
      <VerificationResendStatus
        format={formatResendWaitOnly}
        announce={announce}
        className="font-data text-[11px] uppercase tracking-[0.66px] text-primary"
      />
      <ResetSessionExpired signInAgainHref={signInAgainHref} />
      <VerificationResendFailed />
    </EmailResend>
  )
}

/** The outline "Reset by email" button, beside the card's submit. */
export function ResetByEmailButton() {
  return (
    <VerificationResendButton variant="outline" pendingLabel="Sending...">
      Reset by email
    </VerificationResendButton>
  )
}

/** The line under the buttons saying what Reset by email does. */
export function ResetByEmailNote({ email }: { email: string }) {
  return (
    <p className="text-xs text-muted-foreground">
      Don&apos;t know the current password? Reset by email sends a link to{' '}
      <span className="break-words">{email}</span>. Setting a new password with it signs out every
      other device.
    </p>
  )
}

/**
 * State 6S, in the card's success treatment, while the latest send is a
 * confirmed one. Hidden from assistive tech: the status region speaks the
 * same sentence.
 */
function ResetLinkSent({ email }: { email: string }) {
  const { latestAttemptSent } = useVerificationResendState()
  if (!latestAttemptSent) return null
  return (
    <div
      aria-hidden="true"
      className="mb-4 flex items-center gap-2 rounded-md bg-success p-3 text-sm text-success-foreground"
    >
      <CheckCircle2 className="h-4 w-4 shrink-0" />
      <span>{resetLinkSentMessage(email)}</span>
    </div>
  )
}

function ResetSessionExpired({ signInAgainHref }: { signInAgainHref: string }) {
  return (
    <VerificationResendSessionExpired>
      Your session has expired.{' '}
      <a href={signInAgainHref} className="underline">
        Sign in again
      </a>{' '}
      to send the email.
    </VerificationResendSessionExpired>
  )
}
