/**
 * IP-geolocation default-city resolver (PSY-926 /explore, PSY-946 /shows + home).
 *
 * Why this exists
 * ---------------
 * The city filter (PSY-840 /explore, PSY-932 list pages) needs a sensible
 * default for anonymous / first-visit users who have no `favorite_cities`. A
 * static Phoenix default wrong-foots the traveler the feature targets. We
 * instead derive the visitor's metro from Vercel's edge geo-IP headers and
 * surface it as an *overridable suggestion* — never a hard lock.
 *
 * Two read paths — both through `decodeGeoHeaders` (PSY-946)
 * ----------------------------------------------------------
 * The SAME decode logic (`decodeGeoHeaders`) feeds two transports, chosen per
 * surface by its rendering mode. DO NOT "unify" these into one — the split is
 * deliberate and load-bearing (each preserves a different route's cache mode):
 *
 *   1. `getGeoDefaultCity()` — server `next/headers()` read, used by
 *      `app/explore/page.tsx`. /explore is ALREADY a request-time (dynamic)
 *      render (it `await`s `searchParams`, PSY-840), so reading one more
 *      request-data source there is free: the upstream
 *      `fetch(... { revalidate: 300 })` calls keep their ISR hints, only the
 *      per-request shell varies. Moving /explore to the route-handler path
 *      would ADD a client request it doesn't need.
 *
 *   2. The edge route handler `app/api/geo/route.ts` — calls
 *      `decodeGeoHeaders(request.headers)` and is fetched CLIENT-SIDE by
 *      `useGeoDefaultCity`. Used by `/shows` (ISR, `app/shows/page.tsx`) and
 *      home (`app/page.tsx`, fully static). Those PAGES must NOT read
 *      `next/headers` — doing so opts the whole route into dynamic rendering
 *      and defeats ISR/static (the anti-pattern that got PSY-834 cancelled).
 *      A separate route handler is inherently dynamic without touching the
 *      page's cache mode. ⚠️ If you ever try to make /shows or home read geo
 *      server-side, you will silently un-ISR them — keep the client fetch.
 *
 * Either way the decoded value is a SUGGESTION only — it is reconciled against
 * PH's has-shows data on the client (`useGeoDefaultCity` →
 * `geoCityWithShows`) and only ever pre-selected as the canonical PH city,
 * never the raw header (injection-safe).
 *
 * Privacy
 * -------
 * City + region only, read transiently per request, never stored against an
 * identity and never written to a cookie. The decoded value flows to the
 * client as a render prop (/explore) or a no-store JSON response (route
 * handler) and is discarded after use. Disclosed in the privacy policy
 * (§2.3 / §3).
 *
 * `getGeoDefaultCity` imports `next/headers`, so importing IT from a client
 * component throws at build time — the "server-only" boundary is enforced by
 * the compiler. `decodeGeoHeaders` is pure (takes a `Headers`) and is safe to
 * call from the edge route handler.
 */

import { headers } from 'next/headers'
import type { CityState } from '@/components/filters/CityFilters'

/**
 * A decoded geo suggestion: the visitor's `{city, state}` plus, when Vercel
 * supplies them, their approximate `{latitude, longitude}` (PSY-981).
 *
 * The coords drive the "nearest has-shows city" fallback: when the visitor's
 * exact city has no shows (e.g. Paradise Valley, AZ — a Phoenix suburb), the
 * client picks the geographically nearest has-shows city by haversine
 * (`useGeoDefaultCity`). They are OPTIONAL — some IPs/edge configs omit the
 * lat/long headers — and absence is graceful: the client falls back to exact
 * city-name matching, exactly as it did before PSY-981. The coords are never
 * seeded as a city; only the canonical PH `{city,state}` is (injection-safe).
 */
export interface GeoLocation extends CityState {
  latitude?: number
  longitude?: number
}

/** Vercel edge geo headers (lowercased — Next normalizes header keys). */
const HEADER_CITY = 'x-vercel-ip-city'
const HEADER_REGION = 'x-vercel-ip-country-region'
const HEADER_COUNTRY = 'x-vercel-ip-country'
const HEADER_LATITUDE = 'x-vercel-ip-latitude'
const HEADER_LONGITUDE = 'x-vercel-ip-longitude'

/**
 * Decode a `Headers`-like object carrying the Vercel edge geo headers into a
 * `{ city, state }` suggestion, or `null` when geo is unavailable / ambiguous.
 *
 * Pure (no `next/headers`) so BOTH read paths share it (see file header):
 * `getGeoDefaultCity` passes the server `headers()` store; the edge route
 * handler passes `request.headers`. Anything with a `.get(name) => string |
 * null` shape works.
 *
 * This is a SUGGESTION only — it is NOT validated against PH's shows data
 * here (neither the server nor the edge has cheap access to the
 * cities-with-counts list). The client (`useGeoDefaultCity`) reconciles it
 * against `useShowCities` data and only pre-selects it when the city actually
 * has upcoming shows; otherwise it falls back to "All cities". Keeping the
 * has-shows check on the client reuses PSY-840's existing data source instead
 * of inventing a server lookup.
 *
 * Returns `null` (→ "All cities" fallback) when ANY of these hold:
 *   - the city header is missing or empty (no geo / proxy stripped it),
 *   - the region header is missing or empty (we need a state to match PH's
 *     city,state keying — a bare city is ambiguous),
 *   - the country is present and not US/CA (PH's city data is US/Canada
 *     state/province-keyed; a foreign region code can't match, and surfacing
 *     a foreign city we have no shows for is pointless — the client check
 *     would drop it anyway, so we short-circuit cheaply here).
 *
 * Header values are URL-encoded by Vercel (e.g. "S%C3%A3o%20Paulo"); we
 * decode them. A malformed encoding yields `null` rather than throwing.
 */
export function decodeGeoHeaders(
  headerList: { get(name: string): string | null },
): GeoLocation | null {
  const country = decodeHeader(headerList.get(HEADER_COUNTRY))
  // Vercel returns ISO 3166-1 alpha-2 country codes. PH's city data is keyed
  // by US state / Canadian province codes; only US/CA can match, so reject
  // anything else cheaply. An ABSENT country header (local dev, some edge
  // configs) is allowed through — the client has-shows check is the real gate.
  // The country gate is UNCHANGED by PSY-981's nearest-city work: non-US/CA
  // visitors still get no geo default (PH's show cities are US/CA only).
  if (country !== null && country !== 'US' && country !== 'CA') {
    return null
  }

  const city = decodeHeader(headerList.get(HEADER_CITY))
  const state = decodeHeader(headerList.get(HEADER_REGION))

  // Both halves are required — PH keys cities by (city, state), and the
  // `?cities=` wire format (cityParams.ts) drops any segment missing either
  // half. A bare city with no region is ambiguous → "All cities".
  if (!city || !state) {
    return null
  }

  // Visitor lat/long (PSY-981) for the nearest-has-shows-city fallback. These
  // headers are plain decimal strings (NOT URL-encoded like the city), and are
  // OPTIONAL — `parseCoordinate` returns undefined on a missing/unparseable/
  // out-of-range value, so the city/state suggestion still flows and the client
  // degrades to exact city-name matching. We only attach BOTH coords or
  // NEITHER: a lone latitude is useless for a distance calc and would only
  // invite a half-applied haversine bug downstream.
  const latitude = parseCoordinate(headerList.get(HEADER_LATITUDE), 90)
  const longitude = parseCoordinate(headerList.get(HEADER_LONGITUDE), 180)
  if (latitude !== undefined && longitude !== undefined) {
    return { city, state, latitude, longitude }
  }

  return { city, state }
}

/**
 * Server-side geo read for the /explore page (path 1 above). Reads the Vercel
 * edge geo headers via `next/headers()` and decodes them with
 * `decodeGeoHeaders`. Throws at build time if imported from a client component
 * (compiler-enforced server-only boundary).
 */
export async function getGeoDefaultCity(): Promise<GeoLocation | null> {
  const headerList = await headers()
  return decodeGeoHeaders(headerList)
}

/**
 * Decode a single URL-encoded header value, trimming whitespace. Returns
 * `null` for a missing, empty, or malformed-encoding value so callers can
 * treat "no usable value" uniformly (avoids the truthy-empty-string trap).
 */
function decodeHeader(raw: string | null): string | null {
  if (!raw) return null
  let decoded: string
  try {
    decoded = decodeURIComponent(raw)
  } catch {
    // Malformed percent-encoding — treat as no value rather than throwing.
    return null
  }
  const trimmed = decoded.trim()
  return trimmed === '' ? null : trimmed
}

/**
 * Parse a Vercel lat/long header into a finite number within ±`max` degrees,
 * else `undefined`. Defensive at the trust boundary: the value is a raw HTTP
 * header, so reject anything `Number()` can't turn into a real coordinate —
 * missing, empty, `NaN`/`Infinity` (Number('') is 0, hence the empty-string
 * guard), or out of the valid [-max, max] range. A bad value degrades to "no
 * coords" (exact-match fallback), never a wrong distance calculation.
 */
function parseCoordinate(raw: string | null, max: number): number | undefined {
  if (raw === null) return undefined
  const trimmed = raw.trim()
  if (trimmed === '') return undefined
  const value = Number(trimmed)
  if (!Number.isFinite(value)) return undefined
  if (value < -max || value > max) return undefined
  return value
}
