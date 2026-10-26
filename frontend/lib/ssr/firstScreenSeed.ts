import type { DehydratedState } from '@tanstack/react-query'
import { seedFirstScreen } from '@/lib/query-hydration'
import {
  fetchListPayload,
  type ListPayloadCollection,
} from '@/lib/ssr/fetchListPayload'

/** A route's `searchParams`, as Next hands them to a page. */
export type RouteSearchParams = Record<string, string | string[] | undefined>

/** What `dehydrate()` produces for an empty client: hydrates nothing. */
export const EMPTY_DEHYDRATED_STATE: DehydratedState = {
  mutations: [],
  queries: [],
}

/**
 * Whether a URL's params keep it on a list's seeded first screen: true when
 * none of `defeatingParams` carries a non-empty value.
 *
 * The test is for a non-empty value, not for a value that moves the client
 * hook's cache key, so a param restating the default (`?page=1`) still skips a
 * seed it would have hit. That is the safe direction: a false positive costs a
 * server-rendered loader, while a false negative would seed a key the hook
 * never reads.
 *
 * Empty values (`?tags=`, `?page=`) read as absent, because the list
 * components parse them as absent. A repeated param is an array; any non-empty
 * member counts.
 */
export function isFirstScreenRequest(
  params: RouteSearchParams,
  defeatingParams: readonly string[],
): boolean {
  return !defeatingParams.some(key => {
    const value = params[key]
    if (Array.isArray(value)) return value.some(v => v !== '')
    return value !== undefined && value !== ''
  })
}

/** One cache entry a list blocks its first paint on, and the read that fills it. */
export interface FirstScreenRead {
  /** Absolute API URL of the request the client hook issues. */
  url: string
  /** The key the client hook registers for that request. */
  queryKey: readonly unknown[]
  /** The response key holding the row array (see `fetchListPayload`). */
  collection: ListPayloadCollection
  /** Sentry `service` tag for a failed read. */
  service: string
}

/**
 * The dehydrated first screen of a list, or an empty state when there is none
 * to give.
 *
 * All or nothing: when any read returns `null`, nothing is seeded and the
 * client fetches every entry itself, so the server HTML is either the whole
 * first screen or the list's own loader, never a mixture of seeded rows and
 * placeholder facets.
 *
 * `seedFirstScreen` is reached only on the path that has something to seed, so
 * a URL off the first screen does not pay for its `connection()`.
 */
export async function dehydrateListFirstScreen(
  params: RouteSearchParams,
  defeatingParams: readonly string[],
  reads: readonly FirstScreenRead[],
): Promise<DehydratedState> {
  if (!isFirstScreenRequest(params, defeatingParams)) {
    return EMPTY_DEHYDRATED_STATE
  }

  const payloads = await Promise.all(
    reads.map(({ url, collection, service }) =>
      fetchListPayload<unknown>({ url, collection, service }),
    ),
  )
  if (payloads.some(payload => payload == null)) {
    return EMPTY_DEHYDRATED_STATE
  }

  return seedFirstScreen(
    reads.map((read, i) => ({ queryKey: read.queryKey, data: payloads[i] })),
  )
}
