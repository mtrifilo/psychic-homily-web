'use client'

import { Fragment, type ReactNode } from 'react'
import { Copy, ExternalLink, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuthRouteGuard } from '@/lib/hooks/common/useAuthRouteGuard'
import {
  canAddOnDevice,
  isCalendarComputer,
  useCalendarDevice,
  type CalendarDevice,
} from '@/features/collections/lib/calendarDevice'
import {
  CALENDAR_CLIENT_ADD_TARGET,
  calendarCheckedLine,
  calendarStoppedLine,
  type CalendarApp,
} from '@/features/collections/lib/calendarFeedStatus'
import type { CalendarLinkResetReason } from '@/features/shows/types'
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
  PanelRowButton,
  PanelRowLink,
  holdFocusInPanel,
  useCalendarPanelLayout,
} from './CalendarPanelParts'
import { CalendarConnectedApps } from './CalendarAppStatus'
import { CalendarRowNoteLines } from './CalendarRowNoteLines'
import { PANEL_COPY, RESET_SUB } from './savedShowsCalendarCopy'
import { STEPS_PAGE_COPY } from './calendarStepsPageCopy'
import { useSavedShowsCalendarActions } from './useSavedShowsCalendarActions'
import {
  useCalendarStepsPage,
  type CalendarStepsLink,
} from './useCalendarStepsPage'

const BODY_TEXT = 'text-sm leading-normal text-foreground'
const MUTED_TEXT = 'text-sm leading-normal text-muted-foreground'

/**
 * The page "Email me the steps" opens: Google's two-step for the private
 * link, a step at a time, on a computer; on a phone, why it needs a computer
 * and what the phone can do instead. Signed in only: a signed-out reader is
 * sent to sign in and comes back here.
 *
 * Arriving never makes a link. With none, the page offers one Add to calendar
 * button, and the person's click makes it. Everything below the heading
 * depends on the device, which only the browser knows, so the page renders its
 * loading state until it has hydrated. The address is copied or handed to a
 * calendar app, never shown: every element that carries it has
 * PRIVATE_LINK_CLASS.
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
      {/* Focusable, so a control that unmounts as it is pressed leaves focus
          here rather than on the body (holdFocusInPanel). */}
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
            <IphoneSteps device={device} />
          ) : (
            <AndroidSteps />
          )}
        </CalendarPanelLayoutContext.Provider>
      </div>
    </div>
  )
}

function ComputerSteps({ device }: { device: CalendarDevice }) {
  const page = useCalendarStepsPage({ pollGoogle: true })

  if (page.kind === 'loading') return <ReadingSpinner />
  if (page.kind === 'read_failed') {
    return <LinkFailedLine failure="load" onRetry={page.retry} />
  }
  const { link, apps, resetReason, now } = page
  if (link.kind === 'unrecoverable') {
    return <UnrecoverableLink anyAppConnected={apps.connected.length > 0} />
  }

  return (
    <>
      {resetReason ? <ResetNotice reason={resetReason} /> : null}
      {link.kind === 'none' ? (
        <AddToCalendar link={link} />
      ) : (
        <>
          {/* Numbered because the order matters. */}
          <ol className="flex flex-col gap-4">
            <Step number={1} text={STEPS_PAGE_COPY.stepCopy}>
              <CopyLinkStep feedUrl={link.feedUrl} />
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
          <GoogleCheckLine
            google={appFor(apps.connected, 'google')}
            stoppedGoogle={appFor(apps.stopped, 'google')}
            now={now}
          />
          <div role="separator" className="h-px bg-border" />
          <OtherCalendars
            feedUrl={link.feedUrl}
            device={device}
            appleConnected={apps.connected.some(isApple)}
          />
        </>
      )}
    </>
  )
}

function appFor(apps: CalendarApp[], client: CalendarApp['client']) {
  return apps.find(app => app.client === client)
}

/** Either Apple family: both are added through the one Apple Calendar link. */
function isApple(app: CalendarApp): boolean {
  return CALENDAR_CLIENT_ADD_TARGET[app.client] === 'apple'
}

/** Shown until the page's reads finish; nothing can act before then. */
function ReadingSpinner() {
  return (
    <Loader2
      aria-hidden="true"
      className="size-4 animate-spin text-muted-foreground"
    />
  )
}

/** Why a link that was in use stopped: the panel's reset lines. */
function ResetNotice({ reason }: { reason: CalendarLinkResetReason }) {
  return (
    <div className="flex flex-col gap-0.5">
      <h2 className="text-sm font-semibold leading-5 text-foreground">
        {PANEL_COPY.headingReset}
      </h2>
      <p className={BODY_TEXT}>{RESET_SUB[reason]}</p>
    </div>
  )
}

/**
 * The one control while the account has no link. The click makes the link,
 * and the controls that act on it appear, each acting in its own click.
 */
function AddToCalendar({
  link,
}: {
  link: Extract<CalendarStepsLink, { kind: 'none' }>
}) {
  const layout = useCalendarPanelLayout()
  return (
    <div className="flex flex-col gap-1">
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={link.creating}
        onClick={event => {
          // The button is replaced by the controls once the link exists.
          holdFocusInPanel(event.currentTarget)
          link.create()
        }}
        className={cn('self-start', layout === 'sheet' && 'min-h-11')}
      >
        {link.creating ? (
          <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
        ) : null}
        {STEPS_PAGE_COPY.addToCalendar}
      </Button>
      {link.createFailed && !link.creating ? (
        <LinkFailedLine failure="create" onRetry={link.create} />
      ) : null}
    </div>
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
      {/* The page's styles remove the list's own markers, so the number is
          text that every reader, screen readers included, reads. */}
      <span className="w-[18px] shrink-0 text-sm font-medium leading-5 text-foreground">
        {number}.
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <p className={BODY_TEXT}>{text}</p>
        {children}
      </div>
    </li>
  )
}

/** Step 1's button: it copies the address in its own click. */
function CopyLinkStep({ feedUrl }: { feedUrl: string }) {
  const actions = useSavedShowsCalendarActions(feedUrl)
  const note = actions.notes.copy
  return (
    <div className="flex flex-col">
      {/* A live region, so the label turning to "Copied" is announced. */}
      <div aria-live="polite" className="flex">
        <Button
          type="button"
          variant="outline"
          size="sm"
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

/**
 * The line under the steps: Google's status block once it is connected; the
 * stopped line while a Google calendar that checked before has stopped; the
 * waiting line otherwise.
 */
function GoogleCheckLine({
  google,
  stoppedGoogle,
  now,
}: {
  google: CalendarApp | undefined
  stoppedGoogle: CalendarApp | undefined
  now: Date
}) {
  return (
    <div aria-live="polite">
      {google ? (
        <div className="flex flex-col gap-1">
          <CalendarConnectedApps
            apps={[
              {
                app: google,
                // The page states no comparison with saves.
                checkedLine: calendarCheckedLine(google.lastReadAt, null, now),
              },
            ]}
          />
          <p className={BODY_TEXT}>{STEPS_PAGE_COPY.done}</p>
        </div>
      ) : stoppedGoogle ? (
        <p className={MUTED_TEXT}>{calendarStoppedLine(stoppedGoogle, now)}</p>
      ) : (
        <p className={MUTED_TEXT}>{STEPS_PAGE_COPY.waiting}</p>
      )}
    </div>
  )
}

/**
 * "Using a different calendar?" and the calendars this computer can add. A
 * connected app's own add link is not shown.
 */
function OtherCalendars({
  feedUrl,
  device,
  appleConnected,
}: {
  feedUrl: string
  device: CalendarDevice
  appleConnected: boolean
}) {
  const calendars = [
    ...(canAddOnDevice('apple', device) && !appleConnected
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
    <p className={MUTED_TEXT}>
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
 * page says so in the panel's words and points to Settings. Its last
 * sentence, that a new link disconnects what is connected, is only said when
 * something is connected.
 */
function UnrecoverableLink({ anyAppConnected }: { anyAppConnected: boolean }) {
  return (
    <div className="flex flex-col gap-1">
      <p className={BODY_TEXT}>
        {anyAppConnected
          ? PANEL_COPY.unrecoverable
          : PANEL_COPY.unrecoverableNoApp}
      </p>
      <ManageInSettingsLink />
    </div>
  )
}

function IphoneSteps({ device }: { device: CalendarDevice }) {
  const page = useCalendarStepsPage({ pollGoogle: false })
  const needsComputer = (
    <p className={BODY_TEXT}>{STEPS_PAGE_COPY.phoneNeedsComputer}</p>
  )

  if (page.kind !== 'ready') {
    return (
      <>
        {needsComputer}
        {page.kind === 'read_failed' ? (
          <LinkFailedLine failure="load" onRetry={page.retry} />
        ) : (
          <ReadingSpinner />
        )}
      </>
    )
  }

  const { link, apps, resetReason, now } = page
  const connectedApple = apps.connected.filter(isApple)
  if (connectedApple.length > 0) {
    // Apple Calendar already has it: its status, and the Google app hint.
    return (
      <>
        {needsComputer}
        <div className="flex flex-col gap-2">
          <CalendarConnectedApps
            apps={connectedApple.map(app => ({
              app,
              checkedLine: calendarCheckedLine(app.lastReadAt, null, now),
            }))}
          />
          <PanelNote>{PANEL_COPY.iphoneGoogleAppHint}</PanelNote>
        </div>
      </>
    )
  }

  if (link.kind === 'unrecoverable') {
    return (
      <>
        {needsComputer}
        <UnrecoverableLink anyAppConnected={apps.connected.length > 0} />
      </>
    )
  }

  const stoppedApple = apps.stopped.find(isApple)
  return (
    <>
      {needsComputer}
      {resetReason ? <ResetNotice reason={resetReason} /> : null}
      {stoppedApple ? (
        <p className={MUTED_TEXT}>{calendarStoppedLine(stoppedApple, now)}</p>
      ) : null}
      <div className="flex flex-col">
        <p className={BODY_TEXT}>{STEPS_PAGE_COPY.iphoneAppleInstead}</p>
        {link.kind === 'none' ? (
          <div className="pt-2">
            <AddToCalendar link={link} />
          </div>
        ) : canAddOnDevice('apple', device) ? (
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
        ) : (
          <CopyOutsideSafari feedUrl={link.feedUrl} />
        )}
      </div>
    </>
  )
}

/**
 * An iPhone browser other than Safari, which has not been shown to hand a
 * webcal link to Apple Calendar: the panel's line for it, and Copy link. It
 * stands in place of the Google Calendar app hint, as in the panel.
 */
function CopyOutsideSafari({ feedUrl }: { feedUrl: string }) {
  const actions = useSavedShowsCalendarActions(feedUrl)
  return (
    <div className="flex flex-col pt-2">
      <PanelNote>{PANEL_COPY.iphoneAppleNotSafari}</PanelNote>
      <PanelRowButton
        onClick={actions.copyLink}
        icon={Copy}
        handlesLink
        testId="calendar-copy-link"
      >
        {PANEL_COPY.rowCopy}
      </PanelRowButton>
      <CalendarRowNoteLines note={actions.notes.copy} />
    </div>
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
