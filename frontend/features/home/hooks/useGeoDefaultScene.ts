'use client'

import { useEffect, useRef, useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import type { GeoLocation } from '@/lib/geo-default'
import { GEO_CACHE_KEY, toGeoLocation } from '@/lib/geo-client'

/**
 * Client-side geo suggestion for the homepage graph's default scene (PSY-1346).
 *
 * Reuses the SAME `/api/geo` edge route + sessionStorage cache as the shows
 * city filter's `useGeoDefaultCity` (via the shared `@/lib/geo-client`
 * primitives), so a homepage visit makes at most one geo request no matter how
 * many geo consumers mount. Returns the raw `{city,state}`(+coords) suggestion
 * plus a `resolved` flag; `pickDefaultScene` reconciles the suggestion to an
 * actual scene.
 *
 * Unlike `useGeoDefaultCity` this is deliberately NOT auth/favorites-gated: the
 * graph has no per-user "favorite scene", so geo is an overridable default for
 * every visitor. The component owns precedence — a "Surprise me" pick (or any
 * scene the visitor has already selected) always wins over this suggestion.
 *
 * The `resolved` flag lets the section hold its loading skeleton until geo
 * settles, so it picks (and fetches the graph-card for) the geo scene ONCE
 * rather than rendering the liveliest scene first and then swapping to the
 * visitor's scene — which would both flash and waste a heavy graph fetch. A
 * warm session cache settles synchronously in the initializer (no skeleton
 * beat); a cold cache settles when `/api/geo` resolves or the timeout fires.
 */

// Mirror AtlasGlobe's GEO_TIMEOUT_MS: never strand the skeleton on a slow or
// header-less edge — settle to "no geo" (the liveliest default) after this.
const GEO_TIMEOUT_MS = 700

export interface GeoDefaultScene {
  /** The visitor's geo suggestion, or null when unavailable / not yet known. */
  suggestion: GeoLocation | null
  /** True once geo has settled (warm cache, fetch resolved, or timeout). */
  resolved: boolean
}

export function useGeoDefaultScene(): GeoDefaultScene {
  const [state, setState] = useState<GeoDefaultScene>(readInitialGeo)
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    // Warm cache already settled synchronously in the initializer.
    if (state.resolved) return

    let done = false
    const finish = (suggestion: GeoLocation | null) => {
      if (done) return
      done = true
      setState({ suggestion, resolved: true })
    }
    // Bound the wait so a slow / header-less edge can't hold the skeleton open.
    const timer = setTimeout(() => finish(null), GEO_TIMEOUT_MS)

    fetch('/api/geo')
      .then(res => (res.ok ? (res.json() as Promise<{ geo?: unknown }>) : null))
      .then(body => {
        const value = toGeoLocation(body?.geo)
        // Cache even if we already timed out, so the NEXT visit resolves warm
        // (and synchronously) with the real value.
        try {
          window.sessionStorage.setItem(
            GEO_CACHE_KEY,
            JSON.stringify({ geo: value }),
          )
        } catch {
          // sessionStorage unavailable (private mode / quota) — degrade to a
          // re-fetch next visit; the default still works this time.
        }
        finish(value)
      })
      .catch(error => {
        // A geo-default miss is non-critical (the section keeps its liveliest
        // default), but capture it so a broken edge route is visible.
        Sentry.captureException(error, {
          level: 'warning',
          tags: { service: 'geo-default-scene' },
        })
        finish(null)
      })

    return () => {
      done = true
      clearTimeout(timer)
    }
  }, [state.resolved])

  return state
}

/**
 * Initial state from the shared `/api/geo` sessionStorage cache. A cache HIT
 * (even one that cached "no geo") settles immediately — the answer is already
 * known, so don't hold the skeleton. A miss starts unresolved and the effect
 * fetches. Read synchronously (not in an effect) so a warm cache lands the geo
 * scene on the section's FIRST render; safe because the section is client-only
 * lazy-mounted (never SSR'd), so `window` exists and there's no hydration to
 * mismatch.
 */
function readInitialGeo(): GeoDefaultScene {
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
