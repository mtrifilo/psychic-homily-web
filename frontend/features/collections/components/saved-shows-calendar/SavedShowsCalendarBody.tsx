'use client'

import { Download } from 'lucide-react'
import { API_ENDPOINTS } from '@/lib/api'
import {
  ManageInSettingsLink,
  PanelNote,
  PanelRowLink,
  useCalendarPanelLayout,
} from './CalendarPanelParts'
import { CalendarAddRows } from './CalendarAddRows'
import { CalendarConnectedApps, CalendarStoppedApp } from './CalendarAppStatus'
import { PANEL_COPY, downloadLabel } from './savedShowsCalendarCopy'
import type { SavedShowsCalendarModel } from './savedShowsCalendarModel'
import type { CalendarDevice } from '@/features/collections/lib/calendarDevice'
import { useSavedShowsCalendarActions } from './useSavedShowsCalendarActions'
import { cn } from '@/lib/utils'

interface SavedShowsCalendarBodyProps {
  model: SavedShowsCalendarModel
  device: CalendarDevice
  /** The private address; null when it cannot be shown, where no row acts. */
  feedUrl: string | null
  /** Called when a link leaves the panel for another page. */
  onNavigate: () => void
}

/**
 * The panel under its heading, section by section in the board's order:
 * connected apps, stopped apps, the old-link notice, the add rows, Android's
 * per-show pointer, the download, and the way to Settings.
 */
export function SavedShowsCalendarBody({
  model,
  device,
  feedUrl,
  onNavigate,
}: SavedShowsCalendarBodyProps) {
  const layout = useCalendarPanelLayout()
  const actions = useSavedShowsCalendarActions(feedUrl ?? '')
  const hasDownload = model.downloadCount > 0

  return (
    <div className="flex flex-col gap-3">
      {model.connected.length > 0 ? (
        <CalendarConnectedApps apps={model.connected} />
      ) : null}
      {model.stopped.map(view => (
        <CalendarStoppedApp
          key={view.app.client}
          view={view}
          feedUrl={feedUrl}
          actions={actions}
          onNavigate={onNavigate}
        />
      ))}
      {model.unrecoverable ? (
        <PanelNote>
          {model.connected.length > 0
            ? PANEL_COPY.unrecoverable
            : PANEL_COPY.unrecoverableNoApp}
        </PanelNote>
      ) : null}
      {feedUrl && model.rows.length > 0 ? (
        <div
          className={cn(
            'flex flex-col',
            layout === 'popover' && model.showAddAnother && 'gap-2.5'
          )}
        >
          {model.showAddAnother ? (
            <PanelNote>{PANEL_COPY.addAnother}</PanelNote>
          ) : null}
          <CalendarAddRows
            rows={model.rows}
            device={device}
            feedUrl={feedUrl}
            actions={actions}
            onNavigate={onNavigate}
          />
        </div>
      ) : null}
      {model.showPerShowPointer ? (
        <PanelNote>{PANEL_COPY.androidPointer}</PanelNote>
      ) : null}
      {hasDownload ? (
        <>
          <div role="separator" className="h-px bg-border" />
          <div className="flex flex-col gap-0.5">
            <PanelRowLink
              href={API_ENDPOINTS.CALENDAR.SAVED_SHOWS_DOWNLOAD}
              icon={Download}
            >
              {downloadLabel(model.downloadCount)}
            </PanelRowLink>
            <PanelNote>{PANEL_COPY.downloadCaption}</PanelNote>
          </div>
        </>
      ) : null}
      {model.showManageInSettings ? (
        <ManageInSettingsLink onNavigate={onNavigate} />
      ) : null}
    </div>
  )
}
