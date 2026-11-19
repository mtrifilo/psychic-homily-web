import {
  CALENDAR_CLIENT_ADD_TARGET,
  calendarCheckedLine,
  calendarResetReason,
  calendarStoppedLine,
  classifyCalendarApps,
  type CalendarAddTarget,
  type CalendarApp,
} from '@/features/collections/lib/calendarFeedStatus'
import {
  canAddOnDevice,
  type CalendarDevice,
} from '@/features/collections/lib/calendarDevice'
import type {
  CalendarFeedStatusResponse,
  CalendarLinkResetReason,
} from '@/features/shows/types'

/** An add row. On a phone `google` is the Google block with the email action. */
export type CalendarRow =
  | 'google'
  | 'apple'
  | 'outlook_com'
  | 'outlook_work'
  | 'copy'

export interface ConnectedAppView {
  app: CalendarApp
  checkedLine: string
}

export interface StoppedAppView {
  app: CalendarApp
  stoppedLine: string
  /** The app "Add to ... again" re-adds; null when this device cannot add it. */
  againTarget: CalendarAddTarget | null
}

export interface SavedShowsCalendarModel {
  /** Whether any app has been seen checking the link. */
  heading: 'add' | 'status'
  /** Set when a security action reset the link; its heading wins. */
  resetReason: CalendarLinkResetReason | null
  connected: ConnectedAppView[]
  stopped: StoppedAppView[]
  /**
   * The link exists but its address cannot be shown again: no row can act, so
   * none is drawn, and the panel points to Settings.
   */
  unrecoverable: boolean
  showAddAnother: boolean
  rows: CalendarRow[]
  /** Android's pointer to the per-show calendar link. */
  showPerShowPointer: boolean
  /** Upcoming saved shows in the download; 0 hides the row. */
  downloadCount: number
  showManageInSettings: boolean
}

export interface SavedShowsCalendarInputs {
  status: CalendarFeedStatusResponse
  /** A link exists whose address cannot be shown, so no row can act. */
  unrecoverable: boolean
  /**
   * The reset marker as it stood before this open created a link, which
   * clears it on the server.
   */
  resetReasonBeforeCreate: CalendarLinkResetReason | null
  newestSavedAt: string | null
  upcomingCount: number
  device: CalendarDevice
  now: Date
}

function deviceRows(device: CalendarDevice): CalendarRow[] {
  switch (device.kind) {
    case 'mac':
      return ['google', 'apple', 'outlook_com', 'outlook_work', 'copy']
    case 'computer':
      return ['google', 'outlook_com', 'outlook_work', 'copy']
    case 'iphone':
      return ['apple', 'google', 'copy']
    case 'android':
      return ['google', 'copy']
  }
}

/**
 * Everything the panel shows, from the status and the device. The rules:
 *
 * - A connected app is a status block and never an add row.
 * - A stopped app gets "Add to ... again" in place of its row when this device
 *   can add it; otherwise its row (if this device has one) stays as it is,
 *   since that row is then the way to add it again. One such link per app.
 * - The reset heading shows only while the status carries the reset marker.
 */
export function buildSavedShowsCalendarModel({
  status,
  unrecoverable,
  resetReasonBeforeCreate,
  newestSavedAt,
  upcomingCount,
  device,
  now,
}: SavedShowsCalendarInputs): SavedShowsCalendarModel {
  const apps = classifyCalendarApps(status, now)

  const connected = apps.connected.map(app => ({
    app,
    checkedLine: calendarCheckedLine(app.lastReadAt, newestSavedAt, now),
  }))

  const connectedTargets = new Set<CalendarAddTarget>()
  for (const { client } of apps.connected) {
    const target = CALENDAR_CLIENT_ADD_TARGET[client]
    if (target) connectedTargets.add(target)
  }

  // At most one "again" link per app, on its most recent stopped block (both
  // Apple families add through one Apple row), and none while that app is
  // connected through the other family.
  const againTargets = new Set<CalendarAddTarget>()
  const stopped = apps.stopped.map(app => {
    const target = CALENDAR_CLIENT_ADD_TARGET[app.client]
    const canAddAgain =
      !unrecoverable &&
      target !== null &&
      !connectedTargets.has(target) &&
      !againTargets.has(target) &&
      canAddOnDevice(target, device)
    if (canAddAgain) againTargets.add(target)
    return {
      app,
      stoppedLine: calendarStoppedLine(app, now),
      againTarget: canAddAgain ? target : null,
    }
  })

  const hiddenTargets = new Set([...connectedTargets, ...againTargets])
  const rows = unrecoverable
    ? []
    : deviceRows(device).filter(
        row => !((row === 'google' || row === 'apple') && hiddenTargets.has(row))
      )

  const known = connected.length + stopped.length > 0

  return {
    heading: known ? 'status' : 'add',
    resetReason: calendarResetReason(status) ?? resetReasonBeforeCreate,
    connected,
    stopped,
    unrecoverable,
    showAddAnother:
      connected.length > 0 && stopped.length === 0 && rows.length > 0,
    rows,
    showPerShowPointer: !unrecoverable && device.kind === 'android',
    downloadCount:
      unrecoverable || device.kind === 'android' ? 0 : upcomingCount,
    showManageInSettings: known || unrecoverable,
  }
}
