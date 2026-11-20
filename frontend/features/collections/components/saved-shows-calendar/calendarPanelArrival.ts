/**
 * The fragment that takes a person to the saved-shows calendar link on
 * Library: the address the Library calendar card had, kept so links and
 * bookmarks made for the card still reach the calendar. Arriving focuses the
 * link; it never opens the panel.
 */
export const CALENDAR_ARRIVAL_HASH = '#calendar-feed'

const CALENDAR_ARRIVAL_PATH = '/library'

/**
 * Loads Library afresh at the calendar fragment, landing on the calendar link
 * with focus, for the person to click. A full page load, not a client
 * navigation: it is the way back to the panel when this page's copy of its
 * code failed to load, and a fresh load fetches the code again. Not while the
 * browser reports itself offline: the load would replace the page the person
 * is reading with an error page, so the open is refused in place and the next
 * click, once online, tries again.
 */
export function reloadIntoCalendarPanel(): void {
  if (!navigator.onLine) return
  const target = CALENDAR_ARRIVAL_PATH + CALENDAR_ARRIVAL_HASH
  if (window.location.pathname === CALENDAR_ARRIVAL_PATH) {
    // Assigning a URL that differs only by fragment would not reload.
    window.history.replaceState(null, '', target)
    window.location.reload()
    return
  }
  window.location.assign(target)
}
