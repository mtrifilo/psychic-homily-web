'use client'

import type { ReactNode, Ref } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'
import {
  canAddOnDevice,
  type CalendarDevice,
} from '@/features/collections/lib/calendarDevice'
import {
  outlookComSubscribeUrl,
  outlookWorkSubscribeUrl,
  webcalUrl,
} from '@/lib/utils/calendarFeedUrls'
import type { CalendarLinkResetReason } from '@/features/shows/types'
import {
  CALENDAR_SURFACE_PROPS,
  CalendarAddressShownContext,
  CalendarPanelLayoutContext,
  PRIVATE_LINK_CLASS,
  PanelErrorWithRetry,
  PanelNote,
  holdFocusInPanel,
} from './CalendarPanelParts'
import { CalendarConnectedApps, CalendarStoppedApp } from './CalendarAppStatus'
import { CalendarEmailStepsAction } from './CalendarEmailStepsAction'
import { CalendarRowNoteLines } from './CalendarRowNoteLines'
import { PANEL_COPY } from './savedShowsCalendarCopy'
import type { CalendarRow, SavedShowsCalendarModel } from './savedShowsCalendarModel'
import {
  SETTINGS_CALENDAR_COPY as COPY,
  SETTINGS_RESET_SENTENCE,
} from './settingsCalendarCopy'
import { CalendarCardButton, CalendarCardSendButton } from './SettingsCalendarParts'
import { SettingsCalendarLinks } from './SettingsCalendarLinks'
import {
  useSavedShowsCalendarActions,
  type CalendarActions,
} from './useSavedShowsCalendarActions'
import { useSettingsCalendar, type CalendarLinks } from './useSettingsCalendar'

interface SettingsCalendarCardProps {
  /** The Settings fragment the card answers to. */
  id: string
  /** Settings' anchor scroll: lands a link to `#id` on the card. */
  anchorRef?: Ref<HTMLDivElement>
  className?: string
}

/**
 * Settings' one card for the saved shows calendar and the follows feed, which
 * one key makes: what is checking the calendar, the device's add buttons, and
 * the two addresses with New link and Turn off.
 *
 * Opening Settings never makes a link. Without one the card offers "Add to
 * calendar", and the add buttons appear once that click has made it: every
 * add button then acts inside its own click, with the address already read.
 */
export function SettingsCalendarCard({
  id,
  anchorRef,
  className,
}: SettingsCalendarCardProps) {
  return (
    <Card
      ref={anchorRef}
      id={id}
      tabIndex={-1}
      {...CALENDAR_SURFACE_PROPS}
      // The whole card is out of PostHog's autocapture and replay; Sentry's
      // replay masks text already, so `sentry-block` goes only on the
      // elements that carry an address (PRIVATE_LINK_CLASS).
      className={cn('ph-no-capture focus:outline-none', className)}
    >
      <CardHeader>
        <CardTitle className="text-base">{COPY.title}</CardTitle>
        <CardDescription>{COPY.sub}</CardDescription>
      </CardHeader>
      <CardContent>
        <CalendarPanelLayoutContext.Provider value="card">
          <CalendarAddressShownContext.Provider value>
            <SettingsCalendarBody />
          </CalendarAddressShownContext.Provider>
        </CalendarPanelLayoutContext.Provider>
      </CardContent>
    </Card>
  )
}

function SettingsCalendarBody() {
  const { state, add, onLinkWritten } = useSettingsCalendar()

  switch (state.kind) {
    case 'loading':
      return <PanelNote>{PANEL_COPY.loading}</PanelNote>
    case 'load_failed':
      return (
        <PanelErrorWithRetry onRetry={state.retry}>
          {PANEL_COPY.loadFailed}
        </PanelErrorWithRetry>
      )
    case 'no_link': {
      const addLink = (control: HTMLElement) =>
        add.mutate(state.resetReason, {
          // The button is replaced by the add buttons and the links.
          onSuccess: () => holdFocusInPanel(control),
        })
      return (
        <div className="flex flex-col gap-4">
          <StatusSentence>{statusSentence(state.resetReason)}</StatusSentence>
          <div className="flex flex-col gap-1">
            <CalendarCardButton
              working={add.isPending}
              onClick={event => addLink(event.currentTarget)}
              className="sm:self-start"
            >
              {COPY.addToCalendar}
            </CalendarCardButton>
            {add.isError && !add.isPending ? (
              <PanelErrorWithRetry onRetry={() => add.mutate(state.resetReason)}>
                {PANEL_COPY.createFailed}
              </PanelErrorWithRetry>
            ) : null}
          </div>
        </div>
      )
    }
    case 'link':
      return (
        <div className="flex flex-col gap-4">
          {/* Keyed by the address: a row's note about an add belongs to the
              link it added, not to one that replaced it. */}
          <CalendarAppsAndAdd
            key={state.links?.feedUrl ?? 'unrecoverable'}
            model={state.model}
            device={state.device}
            links={state.links}
          />
          <div role="separator" className="h-px bg-border" />
          <SettingsCalendarLinks
            links={state.links}
            onLinkWritten={onLinkWritten}
          />
        </div>
      )
  }
}

/** The status sentence: why the link was reset, or that nothing checks it. */
function statusSentence(resetReason: CalendarLinkResetReason | null): string {
  return resetReason ? SETTINGS_RESET_SENTENCE[resetReason] : COPY.noneChecking
}

function StatusSentence({ children }: { children: ReactNode }) {
  return <p className="text-sm leading-5 text-foreground">{children}</p>
}

/**
 * The status block, then either the add block or, for a link whose address
 * cannot be shown, the sentence saying a new link is needed first.
 */
function CalendarAppsAndAdd({
  model,
  device,
  links,
}: {
  model: SavedShowsCalendarModel
  device: CalendarDevice
  links: CalendarLinks | null
}) {
  const feedUrl = links?.feedUrl ?? null
  const actions = useSavedShowsCalendarActions(feedUrl ?? '')
  const anyApp = model.connected.length + model.stopped.length > 0

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        {model.resetReason || !anyApp ? (
          <StatusSentence>{statusSentence(model.resetReason)}</StatusSentence>
        ) : null}
        {model.connected.length > 0 ? (
          <CalendarConnectedApps apps={model.connected} />
        ) : null}
        {model.stopped.map(view => (
          <CalendarStoppedApp
            key={view.app.client}
            view={view}
            feedUrl={feedUrl}
            actions={actions}
          />
        ))}
      </div>
      {feedUrl ? (
        <AddBlock rows={model.rows} device={device} feedUrl={feedUrl} actions={actions} />
      ) : (
        <StatusSentence>
          {model.connected.length > 0 ? COPY.oldLinkConnected : COPY.oldLinkNoApp}
        </StatusSentence>
      )}
    </div>
  )
}

type AddButtonRow = Exclude<CalendarRow, 'copy'>

/**
 * The device's add buttons, from the model's rows: a connected app, or a
 * stopped one whose line already offers "Add to ... again", has none. Copy
 * link is not one of them; the links block has Copy. A phone gets no Google
 * button, only "Email me the steps".
 */
function AddBlock({
  rows,
  device,
  feedUrl,
  actions,
}: {
  rows: CalendarRow[]
  device: CalendarDevice
  feedUrl: string
  actions: CalendarActions
}) {
  const buttons = rows.filter((row): row is AddButtonRow => row !== 'copy')
  if (buttons.length === 0) return null
  // A row's note sits under its own button while the buttons stack (below
  // `sm`), and under the whole row once they sit side by side, where a note
  // under one narrow button would wrap to a column. Each note names its app,
  // so it reads right in either place. Only the visible copy is announced.
  return (
    <div className="flex flex-col">
      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-start">
        {buttons.map(row => (
          <div key={row} className="flex flex-col">
            <AddButton row={row} device={device} feedUrl={feedUrl} actions={actions} />
            <div className="sm:hidden">
              <CalendarRowNoteLines note={actions.notes[row]} />
            </div>
          </div>
        ))}
      </div>
      <div className="hidden sm:flex sm:flex-col">
        {buttons.map(row => (
          <CalendarRowNoteLines key={row} note={actions.notes[row]} />
        ))}
      </div>
    </div>
  )
}

function AddButton({
  row,
  device,
  feedUrl,
  actions,
}: {
  row: AddButtonRow
  device: CalendarDevice
  feedUrl: string
  actions: CalendarActions
}) {
  switch (row) {
    case 'google':
      if (!canAddOnDevice('google', device)) {
        return <CalendarEmailStepsAction sendControl={CalendarCardSendButton} />
      }
      return (
        <CalendarCardButton onClick={actions.addToGoogle} className={PRIVATE_LINK_CLASS}>
          {COPY.addGoogle}
        </CalendarCardButton>
      )
    case 'apple':
      // The model offers Apple only on an Apple device; an iPhone outside
      // Safari is told to open the page there or copy the link below.
      if (!canAddOnDevice('apple', device)) {
        return (
          <div className="flex flex-col gap-0.5">
            <p className="text-sm font-medium leading-5 text-foreground">
              {PANEL_COPY.rowApple}
            </p>
            <PanelNote>{PANEL_COPY.iphoneAppleNotSafari}</PanelNote>
          </div>
        )
      }
      return (
        <div className="flex flex-col gap-2">
          <CalendarCardButton asChild className={PRIVATE_LINK_CLASS}>
            <a href={webcalUrl(feedUrl)} onClick={() => actions.noteOpened('apple')}>
              {COPY.addApple}
            </a>
          </CalendarCardButton>
          {device.kind === 'iphone' ? (
            <PanelNote>{PANEL_COPY.iphoneGoogleAppHint}</PanelNote>
          ) : null}
        </div>
      )
    case 'outlook_com':
    case 'outlook_work':
      return (
        <CalendarCardButton asChild className={PRIVATE_LINK_CLASS}>
          <a
            href={
              row === 'outlook_com'
                ? outlookComSubscribeUrl(feedUrl)
                : outlookWorkSubscribeUrl(feedUrl)
            }
            target="_blank"
            rel="noopener noreferrer"
            onClick={() => actions.noteOpened(row)}
          >
            {row === 'outlook_com' ? COPY.outlookCom : COPY.outlookWork}
          </a>
        </CalendarCardButton>
      )
  }
}
