/**
 * URL builders for handing an ICS feed to a calendar client. Every feed
 * surface (venue feeds, the personal saved-shows feed) goes through these so
 * the third-party subscribe contracts live in exactly one place.
 */

/** webcal:// is what hands a feed URL to the OS-registered calendar app. */
export function webcalUrl(feedUrl: string): string {
  return feedUrl.replace(/^https?:\/\//, 'webcal://')
}

/** Google Calendar subscribes by URL, but only accepts the webcal scheme here. */
export function googleCalendarSubscribeUrl(feedUrl: string): string {
  return `https://calendar.google.com/calendar/r?cid=${encodeURIComponent(
    webcalUrl(feedUrl)
  )}`
}
