'use client'

import { useEffect, useMemo, useState } from 'react'
import {
  AUTH_ERROR_CODE_PARAM,
  AUTH_ERROR_PARAM,
  AUTH_REASON_PARAM,
  isReauthReason,
} from '@/lib/auth-href'

/**
 * `query` without the parameters the auth page renders a banner from: the
 * OAuth refusal's message (`error`) and code (`error_code`), and a `reason`
 * that is not a re-authentication (`ACCOUNT_INACTIVE` is the one the backend
 * writes). A re-authentication reason stays, because it decides what the page
 * is (one tab, and no redirect for a signed-in reader) rather than what it
 * says, so a reload has to keep it. `returnTo`, `intent` and every parameter
 * the page does not read stay too.
 *
 * Takes and returns a query string without its leading `?`, serialized by
 * URLSearchParams, as `useSearchParams().toString()` is.
 */
export function withoutBannerParams(query: string): string {
  const params = new URLSearchParams(query)
  params.delete(AUTH_ERROR_PARAM)
  params.delete(AUTH_ERROR_CODE_PARAM)
  if (!isReauthReason(params.get(AUTH_REASON_PARAM))) {
    params.delete(AUTH_REASON_PARAM)
  }
  return params.toString()
}

/** An arrival that carried banner parameters, and its URL without them. */
type BannerArrival = { query: string; strippedQuery: string }

function bannerArrivalOf(query: string): BannerArrival | null {
  const strippedQuery = withoutBannerParams(query)
  return strippedQuery === query ? null : { query, strippedQuery }
}

/**
 * The auth page's search params as they arrived, with the banner parameters
 * taken out of the address bar after the first render.
 *
 * The banners are held here, in memory, for as long as the address bar shows
 * the URL this hook wrote, so the page renders exactly what it rendered on
 * arrival. A reload, or a return to that history entry, starts from the
 * written URL and shows no banner. Any other query string is a new arrival:
 * one with banner parameters is held in turn, one without drops what was
 * held, so a later return to the written URL does not bring a banner back.
 *
 * `live` is `useSearchParams()`. The URL is rewritten with
 * `window.history.replaceState`, which the App Router patches to sync
 * `useSearchParams` with the new URL. The state argument is `null` on
 * purpose: the patch copies the router's own history state into the entry,
 * and skips the sync for state that already carries it.
 */
export function useAuthArrivalParams(live: {
  toString(): string
}): URLSearchParams {
  const liveQuery = live.toString()
  const liveArrival = useMemo(() => bannerArrivalOf(liveQuery), [liveQuery])
  const [held, setHeld] = useState(liveArrival)
  const arrival =
    held !== null &&
    (liveQuery === held.query || liveQuery === held.strippedQuery)
      ? held
      : liveArrival
  if (arrival !== held) {
    setHeld(arrival)
  }

  const strippedQuery = arrival?.strippedQuery
  useEffect(() => {
    if (strippedQuery === undefined || liveQuery === strippedQuery) return
    const search = strippedQuery === '' ? '' : `?${strippedQuery}`
    window.history.replaceState(
      null,
      '',
      `${window.location.pathname}${search}${window.location.hash}`
    )
  }, [liveQuery, strippedQuery])

  const query = arrival?.query ?? liveQuery
  return useMemo(() => new URLSearchParams(query), [query])
}
