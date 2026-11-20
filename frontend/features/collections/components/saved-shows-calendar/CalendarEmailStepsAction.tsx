'use client'

import { useState, type ComponentType } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import type React from 'react'
import {
  emailCalendarStepsFailure,
  useEmailCalendarSteps,
  type EmailCalendarStepsFailure,
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
  PanelErrorWithRetry,
  PanelNote,
  PanelRowButton,
  holdFocusInPanel,
  tapTargetClass,
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
 *
 * `sendControl` draws the send controls; the panel's text-link row unless a
 * surface draws its own (the Settings card's outline buttons).
 */
export function CalendarEmailStepsAction({
  sendControl: SendControl = PanelRowButton,
}: {
  sendControl?: ComponentType<CalendarSendControlProps>
} = {}) {
  const { user } = useAuthContext()
  const send = useEmailCalendarSteps()
  // The last settled outcome, kept through a send in flight so neither the
  // confirmation nor a failure line flashes while it is pending.
  const [outcome, setOutcome] = useState<
    'sent' | EmailCalendarStepsFailure | null
  >(null)

  const onSend = () => {
    send.mutate(undefined, {
      onSuccess: () => setOutcome('sent'),
      onError: error => setOutcome(emailCalendarStepsFailure(error)),
    })
  }
  // The send controls swap or disappear as the outcome changes.
  const sendFromControl = (event: React.MouseEvent<HTMLElement>) => {
    holdFocusInPanel(event.currentTarget)
    onSend()
  }

  return (
    <div className="flex flex-col">
      {outcome === 'sent' ? (
        <>
          <PanelNote className="mt-0.5">
            {emailSentLine(user?.email ?? '')}
          </PanelNote>
          <SendControl onClick={sendFromControl} disabled={send.isPending}>
            {PANEL_COPY.sendAgain}
          </SendControl>
        </>
      ) : (
        <SendControl
          onClick={sendFromControl}
          disabled={send.isPending}
          testId="calendar-email-steps"
        >
          {PANEL_COPY.emailSteps}
        </SendControl>
      )}
      {outcome === 'unverified' ? <VerifyEmailInPlace /> : null}
      {outcome === 'rate_limited' ? (
        <PanelError>{PANEL_COPY.emailRateLimited}</PanelError>
      ) : null}
      {outcome === 'failed' ? (
        <PanelErrorWithRetry onRetry={onSend} retrying={send.isPending}>
          {PANEL_COPY.emailFailed}
        </PanelErrorWithRetry>
      ) : null}
    </div>
  )
}

/** What a send control is given: "Email me the steps" or "Send again". */
export interface CalendarSendControlProps {
  onClick: (event: React.MouseEvent<HTMLButtonElement>) => void
  disabled?: boolean
  testId?: string
  children: string
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
            tapTargetClass(layout)
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
