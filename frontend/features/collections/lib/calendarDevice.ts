import { useSyncExternalStore } from 'react'
import type { CalendarAddTarget } from './calendarFeedStatus'

/**
 * Which add rows a device gets. Read from the user agent, so it is known only
 * in the browser: the server renders no rows, and a surface waits for the
 * device before drawing any.
 *
 * - `iphone`: an iPhone, or an iPad that says so. Apple Calendar adds the
 *   link in one tap, in Safari only; Google needs a computer.
 * - `android`: Google needs a computer, and there is no Apple Calendar.
 * - `mac`: a computer that has Apple Calendar. An iPad that reports a desktop
 *   user agent lands here too, which suits it: every computer row works there.
 * - `computer`: any other computer.
 */
export type CalendarDeviceKind = 'iphone' | 'android' | 'mac' | 'computer'

export interface CalendarDevice {
  kind: CalendarDeviceKind
  /**
   * Safari itself, where a tapped webcal link opens Apple Calendar. Other
   * iPhone browsers and in-app browsers have not been shown to hand it over.
   * Meaningful only for `iphone`.
   */
  isSafari: boolean
}

// Browsers and in-app web views on iOS that put "Safari" in their user agent
// without being Safari.
const IOS_NOT_SAFARI =
  /CriOS|FxiOS|EdgiOS|OPiOS|OPT\/|GSA\/|YaBrowser|DuckDuckGo|FBAN|FBAV|Instagram|LinkedInApp|Twitter|Line\/|Snapchat|Pinterest/

export function detectCalendarDevice(userAgent: string): CalendarDevice {
  if (/iPhone|iPad|iPod/.test(userAgent)) {
    return {
      kind: 'iphone',
      isSafari: /Safari\//.test(userAgent) && !IOS_NOT_SAFARI.test(userAgent),
    }
  }
  if (/Android/.test(userAgent)) return { kind: 'android', isSafari: false }
  if (/Macintosh|Mac OS X/.test(userAgent)) {
    return { kind: 'mac', isSafari: false }
  }
  return { kind: 'computer', isSafari: false }
}

/** Whether the device has a computer's row list. */
export function isCalendarComputer(device: CalendarDevice): boolean {
  return device.kind === 'mac' || device.kind === 'computer'
}

/**
 * Whether this device can add `target` from the panel in one step: the
 * condition for offering "Add to ... again" on a stopped app. Google on a
 * phone cannot (Google's phone app cannot add a calendar from a link); Apple
 * needs Apple Calendar, and on an iPhone, Safari.
 */
export function canAddOnDevice(
  target: CalendarAddTarget,
  device: CalendarDevice
): boolean {
  if (target === 'google') return isCalendarComputer(device)
  if (device.kind === 'mac') return true
  return device.kind === 'iphone' && device.isSafari
}

const subscribeNever = () => () => {}
let cachedUserAgent: string | null = null
let cachedDevice: CalendarDevice | null = null

function deviceSnapshot(): CalendarDevice {
  // Cached by user agent so the snapshot is referentially stable, as
  // useSyncExternalStore requires.
  if (cachedDevice === null || cachedUserAgent !== navigator.userAgent) {
    cachedUserAgent = navigator.userAgent
    cachedDevice = detectCalendarDevice(cachedUserAgent)
  }
  return cachedDevice
}

/** The browser's device; null on the server and during hydration. */
export function useCalendarDevice(): CalendarDevice | null {
  return useSyncExternalStore(subscribeNever, deviceSnapshot, () => null)
}
