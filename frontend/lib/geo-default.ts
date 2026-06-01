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

/** Vercel edge geo headers (lowercased — Next normalizes header keys). */
const HEADER_CITY = 'x-vercel-ip-city'
const HEADER_REGION = 'x-vercel-ip-country-region'
const HEADER_COUNTRY = 'x-vercel-ip-country'

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
): CityState | null {
  const country = decodeHeader(headerList.get(HEADER_COUNTRY))
  // Vercel returns ISO 3166-1 alpha-2 country codes. PH's city data is keyed
  // by US state / Canadian province codes; only US/CA can match, so reject
  // anything else cheaply. An ABSENT country header (local dev, some edge
  // configs) is allowed through — the client has-shows check is the real gate.
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

  return { city, state }
}

/**
 * Server-side geo read for the /explore page (path 1 above). Reads the Vercel
 * edge geo headers via `next/headers()` and decodes them with
 * `decodeGeoHeaders`. Throws at build time if imported from a client component
 * (compiler-enforced server-only boundary).
 */
export async function getGeoDefaultCity(): Promise<CityState | null> {
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
