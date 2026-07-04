'use client'

import { useEffect, useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import type { GeoLocation } from '@/lib/geo-default'
import { GEO_CACHE_KEY, toGeoLocation } from '@/lib/geo-client'

/**
 * Client-side geo suggestion for the homepage graph's default scene (PSY-1346).
 *
 * Reuses the SAME `/api/geo` edge route + sessionStorage cache as the shows
 * city filter's `useGeoDefaultCity` (via the shared `@/lib/geo-client`
 * primitives), so once one consumer has warmed the cache this session, the
 * other reads it instead of re-fetching. (It is NOT a hard single-flight
 * guarantee: on a cold cache both consumers can fire `/api/geo` before either
 * write lands — one extra idempotent edge hit, not a correctness issue.)
 *
 * Non-blocking, exactly like `useGeoDefaultCity`: it returns `null` until geo
 * arrives, so the section renders its liveliest-scene default immediately and
 * swaps to the visitor's scene when the suggestion resolves (via the section's
 * existing scene-rotation path). A warm session cache resolves synchronously in
 * the initializer, so the common case shows the geo scene on the first render
 * with no swap. Reading the cache in the initializer (not an effect) is safe
 * because the section is client-only lazy-mounted (never SSR'd), so `window`
 * exists and there's no hydration to mismatch.
 *
 * Deliberately NOT auth/favorites-gated (unlike `useGeoDefaultCity`): the graph
 * has no per-user "favorite scene", so geo is an overridable default for every
 * visitor. Precedence is the component's job — a "Surprise me" pick (or any
 * scene the visitor already selected) always wins over this suggestion.
 */
export function useGeoDefaultScene(): GeoLocation | null {
  // `resolved` is internal fetch-control only (not returned): it stops the
  // effect from re-fetching once we have an answer, and — because it lives in
  // state, not a ref — it survives React StrictMode's dev remount correctly
  // (a persistent ref would let the first mount's cleanup neutralize its fetch
  // while blocking the second mount's, stranding geo at null in dev).
  const [state, setState] = useState<{
    suggestion: GeoLocation | null
    resolved: boolean
  }>(readInitialGeo)

  useEffect(() => {
    if (state.resolved) return
    let cancelled = false
    fetch('/api/geo')
      .then(res => (res.ok ? (res.json() as Promise<{ geo?: unknown }>) : null))
      .then(body => {
        if (cancelled) return
        const suggestion = toGeoLocation(body?.geo)
        setState({ suggestion, resolved: true })
        try {
          window.sessionStorage.setItem(
            GEO_CACHE_KEY,
            JSON.stringify({ geo: suggestion }),
          )
        } catch {
          // sessionStorage unavailable (private mode / quota) — degrade to a
          // re-fetch next visit; the default still works this time.
        }
      })
      .catch(error => {
        if (cancelled) return
        // A geo-default miss is non-critical (the section keeps its liveliest
        // default), but capture it so a broken edge route is visible.
        Sentry.captureException(error, {
          level: 'warning',
          tags: { service: 'geo-default-scene' },
        })
        setState({ suggestion: null, resolved: true })
      })

    return () => {
      cancelled = true
    }
  }, [state.resolved])

  return state.suggestion
}

/**
 * Initial state from the shared `/api/geo` sessionStorage cache. A cache HIT
 * (even one that cached "no geo") is `resolved` so the effect won't re-fetch; a
 * miss starts unresolved and the effect fetches. Read synchronously (not in an
 * effect) so a warm cache lands the geo scene on the section's FIRST render.
 */
function readInitialGeo(): { suggestion: GeoLocation | null; resolved: boolean } {
  if (typeof window === 'undefined') return { suggestion: null, resolved: false }
  try {
    const cached = window.sessionStorage.getItem(GEO_CACHE_KEY)
    if (cached === null) return { suggestion: null, resolved: false }
    const parsed = JSON.parse(cached) as { geo?: unknown }
    return { suggestion: toGeoLocation(parsed?.geo), resolved: true }
  } catch {
    // Corrupted cache / unavailable — treat as a miss; the effect will fetch.
    return { suggestion: null, resolved: false }
  }
}
