/**
 * IP-geolocation default-city resolver for /explore (PSY-926).
 *
 * Why this exists
 * ---------------
 * The /explore Upcoming Shows city filter (PSY-840) needs a sensible default
 * for anonymous / first-visit users who have no `favorite_cities`. A static
 * Phoenix default wrong-foots the traveler the feature targets. We instead
 * derive the visitor's metro from Vercel's edge geo-IP headers and surface it
 * as an *overridable suggestion* — never a hard lock.
 *
 * Dynamic-boundary contract
 * --------------------------
 * Vercel injects `X-Vercel-IP-City` / `X-Vercel-IP-Country-Region` /
 * `X-Vercel-IP-Country` onto the incoming request at the edge. This module is
 * read from `app/explore/page.tsx`, which is ALREADY a request-time (dynamic)
 * render — it `await`s `searchParams` (PSY-840's URL persistence). Reading one
 * more request-data source (`headers()`) there does not un-cache the upstream
 * `fetch(... { next: { revalidate: 300 } })` calls, so ISR on the /explore
 * *data* is preserved; only the per-request shell varies. This is the
 * "dynamic boundary" the PSY-926 AC requires, and it avoids touching
 * `frontend/proxy.ts`'s carefully-tuned soft-404 matcher (the proxy is not
 * needed because the page is already dynamic).
 *
 * Privacy
 * -------
 * City + region only, read transiently per request, never stored against an
 * identity and never written to a cookie. The decoded value flows to the
 * client as a render prop and is discarded after render. Disclosed in the
 * privacy policy (§2.3 / §3).
 *
 * Importing this from a client component throws at build time (it reads
 * `next/headers`), so the "server-only" boundary is enforced by the compiler.
 */

import { headers } from 'next/headers'
import type { CityState } from '@/components/filters/CityFilters'

/** Vercel edge geo headers (lowercased — Next normalizes header keys). */
const HEADER_CITY = 'x-vercel-ip-city'
const HEADER_REGION = 'x-vercel-ip-country-region'
const HEADER_COUNTRY = 'x-vercel-ip-country'

/**
 * Read the Vercel edge geo headers and decode them into a `{ city, state }`
 * suggestion, or `null` when geo is unavailable / ambiguous.
 *
 * This is a SUGGESTION only — it is NOT validated against PH's shows data
 * here (the proxy/server has no cheap access to the cities-with-counts list).
 * The client (`UpcomingShowsList`) reconciles it against `useShowCities`
 * data and only pre-selects it when the city actually has upcoming shows;
 * otherwise it falls back to "All cities". Keeping the has-shows check on the
 * client reuses PSY-840's existing data source instead of inventing a server
 * lookup.
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
export async function getGeoDefaultCity(): Promise<CityState | null> {
  const headerList = await headers()

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
