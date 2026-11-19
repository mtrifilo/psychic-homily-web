'use client'

import { Copy, ExternalLink } from 'lucide-react'
import {
  isCalendarComputer,
  type CalendarDevice,
} from '@/features/collections/lib/calendarDevice'
import {
  PanelBlockTitle,
  PanelNote,
  PanelRowButton,
  rowStackClass,
  useCalendarPanelLayout,
} from './CalendarPanelParts'
import { CalendarEmailStepsAction } from './CalendarEmailStepsAction'
import { CalendarRowNoteLines } from './CalendarRowNoteLines'
import { PANEL_COPY } from './savedShowsCalendarCopy'
import type { CalendarRow } from './savedShowsCalendarModel'
import type { CalendarActions } from './useSavedShowsCalendarActions'

interface CalendarAddRowsProps {
  rows: CalendarRow[]
  device: CalendarDevice
  actions: CalendarActions
  onNavigate: () => void
}

/** The add rows, in the order the model gives, each drawn for this device. */
export function CalendarAddRows({
  rows,
  device,
  actions,
  onNavigate,
}: CalendarAddRowsProps) {
  const layout = useCalendarPanelLayout()
  return (
    <div className={rowStackClass(layout)}>
      {rows.map(row => (
        <div key={row} className="flex flex-col">
          <CalendarAddRow row={row} device={device} actions={actions} />
          <CalendarRowNoteLines note={actions.notes[row]} onNavigate={onNavigate} />
        </div>
      ))}
    </div>
  )
}

function CalendarAddRow({
  row,
  device,
  actions,
}: {
  row: CalendarRow
  device: CalendarDevice
  actions: CalendarActions
}) {
  switch (row) {
    case 'google':
      if (isCalendarComputer(device)) {
        return (
          <PanelRowButton
            onClick={actions.addToGoogle}
            icon={ExternalLink}
            handlesLink
          >
            {PANEL_COPY.rowGoogle}
          </PanelRowButton>
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
    case 'apple':
      if (device.kind === 'iphone' && !device.isSafari) {
        return (
          <div className="flex flex-col">
            <PanelBlockTitle>{PANEL_COPY.rowApple}</PanelBlockTitle>
            <PanelNote className="mt-0.5">
              {PANEL_COPY.iphoneAppleNotSafari}
            </PanelNote>
          </div>
        )
      }
      return (
        <>
          <PanelRowButton
            onClick={actions.addToApple}
            icon={ExternalLink}
            handlesLink
          >
            {PANEL_COPY.rowApple}
          </PanelRowButton>
          {device.kind === 'iphone' ? (
            <PanelNote>{PANEL_COPY.iphoneGoogleAppHint}</PanelNote>
          ) : null}
        </>
      )
    case 'outlook_com':
    case 'outlook_work':
      return (
        <PanelRowButton
          onClick={() => actions.addToOutlook(row)}
          icon={ExternalLink}
          handlesLink
        >
          {row === 'outlook_com'
            ? PANEL_COPY.rowOutlookCom
            : PANEL_COPY.rowOutlookWork}
        </PanelRowButton>
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
