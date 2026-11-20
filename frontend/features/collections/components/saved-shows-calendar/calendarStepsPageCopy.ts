/**
 * The strings of the page "Email me the steps" opens (/library/calendar),
 * verbatim from the owner-approved board spec, section L1
 * (docs/research/calendar-subscribe-v4.1-board-spec-2026-11.md, an agent-only
 * document kept outside version control; board L1 in Product Designs shows
 * the same strings). Its error lines, the app names and labels of the other
 * calendars, the iPhone Google Calendar app hint and Android's per-show
 * pointer are the panel's own (savedShowsCalendarCopy, lib/calendarFeedStatus),
 * reused where the same thing happens; the page adds no line of its own beyond
 * these.
 */
export const STEPS_PAGE_COPY = {
  heading: 'Add your saved shows to Google Calendar',

  stepCopy: 'Copy your calendar link.',
  copyButton: 'Copy link',
  copiedButton: 'Copied',
  stepOpen: "Open Google Calendar's Add from URL page.",
  openButton: 'Open Google Calendar',
  stepPaste: 'Paste the link there and choose Add calendar.',

  waiting: "Waiting for Google's first check.",
  done: "You're set. New saves appear at its next check.",

  otherCalendarsLead: 'Using a different calendar?',

  phoneNeedsComputer: 'This step needs a computer. Open the email there.',
  iphoneAppleInstead:
    'On this iPhone you can add it through Apple Calendar instead:',
} as const
