/**
 * The Settings calendar card's fragment. Kept apart from the card so a page
 * that only links to it (a password reset, the Sessions card) does not pull
 * the card's code in. Not `calendar-feed`, which Library reads as the old
 * address of its calendar panel and opens the panel for.
 */
export const CALENDAR_FEED_SETTINGS_ANCHOR = 'calendar'

/** The Settings calendar card's address, where an "add it again" link lands. */
export const CALENDAR_FEED_SETTINGS_HREF = `/profile?tab=settings#${CALENDAR_FEED_SETTINGS_ANCHOR}`
