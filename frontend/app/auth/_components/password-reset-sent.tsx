'use client'

import { useEffect, useRef } from 'react'
import { useRequestPasswordReset } from '@/features/auth'
import {
  EmailResend,
  SENT_AGAIN,
  SentConfirmation,
  VerificationResendButton,
  VerificationResendFailed,
  VerificationResendStatus,
  announceSent,
} from '@/features/auth/components/verification-resend'
import { formatResendWaitOnly } from '@/features/auth/hooks/useVerificationResendCooldown'
import { PASSWORD_RESET_EXPIRY_MINUTES } from '@/lib/password-reset'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'

interface PasswordResetSentProps {
  /** The address the reset was asked for, as typed. */
  email: string
  /** Returns to the Sign in tab. */
  onBack: () => void
}

/**
 * The Sign in tab's "Forgot password?" sent state, in place on /auth so the
 * address never travels through a URL. The request answers the same whether
 * or not an account uses the address, so the sentence naming the address
 * says a link was sent IF an account uses it.
 *
 * Resend asks again for the same address through the shared resend control.
 * The request route needs no session, so the control's expired-session part
 * has nothing to report here and is left out.
 */
export function PasswordResetSent({ email, onBack }: PasswordResetSentProps) {
  const headingRef = useRef<HTMLHeadingElement>(null)
  const requestReset = useRequestPasswordReset()

  // The card replaces the form the reader submitted from, taking its focused
  // control with it, so focus moves to the heading.
  useEffect(() => {
    headingRef.current?.focus()
  }, [])

  return (
    <Card className="gap-4 px-10 py-9">
      <p className="text-xs text-muted-foreground">
        One email sent · password reset
      </p>

      <h1
        ref={headingRef}
        tabIndex={-1}
        className="font-display text-[26px] font-bold leading-tight text-foreground focus:outline-none"
      >
        Check your inbox.
      </h1>

      <p className="text-sm leading-[22px] text-foreground">
        If an account uses <span className="font-data break-words">{email}</span>, we sent it a
        link to choose a new password. It expires in {PASSWORD_RESET_EXPIRY_MINUTES} minutes.
      </p>

      <div className="space-y-1.5 border border-border bg-background px-4 py-3">
        <p className="text-xs text-muted-foreground">
          Nothing changes until you open it
        </p>
        <p className="text-[13px] leading-5 text-foreground">
          Your current password, your signed-in devices and your passkeys stay as they are. The
          link lets you choose a new password, and setting it signs out every other device.
        </p>
      </div>

      <EmailResend
        service="auth_password_reset_sent"
        errorType="password_reset_email"
        send={() => requestReset.mutateAsync(email)}
        isPending={requestReset.isPending}
      >
        <div className="flex flex-wrap items-center gap-3">
          <Button type="button" onClick={onBack}>
            Back to sign in
          </Button>
          <VerificationResendButton variant="outline" pendingLabel="Sending...">
            Resend email
          </VerificationResendButton>
        </div>

        <SentConfirmation className="text-sm text-success-foreground">{SENT_AGAIN}</SentConfirmation>
        <VerificationResendStatus
          format={formatResendWaitOnly}
          announce={announceSent(SENT_AGAIN)}
          className="text-xs text-link"
        />
        <VerificationResendFailed />
      </EmailResend>
    </Card>
  )
}
