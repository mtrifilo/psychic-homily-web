'use client'

import {
  useVerificationResendState,
  type ResendAnnouncement,
} from '@/features/auth/components/verification-resend'
import { resendStatusAnnouncement } from '@/features/auth/hooks/useVerificationResendCooldown'

/**
 * The check-your-inbox cards' resend wording. Each card confirms a resend in
 * its own sentence, so the shared status line carries only the wait
 * (formatResendWaitOnly).
 */
export const SENT_AGAIN = 'Sent again. Give it a minute to arrive.'

/**
 * The live region speaks a confirmed resend in the same words the card shows,
 * and only while that resend is the latest outcome. The region empties when
 * the next attempt starts, so every success is announced.
 */
export const announceSentAgain: ResendAnnouncement = ({ latestAttemptSent, isCoolingDown }) =>
  latestAttemptSent ? SENT_AGAIN : resendStatusAnnouncement(false, isCoolingDown)

/**
 * The card's confirmation of a resend, shown only while that resend is the
 * latest outcome: a later attempt in flight, throttled, or refused takes it
 * down, so it never claims a send that did not happen.
 *
 * Hidden from assistive tech because the live region speaks the same sentence
 * under the same condition (`announceSentAgain`).
 */
export function SentAgainConfirmation() {
  const { latestAttemptSent } = useVerificationResendState()
  if (!latestAttemptSent) {
    return null
  }
  return (
    <p aria-hidden="true" className="text-sm text-success-foreground">
      {SENT_AGAIN}
    </p>
  )
}
