import type { DehydratedState } from '@tanstack/react-query'
import { connection } from 'next/server'
import { seedFirstScreen } from '@/lib/query-hydration'
import {
  fetchListPayload,
  type ListPayloadCollection,
} from '@/lib/ssr/fetchListPayload'
import {
  isFirstScreenRequest,
  type RouteSearchParams,
} from '@/lib/ssr/firstScreenParams'

/** What `dehydrate()` produces for an empty client: hydrates nothing. */
export const EMPTY_DEHYDRATED_STATE: DehydratedState = {
  mutations: [],
  queries: [],
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
 * The dehydrated first screen of a list on a URL that has one, or an empty
 * state when the URL's params move the list off it (see `isFirstScreenRequest`).
 *
 * A URL off the first screen neither reads nor pays for `connection()`.
 */
export async function dehydrateListFirstScreen(
  params: RouteSearchParams,
  defeatingParams: readonly string[],
  reads: readonly FirstScreenRead[],
): Promise<DehydratedState> {
  if (!isFirstScreenRequest(params, defeatingParams)) {
    return EMPTY_DEHYDRATED_STATE
  }
  return dehydrateFirstScreen(reads)
}

/**
 * The dehydrated first screen of a list, or an empty state when there is none
 * to give. For a route whose every request is the first screen; a route with
 * URL params that move the list goes through `dehydrateListFirstScreen`.
 *
 * `connection()` comes before the reads, so they run per request and never
 * during a prerender, whatever they return (see `seedFirstScreen`).
 *
 * All or nothing: when any read returns `null`, nothing is seeded and the
 * client fetches every entry itself, so the server HTML is either the whole
 * first screen or the list's own loader, never a mixture of seeded rows and
 * placeholder facets.
 */
export async function dehydrateFirstScreen(
  reads: readonly FirstScreenRead[],
): Promise<DehydratedState> {
  await connection()
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
