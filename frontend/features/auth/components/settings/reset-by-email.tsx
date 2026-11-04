'use client'

import type { ReactNode } from 'react'
import { CheckCircle2 } from 'lucide-react'
import { useSendOwnPasswordReset } from '@/features/auth'
import {
  EmailResend,
  SentConfirmation,
  VerificationResendButton,
  VerificationResendFailed,
  VerificationResendSessionExpired,
  VerificationResendStatus,
  announceSent,
} from '@/features/auth/components/verification-resend'
import { formatResendWaitOnly } from '@/features/auth/hooks/useVerificationResendCooldown'
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
 * and the shared status, session and failure lines below it.
 * `ResetByEmailButton` reads this provider, so it must sit inside `children`.
 */
export function ResetByEmail({
  email,
  children,
}: {
  /** The account's own address, where the link goes. */
  email: string
  children: ReactNode
}) {
  const sendOwnReset = useSendOwnPasswordReset()
  const sentMessage = resetLinkSentMessage(email)

  return (
    <EmailResend
      service="settings_password_reset"
      errorType="password_reset_email"
      send={sendOwnReset.mutateAsync}
      isPending={sendOwnReset.isPending}
    >
      {/* State 6S, in the card's success treatment. */}
      <SentConfirmation className="mb-4 flex items-center gap-2 rounded-md bg-success p-3 text-sm text-success-foreground">
        <CheckCircle2 className="h-4 w-4 shrink-0" />
        <span>{sentMessage}</span>
      </SentConfirmation>
      {children}
      <VerificationResendStatus
        format={formatResendWaitOnly}
        announce={announceSent(sentMessage)}
        className="text-xs text-link"
      />
      <VerificationResendSessionExpired>
        Your session has expired. Sign in again to send the email.
      </VerificationResendSessionExpired>
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
      <span className="font-data break-words">{email}</span>. Setting a new password with it signs out every
      other device.
    </p>
  )
}
