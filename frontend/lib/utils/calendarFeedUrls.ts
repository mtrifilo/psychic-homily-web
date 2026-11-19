/**
 * URL builders for handing calendar data to a calendar client — live ICS
 * feeds (subscriptions) and one-shot single-event exports. Every calendar
 * surface goes through these so the third-party contracts live in one place.
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

/**
 * Google Calendar's "Add from URL" settings page, where a person pastes a
 * private calendar link and chooses Add calendar. The private feed goes
 * through this page rather than the one-click `cid` link above, which refused
 * an https address in the owner's device check.
 */
export const GOOGLE_CALENDAR_ADD_BY_URL_PAGE =
  'https://calendar.google.com/calendar/r/settings/addbyurl'

/** The name a calendar app gives the saved-shows calendar when it asks for one. */
const SAVED_SHOWS_CALENDAR_NAME = 'Psychic Homily saved shows'

/**
 * Microsoft's "Subscribe from web" page with the address filled in. The url
 * value is the webcal form, the one the owner's device check confirmed on
 * both Outlook.com and Microsoft 365.
 */
function outlookAddFromWebUrl(origin: string, feedUrl: string): string {
  const params = new URLSearchParams({
    url: webcalUrl(feedUrl),
    name: SAVED_SHOWS_CALENDAR_NAME,
  })
  return `${origin}/calendar/0/addfromweb/?${params.toString()}`
}

/** Outlook.com (personal Microsoft accounts). */
export function outlookComSubscribeUrl(feedUrl: string): string {
  return outlookAddFromWebUrl('https://outlook.live.com', feedUrl)
}

/** Outlook for work or school (Microsoft 365). */
export function outlookWorkSubscribeUrl(feedUrl: string): string {
  return outlookAddFromWebUrl('https://outlook.office.com', feedUrl)
}

/** Formats an instant as the compact UTC form Google's template URL expects. */
function googleUtcStamp(date: Date): string {
  return date.toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '')
}

export interface GoogleCalendarEvent {
  title: string
  /** Absolute start instant. */
  start: Date
  /** Absolute end instant. */
  end: Date
  details?: string
  location?: string
}

/**
 * Google Calendar's single-event template URL (`action=TEMPLATE`) — the
 * per-event counterpart of googleCalendarSubscribeUrl.
 *
 * Times are passed in UTC (`...Z/...Z`) rather than as floating local times
 * with a `ctz` parameter: the event is an absolute instant, UTC states it
 * exactly, and Google renders it in the viewer's calendar timezone — which
 * removes any need for this client to know the venue's IANA zone.
 */
export function googleCalendarEventUrl(event: GoogleCalendarEvent): string {
  const params = new URLSearchParams({
    action: 'TEMPLATE',
    text: event.title,
    dates: `${googleUtcStamp(event.start)}/${googleUtcStamp(event.end)}`,
  })
  if (event.details) params.set('details', event.details)
  if (event.location) params.set('location', event.location)
  return `https://calendar.google.com/calendar/render?${params.toString()}`
}
