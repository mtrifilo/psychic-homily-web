import type { CalendarLinkResetReason } from '@/features/shows/types'
import type { CalendarAddTarget } from '@/features/collections/lib/calendarFeedStatus'

/**
 * Every string the saved-shows calendar panel shows, verbatim from the
 * owner-approved board spec (docs/research/calendar-subscribe-v4.1-board-spec).
 * Kept in one place so a wording change is one edit and the tests can check
 * the whole set for banned words and characters.
 */
export const PANEL_COPY = {
  headingAdd: 'Add your saved shows to your calendar',
  subAdd: 'Add it once. New saves show up the next time your calendar checks.',
  headingStatus: 'Saved shows calendar',
  headingReset: 'Your calendar link was reset',
  loading: 'Checking your calendar',
  addAnother: 'Add another',

  rowGoogle: 'Google Calendar',
  rowApple: 'Apple Calendar',
  rowOutlookCom: 'Outlook.com',
  rowOutlookWork: 'Outlook (work or school)',
  rowCopy: 'Copy link',

  googlePending: [
    'Link copied, and Google Calendar is open in a new tab.',
    'Paste the link there, then choose Add calendar.',
    "Waiting for Google's first check.",
  ],
  appleOpened: 'Opened Apple Calendar. Waiting for its first check.',
  outlookOpened: 'Opened Outlook. Waiting for its first check.',
  copied: 'Copied. Paste it where your calendar app adds a calendar from a link.',

  iphoneGoogleAppHint:
    'Using the Google Calendar app? Turn on iCloud under Settings > Manage accounts and your shows appear there too.',
  iphoneAppleNotSafari:
    'Open this page in Safari to add it in one tap, or copy the link below.',
  iphoneGoogleSub:
    'To add it to your Google account itself, finish once on a computer.',
  androidGoogleSub:
    "Google's phone app can't add a calendar link. Finish once on a computer.",
  androidPointer:
    'On this phone you can add one show at a time: open a show and choose [Add to calendar].',

  emailSteps: 'Email me the steps',
  sendAgain: 'Send again',
  emailUnverified: 'Verify your email address first, then try again.',
  verifyEmail: 'Verify email',
  emailRateLimited: "You've asked for this a few times. Try again in an hour.",
  emailFailed: "Couldn't send the email. Try again.",

  downloadCaption: 'A one-time copy. It does not update.',
  manageInSettings: 'Manage in Settings',
  removedIt: 'I removed it',
  unrecoverable:
    'This calendar was set up before links could be shown again. To copy the link or add another app, make a new link in Settings. That disconnects the calendar connected now.',

  createFailed: "Couldn't create your calendar link. Try again.",
  loadFailed: "Couldn't load your calendar link. Try again.",
  forgetFailed: "Couldn't remove that. Try again.",
  copyFailed: "Couldn't copy the link. Copy it from Settings instead.",
} as const

export const RESET_SUB: Record<CalendarLinkResetReason, string> = {
  sign_out_everywhere:
    'Signing out everywhere else reset it, so your calendar stopped updating. Add it again, and remove the old calendar in your calendar app.',
  password_reset:
    'Resetting your password reset it, so your calendar stopped updating. Add it again, and remove the old calendar in your calendar app.',
}

/** The download row; never called with zero, where the row is not shown. */
export function downloadLabel(upcomingCount: number): string {
  return upcomingCount === 1
    ? 'Download this show (.ics)'
    : `Download these ${upcomingCount} shows (.ics)`
}

/** The row label of each app with a row of its own. */
export const ADD_TARGET_ROW: Record<CalendarAddTarget, string> = {
  google: PANEL_COPY.rowGoogle,
  apple: PANEL_COPY.rowApple,
}

/** A stopped app's way back, by the plain name of its row. */
export function addAgainLabel(target: CalendarAddTarget): string {
  return `Add to ${ADD_TARGET_ROW[target]} again`
}

export function emailSentLine(email: string): string {
  return `Sent to ${email}. Open it on a computer.`
}
