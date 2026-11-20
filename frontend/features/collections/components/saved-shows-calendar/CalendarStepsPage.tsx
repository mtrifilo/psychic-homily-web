'use client'

import { Fragment, type ReactNode } from 'react'
import { ExternalLink, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuthRouteGuard } from '@/lib/hooks/common/useAuthRouteGuard'
import {
  canAddOnDevice,
  isCalendarComputer,
  useCalendarDevice,
  type CalendarDevice,
} from '@/features/collections/lib/calendarDevice'
import {
  GOOGLE_CALENDAR_ADD_BY_URL_PAGE,
  outlookComSubscribeUrl,
  outlookWorkSubscribeUrl,
  webcalUrl,
} from '@/lib/utils/calendarFeedUrls'
import { cn } from '@/lib/utils'
import {
  CALENDAR_LINK_TEXT_CLASS,
  CALENDAR_SURFACE_PROPS,
  CalendarPanelLayoutContext,
  LinkFailedLine,
  ManageInSettingsLink,
  PRIVATE_LINK_CLASS,
  PanelNote,
  PanelRowLink,
} from './CalendarPanelParts'
import { CalendarConnectedApps } from './CalendarAppStatus'
import { CalendarRowNoteLines } from './CalendarRowNoteLines'
import { PANEL_COPY } from './savedShowsCalendarCopy'
import { STEPS_PAGE_COPY } from './calendarStepsPageCopy'
import { useSavedShowsCalendarActions } from './useSavedShowsCalendarActions'
import {
  useCalendarStepsLink,
  useGoogleCalendarCheck,
  type CalendarStepsLink,
} from './useCalendarStepsPage'

const BODY_TEXT = 'text-sm leading-normal text-foreground'

/**
 * The page "Email me the steps" opens: Google's two-step for the private
 * link, a step at a time, on a computer; on a phone, why it needs a computer
 * and what the phone can do instead. Signed in only: a signed-out reader is
 * sent to sign in and comes back here.
 *
 * Everything below the heading depends on the device, which only the browser
 * knows, so the page renders its loading state until it has hydrated. The
 * address is copied or handed to a calendar app, never shown: every element
 * that carries it has PRIVATE_LINK_CLASS.
 */
export function CalendarStepsPage() {
  const gate = useAuthRouteGuard('redirect')
  const device = useCalendarDevice()

  // Only 'loading' and 'ready' reach here: 'redirect' mode throws.
  if (gate !== 'ready' || !device) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    )
  }

  const computer = isCalendarComputer(device)
  return (
    <div className="container mx-auto max-w-6xl px-4 py-5 md:py-10">
      {/* Focusable, so a retry line that unmounts as it is pressed leaves
          focus here rather than on the body (holdFocusInPanel). */}
      <div
        {...CALENDAR_SURFACE_PROPS}
        tabIndex={-1}
        className="flex max-w-2xl flex-col gap-4 focus:outline-none md:gap-6"
      >
        <h1 className="pb-1 font-display text-2xl font-semibold tracking-tight text-foreground md:text-[28px] md:leading-9">
          {STEPS_PAGE_COPY.heading}
        </h1>
        {/* A phone's controls get the sheet's 44px tap targets. */}
        <CalendarPanelLayoutContext.Provider value={computer ? 'popover' : 'sheet'}>
          {computer ? (
            <ComputerSteps device={device} />
          ) : device.kind === 'iphone' ? (
            <IphoneSteps />
          ) : (
            <AndroidSteps />
          )}
        </CalendarPanelLayoutContext.Provider>
      </div>
    </div>
  )
}

function ComputerSteps({ device }: { device: CalendarDevice }) {
  const link = useCalendarStepsLink()

  if (link.kind === 'unrecoverable') return <UnrecoverableLink />

  return (
    <>
      {/* Numbered because the order matters. */}
      <ol className="flex flex-col gap-4">
        <Step number={1} text={STEPS_PAGE_COPY.stepCopy}>
          <CopyLinkStep link={link} />
        </Step>
        <Step number={2} text={STEPS_PAGE_COPY.stepOpen}>
          <Button asChild variant="outline" size="sm" className="self-start">
            <a
              href={GOOGLE_CALENDAR_ADD_BY_URL_PAGE}
              target="_blank"
              rel="noopener noreferrer"
            >
              {STEPS_PAGE_COPY.openButton}
            </a>
          </Button>
        </Step>
        <Step number={3} text={STEPS_PAGE_COPY.stepPaste} />
      </ol>
      <GoogleCheckStatus pollUntilGoogle={link.kind === 'ready'} />
      {link.kind === 'ready' ? (
        <>
          <div role="separator" className="h-px bg-border" />
          <OtherCalendars feedUrl={link.feedUrl} device={device} />
        </>
      ) : null}
    </>
  )
}

function Step({
  number,
  text,
  children,
}: {
  number: number
  text: string
  children?: ReactNode
}) {
  return (
    <li className="flex gap-2.5">
      {/* The list announces its own numbering. */}
      <span
        aria-hidden="true"
        className="w-[18px] shrink-0 text-sm font-medium leading-5 text-foreground"
      >
        {number}.
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <p className={BODY_TEXT}>{text}</p>
        {children}
      </div>
    </li>
  )
}

/**
 * Step 1's button. Disabled until the link is ready, so the copy always runs
 * inside the click; the link failing to load or be made replaces it with that
 * line and its retry.
 */
function CopyLinkStep({ link }: { link: CalendarStepsLink }) {
  const actions = useSavedShowsCalendarActions(
    link.kind === 'ready' ? link.feedUrl : ''
  )
  if (link.kind === 'failed') {
    return <LinkFailedLine failure={link.failure} onRetry={link.retry} />
  }
  const note = actions.notes.copy
  return (
    <div className="flex flex-col">
      {/* A live region, so the label turning to "Copied" is announced. */}
      <div aria-live="polite" className="flex">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={link.kind !== 'ready'}
          onClick={actions.copyLink}
          className={PRIVATE_LINK_CLASS}
        >
          {note === 'copied'
            ? STEPS_PAGE_COPY.copiedButton
            : STEPS_PAGE_COPY.copyButton}
        </Button>
      </div>
      {/* The button's own label confirms a copy; only a refusal adds lines. */}
      <CalendarRowNoteLines note={note === 'copy_failed' ? note : undefined} />
    </div>
  )
}

/** The line under the steps: waiting for Google, or Google's status block. */
function GoogleCheckStatus({ pollUntilGoogle }: { pollUntilGoogle: boolean }) {
  const check = useGoogleCalendarCheck({ pollUntilGoogle })
  return (
    <div aria-live="polite" className="empty:hidden">
      {check.kind === 'failed' ? (
        <LinkFailedLine failure="load" onRetry={check.retry} />
      ) : check.kind === 'waiting' ? (
        <p className="text-sm leading-normal text-muted-foreground">
          {STEPS_PAGE_COPY.waiting}
        </p>
      ) : check.kind === 'connected' ? (
        <div className="flex flex-col gap-1">
          <CalendarConnectedApps
            apps={[{ app: check.google, checkedLine: check.checkedLine }]}
          />
          <p className={BODY_TEXT}>{STEPS_PAGE_COPY.done}</p>
        </div>
      ) : null}
    </div>
  )
}

/** "Using a different calendar?" and the calendars this computer can add. */
function OtherCalendars({
  feedUrl,
  device,
}: {
  feedUrl: string
  device: CalendarDevice
}) {
  const calendars = [
    ...(canAddOnDevice('apple', device)
      ? [{ label: PANEL_COPY.rowApple, href: webcalUrl(feedUrl), newTab: false }]
      : []),
    {
      label: PANEL_COPY.rowOutlookCom,
      href: outlookComSubscribeUrl(feedUrl),
      newTab: true,
    },
    {
      label: PANEL_COPY.rowOutlookWork,
      href: outlookWorkSubscribeUrl(feedUrl),
      newTab: true,
    },
  ]
  return (
    <p className="text-sm leading-normal text-muted-foreground">
      {STEPS_PAGE_COPY.otherCalendarsLead}{' '}
      {calendars.map(({ label, href, newTab }, index) => (
        <Fragment key={label}>
          {index > 0 ? <span aria-hidden="true"> · </span> : null}
          <a
            href={href}
            className={cn(CALENDAR_LINK_TEXT_CLASS, PRIVATE_LINK_CLASS)}
            {...(newTab ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
          >
            {label}
          </a>
        </Fragment>
      ))}
    </p>
  )
}

/**
 * A link whose address cannot be shown again: nothing here can act, so the
 * page says so as the panel does and points to Settings. Its last sentence,
 * that a new link disconnects what is connected, waits for the status.
 */
function UnrecoverableLink() {
  const check = useGoogleCalendarCheck({ pollUntilGoogle: false })
  if (check.kind === 'unknown') return null
  if (check.kind === 'failed') {
    return <LinkFailedLine failure="load" onRetry={check.retry} />
  }
  return (
    <div className="flex flex-col gap-1">
      <p className={BODY_TEXT}>
        {check.anyAppConnected
          ? PANEL_COPY.unrecoverable
          : PANEL_COPY.unrecoverableNoApp}
      </p>
      <ManageInSettingsLink />
    </div>
  )
}

function IphoneSteps() {
  const link = useCalendarStepsLink()
  const needsComputer = <p className={BODY_TEXT}>{STEPS_PAGE_COPY.phoneNeedsComputer}</p>

  if (link.kind === 'unrecoverable') {
    return (
      <>
        {needsComputer}
        <UnrecoverableLink />
      </>
    )
  }

  return (
    <>
      {needsComputer}
      <div className="flex flex-col">
        <p className={BODY_TEXT}>{STEPS_PAGE_COPY.iphoneAppleInstead}</p>
        {link.kind === 'ready' ? (
          <>
            <PanelRowLink
              href={webcalUrl(link.feedUrl)}
              icon={ExternalLink}
              handlesLink
            >
              {PANEL_COPY.rowApple}
            </PanelRowLink>
            <PanelNote>{PANEL_COPY.iphoneGoogleAppHint}</PanelNote>
          </>
        ) : link.kind === 'failed' ? (
          <LinkFailedLine failure={link.failure} onRetry={link.retry} />
        ) : null}
      </div>
    </>
  )
}

function AndroidSteps() {
  return (
    <>
      <p className={BODY_TEXT}>{STEPS_PAGE_COPY.phoneNeedsComputer}</p>
      <p className={BODY_TEXT}>{PANEL_COPY.androidPointer}</p>
    </>
  )
}
