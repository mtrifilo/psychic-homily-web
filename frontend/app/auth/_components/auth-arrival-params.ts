'use client'

import { useEffect, useMemo, useState } from 'react'
import { usePathname } from 'next/navigation'
import {
  AUTH_ERROR_CODE_PARAM,
  AUTH_ERROR_PARAM,
  AUTH_PATH,
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
 * arrival. A reload starts from the written URL and shows no banner. Any
 * other query string is a new arrival: one with banner parameters is held in
 * turn, one without drops what was held, so a later return to the written
 * URL does not bring a banner back.
 *
 * The App Router keeps a page it navigated away from mounted but hidden, and
 * a hidden page re-renders under the current route's query string. So hiding
 * the page drops the hold, and only a query string read on the auth page
 * itself is held: a page shown again neither keeps its old banner nor adopts
 * another route's parameters as one.
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
  const onAuthPage = usePathname() === AUTH_PATH
  const liveArrival = useMemo(
    () => (onAuthPage ? bannerArrivalOf(liveQuery) : null),
    [onAuthPage, liveQuery]
  )
  const [held, setHeld] = useState(liveArrival)
  const arrival =
    held !== null &&
    (liveQuery === held.query || liveQuery === held.strippedQuery)
      ? held
      : liveArrival
  if (arrival !== held) {
    setHeld(arrival)
  }

  // A hidden page runs its effect cleanups, so this is the page's "hidden"
  // signal.
  useEffect(() => () => setHeld(null), [])

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
