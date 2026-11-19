'use client'

import { Copy, ExternalLink } from 'lucide-react'
import {
  canAddOnDevice,
  type CalendarDevice,
} from '@/features/collections/lib/calendarDevice'
import type { CalendarAddTarget } from '@/features/collections/lib/calendarFeedStatus'
import {
  outlookComSubscribeUrl,
  outlookWorkSubscribeUrl,
  webcalUrl,
} from '@/lib/utils/calendarFeedUrls'
import {
  PanelBlockTitle,
  PanelNote,
  PanelRowButton,
  PanelRowLink,
  rowStackClass,
  useCalendarPanelLayout,
} from './CalendarPanelParts'
import { CalendarEmailStepsAction } from './CalendarEmailStepsAction'
import { CalendarRowNoteLines } from './CalendarRowNoteLines'
import { ADD_TARGET_ROW, PANEL_COPY } from './savedShowsCalendarCopy'
import type { CalendarRow } from './savedShowsCalendarModel'
import type { CalendarActions } from './useSavedShowsCalendarActions'

interface CalendarAddRowsProps {
  rows: CalendarRow[]
  device: CalendarDevice
  feedUrl: string
  actions: CalendarActions
  onNavigate: () => void
}

/** The add rows, in the order the model gives, each drawn for this device. */
export function CalendarAddRows({
  rows,
  device,
  feedUrl,
  actions,
  onNavigate,
}: CalendarAddRowsProps) {
  const layout = useCalendarPanelLayout()
  return (
    <div className={rowStackClass(layout)}>
      {rows.map(row => (
        <div key={row} className="flex flex-col">
          <CalendarAddRow
            row={row}
            device={device}
            feedUrl={feedUrl}
            actions={actions}
          />
          <CalendarRowNoteLines note={actions.notes[row]} onNavigate={onNavigate} />
        </div>
      ))}
    </div>
  )
}

function CalendarAddRow({
  row,
  device,
  feedUrl,
  actions,
}: {
  row: CalendarRow
  device: CalendarDevice
  feedUrl: string
  actions: CalendarActions
}) {
  switch (row) {
    case 'google':
    case 'apple':
      // A device that cannot add the app in one step gets its block of
      // instructions in place of the row.
      if (!canAddOnDevice(row, device)) {
        return <PhoneOnlyBlock row={row} device={device} />
      }
      return (
        <>
          <AddTargetControl target={row} feedUrl={feedUrl} actions={actions}>
            {ADD_TARGET_ROW[row]}
          </AddTargetControl>
          {row === 'apple' && device.kind === 'iphone' ? (
            <PanelNote>{PANEL_COPY.iphoneGoogleAppHint}</PanelNote>
          ) : null}
        </>
      )
    case 'outlook_com':
    case 'outlook_work':
      return (
        <PanelRowLink
          href={
            row === 'outlook_com'
              ? outlookComSubscribeUrl(feedUrl)
              : outlookWorkSubscribeUrl(feedUrl)
          }
          newTab
          handlesLink
          icon={ExternalLink}
          onClick={() => actions.noteOpened(row)}
        >
          {row === 'outlook_com'
            ? PANEL_COPY.rowOutlookCom
            : PANEL_COPY.rowOutlookWork}
        </PanelRowLink>
      )
    case 'copy':
      return (
        <PanelRowButton
          onClick={actions.copyLink}
          icon={Copy}
          handlesLink
          testId="calendar-copy-link"
        >
          {PANEL_COPY.rowCopy}
        </PanelRowButton>
      )
  }
}

/**
 * The one-step add for an app with a row of its own, shared by its row and by
 * a stopped app's "Add to ... again": Google's two-step, or a webcal link that
 * hands the address to Apple Calendar.
 */
export function AddTargetControl({
  target,
  feedUrl,
  actions,
  children,
}: {
  target: CalendarAddTarget
  feedUrl: string
  actions: CalendarActions
  children: string
}) {
  if (target === 'google') {
    return (
      <PanelRowButton onClick={actions.addToGoogle} icon={ExternalLink} handlesLink>
        {children}
      </PanelRowButton>
    )
  }
  return (
    <PanelRowLink
      href={webcalUrl(feedUrl)}
      handlesLink
      icon={ExternalLink}
      onClick={() => actions.noteOpened('apple')}
    >
      {children}
    </PanelRowLink>
  )
}

/**
 * Google on a phone (finish on a computer, or have the steps emailed), and
 * Apple on an iPhone outside Safari (open the page in Safari).
 */
function PhoneOnlyBlock({
  row,
  device,
}: {
  row: 'google' | 'apple'
  device: CalendarDevice
}) {
  if (row === 'apple') {
    return (
      <div className="flex flex-col">
        <PanelBlockTitle>{PANEL_COPY.rowApple}</PanelBlockTitle>
        <PanelNote className="mt-0.5">{PANEL_COPY.iphoneAppleNotSafari}</PanelNote>
      </div>
    )
  }
  return (
    <div className="flex flex-col">
      <PanelBlockTitle>{PANEL_COPY.rowGoogle}</PanelBlockTitle>
      <PanelNote className="mt-0.5">
        {device.kind === 'android'
          ? PANEL_COPY.androidGoogleSub
          : PANEL_COPY.iphoneGoogleSub}
      </PanelNote>
      <CalendarEmailStepsAction />
    </div>
  )
}
