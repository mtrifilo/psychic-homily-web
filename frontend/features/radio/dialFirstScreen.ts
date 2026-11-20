import { radioEndpoints, radioQueryKeys } from './api'

/**
 * The exact request `useRadioStations` issues for the dial on `/radio`, and
 * the cache key that request lands on.
 *
 * `app/radio/page.tsx` fetches the URL server-side and seeds the key, so every
 * dial strip, with its `/radio/<slug>` link, is in the server HTML. Each
 * strip's detail and now-playing reads stay in the browser. The two only work
 * as a pair: a seeded key the hook does not ask for leaves the dial on its
 * loader, so `app/radio/page.test.tsx` renders the real dial over the seed.
 *
 * The endpoint takes no query parameters and is unpaginated, so this is every
 * station; the hub filters and orders it for the dial.
 *
 * No directive: the server page imports this module, and a value from a
 * `'use client'` module reaches a server module as a client reference rather
 * than as the value.
 */
export const RADIO_DIAL_FIRST_SCREEN_URL = radioEndpoints.STATIONS

export const RADIO_DIAL_FIRST_SCREEN_KEY = radioQueryKeys.stations()
