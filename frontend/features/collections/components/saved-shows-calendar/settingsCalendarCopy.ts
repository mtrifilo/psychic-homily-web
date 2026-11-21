import type { CalendarLinkResetReason } from '@/features/shows/types'

/**
 * The Settings calendar card's own strings, verbatim from the owner-approved
 * board spec (section S4 and its decisions of 2026-11-20),
 * docs/research/calendar-subscribe-v4.1-board-spec-2026-11.md (an agent-only
 * document kept outside version control). The status, freshness and stopped
 * lines, the row notes and the failure lines the card shares with the panel
 * are the panel's own (savedShowsCalendarCopy, lib/calendarFeedStatus).
 */
export const SETTINGS_CALENDAR_COPY = {
  title: 'Calendar',
  sub: 'Your saved shows in the calendar you already use.',
  noneChecking: 'No calendar is checking your saved shows yet.',
  addToCalendar: 'Add to calendar',

  addGoogle: 'Add to Google Calendar',
  addApple: 'Add to Apple Calendar',
  outlookCom: 'Outlook.com',
  outlookWork: 'Outlook (work or school)',

  calendarLinkLabel: 'Saved shows calendar',
  followsLinkLabel: 'Follows feed',
  followsLinkDetail:
    '(new shows and releases from artists you follow, for a feed reader)',
  copy: 'Copy',
  newLink: 'New link',
  turnOff: 'Turn off',
  note: 'Anyone with these links can see your saved shows and who you follow. One key makes both, so a new link changes both.',

  confirmNewLink:
    'A new link disconnects every calendar and feed reader using the current one. You add them again with the new link.',
  confirmNewLinkAction: 'Make a new link',
  confirmTurnOff:
    'Turning this off disconnects every calendar and feed reader using your links.',
  confirmTurnOffAction: 'Turn off',
  cancel: 'Cancel',

  oldLinkConnected:
    'This calendar was set up before links could be shown again. To copy the link or add another app, make a new link. That disconnects the calendar connected now.',
  oldLinkNoApp:
    'This calendar was set up before links could be shown again. To copy the link or add an app, make a new link.',

  turnOffFailed: "Couldn't turn this off. Try again.",
  copyFailed: "Couldn't copy the link. Select it and copy it by hand.",
  followsCopied: 'Copied. Paste it into your feed reader.',
} as const

/** The status sentence after a security action reset the link. */
export const SETTINGS_RESET_SENTENCE: Record<CalendarLinkResetReason, string> = {
  sign_out_everywhere:
    'Your calendar link was reset when you signed out everywhere else. Calendars that used it stopped updating. Add it again, and remove the old calendar in your calendar app.',
  password_reset:
    'Your calendar link was reset when you reset your password. Calendars that used it stopped updating. Add it again, and remove the old calendar in your calendar app.',
}
