'use client'

import { Check } from 'lucide-react'
import { useForgetCalendarClient } from '@/features/auth/hooks/useCalendarFeed'
import {
  PanelErrorWithRetry,
  PanelNote,
  PanelRowButton,
  holdFocusInSurface,
} from './CalendarPanelParts'
import { CalendarRowNoteLines } from './CalendarRowNoteLines'
import { AddTargetControl } from './CalendarAddRows'
import { PANEL_COPY, addAgainLabel } from './savedShowsCalendarCopy'
import type {
  ConnectedAppView,
  StoppedAppView,
} from './savedShowsCalendarModel'
import type { CalendarActions } from './useSavedShowsCalendarActions'

/** One two-line block per connected app: its name, then when it checked. */
export function CalendarConnectedApps({ apps }: { apps: ConnectedAppView[] }) {
  return (
    <ul className="flex flex-col gap-2">
      {apps.map(({ app, checkedLine }) => (
        <li key={app.client} className="flex flex-col gap-0.5">
          <p className="flex items-center gap-1.5 text-sm font-medium leading-5 text-foreground">
            <Check className="size-3.5 shrink-0" aria-hidden="true" />
            {app.name}
          </p>
          <PanelNote>{checkedLine}</PanelNote>
        </li>
      ))}
    </ul>
  )
}

interface CalendarStoppedAppProps {
  view: StoppedAppView
  /** Null when the address cannot be shown, where no "again" link is offered. */
  feedUrl: string | null
  actions: CalendarActions
  onNavigate: () => void
}

/**
 * A stopped app: its line, "Add to ... again" when this device can add it,
 * and "I removed it", which forgets the app so the panel offers its row as
 * new.
 */
export function CalendarStoppedApp({
  view,
  feedUrl,
  actions,
  onNavigate,
}: CalendarStoppedAppProps) {
  const forget = useForgetCalendarClient()
  const { againTarget } = view
  const forgetApp = () => forget.mutate(view.app.client)

  return (
    <div className="flex flex-col gap-2">
      <PanelNote>{view.stoppedLine}</PanelNote>
      {againTarget && feedUrl ? (
        <div className="flex flex-col">
          <AddTargetControl target={againTarget} feedUrl={feedUrl} actions={actions}>
            {addAgainLabel(againTarget)}
          </AddTargetControl>
          <CalendarRowNoteLines
            note={actions.notes[againTarget]}
            onNavigate={onNavigate}
          />
        </div>
      ) : null}
      <div className="flex flex-col">
        <PanelRowButton
          onClick={event => {
            // This block disappears once the app is forgotten.
            holdFocusInSurface(event.currentTarget)
            forgetApp()
          }}
          disabled={forget.isPending}
        >
          {PANEL_COPY.removedIt}
        </PanelRowButton>
        {forget.isError ? (
          <PanelErrorWithRetry onRetry={forgetApp}>
            {PANEL_COPY.forgetFailed}
          </PanelErrorWithRetry>
        ) : null}
      </div>
    </div>
  )
}
