import { API_BASE_URL } from '@/lib/api-base'

/**
 * Where a radio station is served, and the one read that decides it.
 *
 * A sub-stream (a non-flagship station in a network) lives at
 * `/radio/{network-slug}/channel/{local-slug}`; a flagship or network-less
 * station lives at `/radio/{slug}`. A sub-stream requested at its one-segment
 * URL redirects to its channel URL.
 *
 * Imported by `proxy.ts`, so this module must stay free of `features/` imports
 * and of anything that renders.
 */

/**
 * The data-cache window for the station read, shared by the page and the
 * proxy. Next keys a cached fetch on its URL and init but not on this window,
 * so both callers read and refresh ONE entry only while they also build the
 * same URL with the same init, which is why both come from here.
 */
export const RADIO_STATION_REVALIDATE_SECONDS = 3600

/** The backend read for one station, by slug or numeric id. */
export function radioStationApiUrl(slug: string): string {
  return `${API_BASE_URL}/radio-stations/${slug}`
}

/** The station's network membership, as the station read returns it. */
export interface RadioStationNetwork {
  slug: string
  is_flagship: boolean
}

/**
 * The channel path a request for `/radio/{requestedSlug}` redirects to, or
 * null when the station is served at the one-segment URL.
 *
 * The local slug is the requested slug with the `{network-slug}-` prefix
 * stripped; a slug without the prefix is used whole rather than producing a
 * malformed URL.
 */
export function radioSubStreamRedirectPath(
  requestedSlug: string,
  network: RadioStationNetwork | null | undefined
): string | null {
  if (!network || network.is_flagship) return null
  const prefix = `${network.slug}-`
  const localSlug = requestedSlug.startsWith(prefix)
    ? requestedSlug.slice(prefix.length)
    : requestedSlug
  return `/radio/${network.slug}/channel/${localSlug}`
}
