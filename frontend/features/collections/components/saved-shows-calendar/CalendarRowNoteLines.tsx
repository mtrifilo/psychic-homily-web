'use client'

import { ManageInSettingsLink, PanelError, PanelNote } from './CalendarPanelParts'
import { PANEL_COPY } from './savedShowsCalendarCopy'
import type { CalendarRowNote } from './useSavedShowsCalendarActions'

/**
 * What a row says under itself after it was chosen. Always mounted as a live
 * region, so a confirmation that appears in it is announced; it takes no space
 * while empty.
 */
export function CalendarRowNoteLines({
  note,
  onNavigate,
}: {
  note: CalendarRowNote | undefined
  onNavigate: () => void
}) {
  return (
    <div aria-live="polite" className="flex flex-col gap-0.5 not-empty:mt-0.5">
      {note ? <NoteContent note={note} onNavigate={onNavigate} /> : null}
    </div>
  )
}

function NoteContent({
  note,
  onNavigate,
}: {
  note: CalendarRowNote
  onNavigate: () => void
}) {
  switch (note) {
    case 'google_pending':
      return (
        <>
          {PANEL_COPY.googlePending.map(line => (
            <PanelNote key={line}>{line}</PanelNote>
          ))}
        </>
      )
    case 'apple_opened':
      return <PanelNote>{PANEL_COPY.appleOpened}</PanelNote>
    case 'outlook_opened':
      return <PanelNote>{PANEL_COPY.outlookOpened}</PanelNote>
    case 'copied':
      return <PanelNote>{PANEL_COPY.copied}</PanelNote>
    case 'create_failed':
      return <PanelError>{PANEL_COPY.createFailed}</PanelError>
    case 'load_failed':
      return <PanelError>{PANEL_COPY.loadFailed}</PanelError>
    case 'copy_failed':
      return (
        <>
          <PanelError>{PANEL_COPY.copyFailed}</PanelError>
          <ManageInSettingsLink onNavigate={onNavigate} />
        </>
      )
  }
}
