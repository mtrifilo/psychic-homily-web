'use client'

/**
 * Venue confirm-current (PSY-1542).
 *
 * One tap that says "this listing is still accurate" and edits nothing — the
 * cheapest contribution the app offers, and the mechanic that makes a
 * crowdsourced venue map cheap enough to maintain.
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, type ApiError } from '@/lib/api'
import { venueEndpoints, venueQueryKeys } from '@/features/venues/api'
import type { VenueConfirmationResponse } from '../types'

/**
 * Confirm a venue's info is current.
 *
 * The write is idempotent server-side (composite-PK insert, ON CONFLICT DO
 * NOTHING), so there is no optimistic-update rollback to manage and no
 * "already confirmed" error to special-case: a repeat tap returns the same
 * aggregate with 200. Callers render from the returned counts.
 *
 * On success every cached venue query is invalidated so the provenance stamps
 * on the rail and the venue page pick up the new confirmation count. This is
 * deliberately broad rather than surgical: the same venue appears under several
 * list keys (per city, per filter set, with and without rail fields), and
 * patching each in place would silently miss one and leave two stamps
 * disagreeing about the same venue on the same screen.
 */
export function useVenueConfirm() {
  const queryClient = useQueryClient()

  return useMutation<VenueConfirmationResponse, ApiError, number>({
    mutationFn: async (venueId: number) =>
      apiRequest<VenueConfirmationResponse>(venueEndpoints.CONFIRM(venueId), {
        method: 'POST',
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: venueQueryKeys.all })
    },
  })
}

/**
 * Turn a failed confirm into one line a person can act on.
 *
 * The 429 branch is the one that matters: the confirm endpoint sits on the
 * shared engagement-mutation budget, and a silent failure there would read as
 * "the button is broken" rather than "slow down". `ApiError.retryAfter` is
 * parsed from the response's `Retry-After` header by `lib/api.ts`.
 *
 * There is no toast library in this codebase — the caller renders the returned
 * string inline, beside the control that failed.
 */
export function formatVenueConfirmError(error: unknown): string | null {
  if (!error) return null
  const apiErr = error as ApiError

  if (apiErr.status === 429) {
    if (apiErr.retryAfter && Number.isFinite(apiErr.retryAfter)) {
      return `Too many confirmations — try again in ${apiErr.retryAfter}s.`
    }
    return 'Too many confirmations — try again in a minute.'
  }
  if (apiErr.status === 401) {
    return 'Sign in to confirm this venue.'
  }
  if (apiErr.status === 404) {
    return 'This venue no longer exists.'
  }
  return 'Couldn’t confirm this venue. Please try again.'
}
