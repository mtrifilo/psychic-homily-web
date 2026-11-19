'use client'

import { Check, ExternalLink } from 'lucide-react'
import { useForgetCalendarClient } from '@/features/auth/hooks/useCalendarFeed'
import { CALENDAR_ADD_TARGET_NAMES } from '@/features/collections/lib/calendarFeedStatus'
import { PanelError, PanelNote, PanelRowButton } from './CalendarPanelParts'
import { CalendarRowNoteLines } from './CalendarRowNoteLines'
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
  actions,
  onNavigate,
}: CalendarStoppedAppProps) {
  const forget = useForgetCalendarClient()
  const { againTarget } = view

  return (
    <div className="flex flex-col gap-2">
      <PanelNote>{view.stoppedLine}</PanelNote>
      {againTarget ? (
        <div className="flex flex-col">
          <PanelRowButton
            onClick={
              againTarget === 'google' ? actions.addToGoogle : actions.addToApple
            }
            icon={ExternalLink}
            handlesLink
          >
            {addAgainLabel(CALENDAR_ADD_TARGET_NAMES[againTarget])}
          </PanelRowButton>
          <CalendarRowNoteLines
            note={actions.notes[againTarget]}
            onNavigate={onNavigate}
          />
        </div>
      ) : null}
      <div className="flex flex-col">
        <PanelRowButton
          onClick={() => forget.mutate(view.app.client)}
          disabled={forget.isPending}
        >
          {PANEL_COPY.removedIt}
        </PanelRowButton>
        {forget.isError ? (
          <PanelError>{PANEL_COPY.forgetFailed}</PanelError>
        ) : null}
      </div>
    </div>
  )
}
