'use client'

import { useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
  emailCalendarStepsFailure,
  useEmailCalendarSteps,
} from '@/features/auth/hooks/useCalendarFeed'
import {
  VerificationResend,
  VerificationResendButton,
  VerificationResendFailed,
  VerificationResendSessionExpired,
  VerificationResendStatus,
} from '@/features/auth/components/verification-resend'
import { useAuthContext } from '@/lib/context/AuthContext'
import { buildAuthHref } from '@/lib/auth-href'
import { cn } from '@/lib/utils'
import {
  PanelError,
  PanelNote,
  PanelRowButton,
  useCalendarPanelLayout,
} from './CalendarPanelParts'
import { PANEL_COPY, emailSentLine } from './savedShowsCalendarCopy'

/**
 * "Email me the steps": on a phone, where Google's app cannot add a calendar
 * from a link, the server emails the person a way to finish on a computer.
 *
 * Once a send succeeds the row confirms where it went and offers "Send again";
 * a later failure takes the confirmation down and names the failure, so the
 * panel never claims an email that a later attempt did not send. An address
 * that is not verified gets the house verification resend in place.
 */
export function CalendarEmailStepsAction() {
  const { user } = useAuthContext()
  const send = useEmailCalendarSteps()
  // Sticky across a "Send again" in flight, which would otherwise flash the
  // first-send row back for the length of the request.
  const [hasSent, setHasSent] = useState(false)

  const onSend = () => {
    send.mutate(undefined, { onSuccess: () => setHasSent(true) })
  }

  const failure = send.isError ? emailCalendarStepsFailure(send.error) : null
  const showSent = hasSent && !failure

  return (
    <div className="flex flex-col">
      {showSent ? (
        <>
          <PanelNote className="mt-0.5">
            {emailSentLine(user?.email ?? '')}
          </PanelNote>
          <PanelRowButton onClick={onSend} disabled={send.isPending}>
            {PANEL_COPY.sendAgain}
          </PanelRowButton>
        </>
      ) : (
        <PanelRowButton
          onClick={onSend}
          disabled={send.isPending}
          testId="calendar-email-steps"
        >
          {PANEL_COPY.emailSteps}
        </PanelRowButton>
      )}
      {failure === 'unverified' ? <VerifyEmailInPlace /> : null}
      {failure === 'rate_limited' ? (
        <PanelError>{PANEL_COPY.emailRateLimited}</PanelError>
      ) : null}
      {failure === 'failed' ? (
        <PanelError>{PANEL_COPY.emailFailed}</PanelError>
      ) : null}
    </div>
  )
}

/**
 * The unverified line and the house verification resend, worded as every
 * resend surface is: the status, failure and session lines are the shared
 * component's own, and the session line is the one the submission desk uses.
 */
function VerifyEmailInPlace() {
  const layout = useCalendarPanelLayout()
  const pathname = usePathname()
  return (
    <>
      <PanelError>{PANEL_COPY.emailUnverified}</PanelError>
      <VerificationResend service="saved_shows_calendar_panel">
        <VerificationResendButton
          variant="link"
          className={cn(
            'h-auto self-start p-0 text-sm font-medium',
            layout === 'sheet' && 'min-h-11'
          )}
        >
          {PANEL_COPY.verifyEmail}
        </VerificationResendButton>
        <VerificationResendStatus className="text-xs text-muted-foreground" />
        <VerificationResendSessionExpired>
          Your session has expired.{' '}
          <Link href={buildAuthHref(pathname)} className="underline">
            Sign in again
          </Link>{' '}
          to send the email.
        </VerificationResendSessionExpired>
        <VerificationResendFailed />
      </VerificationResend>
    </>
  )
}
