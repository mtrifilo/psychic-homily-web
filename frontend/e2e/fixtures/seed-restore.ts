import { runE2ESql } from '../e2e-db'

/**
 * PSY-1663 — restore the admin-workflow seed rows to a known state.
 *
 * The admin specs verify a venue and approve/reject shows. Those mutations are
 * one-way: nothing in the product un-verifies a venue or moves a show back to
 * `pending`, and the `/admin/test-fixtures/reset` endpoint only DELETES rows
 * owned by a seeded worker user — neither of which restores an admin fixture.
 * So a spec that failed after mutating left the next attempt asserting against
 * state that could no longer occur, and the retry was guaranteed to fail:
 * exactly the "transient failure becomes a hard one" shape this ticket exists
 * to remove.
 *
 * These helpers put the row back at test ENTRY rather than cleaning up at
 * exit, because entry is the only point that is reached on every attempt. On a
 * clean database each call is a no-op write — attempt 1 still exercises the
 * real flow against the real seed state, so nothing here can mask a product
 * regression.
 *
 * The seeds themselves are created by e2e/setup-db.sh ("Inserting admin
 * workflow test data"); the slugs below are the contract between the two
 * files. Every value interpolated into SQL here is a module constant or a
 * value from a closed union — none of it is test-derived input.
 */

export const UNVERIFIED_VENUE_SEED_SLUG = 'e2e-unverified-venue'
export const PENDING_SHOW_APPROVE_SEED_SLUG = 'e2e-pending-show-approve'
export const PENDING_SHOW_REJECT_SEED_SLUG = 'e2e-pending-show-reject'

/** The `shows.status` values the admin pending-shows flow moves between. */
export type PendingShowSeedStatus = 'pending' | 'approved' | 'rejected'

/** Only the two seeded admin fixtures may be moved by this module. */
export type PendingShowSeedSlug =
  | typeof PENDING_SHOW_APPROVE_SEED_SLUG
  | typeof PENDING_SHOW_REJECT_SEED_SLUG

/**
 * updateExactlyOneSeedRow runs an UPDATE that must match exactly one row, and
 * fails loudly otherwise. A missing seed row would otherwise surface much
 * later as an unexplained "element not visible" timeout in the spec; here it
 * names the row and points at the two things that can remove it.
 */
function updateExactlyOneSeedRow(update: string, describe: string): void {
  const affected = runE2ESql(
    `WITH updated AS (${update} RETURNING 1) SELECT count(*) FROM updated`,
  )
  if (affected !== '1') {
    throw new Error(
      `[PSY-1663] restoring ${describe} affected ${affected} rows, expected 1. ` +
        `The seed is created by e2e/setup-db.sh; a count of 0 means the row is ` +
        `missing (a failed seed, or a fixture reset that deleted it).`,
    )
  }
}

/**
 * restoreUnverifiedVenueSeed puts the verify-venue seed back to unverified.
 *
 * `VerifyVenue` (services/catalog/venue.go) sets `verified` and, when the
 * venue has no slug, generates one. The seed ships with a slug, so `verified`
 * is the only column that moves.
 */
export function restoreUnverifiedVenueSeed(): void {
  updateExactlyOneSeedRow(
    `UPDATE venues SET verified = false, updated_at = NOW()
       WHERE slug = '${UNVERIFIED_VENUE_SEED_SLUG}'`,
    `venue seed '${UNVERIFIED_VENUE_SEED_SLUG}'`,
  )
}

/**
 * setPendingShowSeedStatus forces one admin pending-show seed to a status.
 *
 * Callers state the precondition each test needs rather than assuming the
 * previous test left the right one behind, so a test is recoverable on retry
 * AND runnable on its own. `rejection_reason` moves with the status because
 * ApproveShow clears it and RejectShow sets it (services/catalog/show.go); the
 * seed leaves it NULL.
 */
export function setPendingShowSeedStatus(
  slug: PendingShowSeedSlug,
  status: PendingShowSeedStatus,
): void {
  updateExactlyOneSeedRow(
    `UPDATE shows SET status = '${status}', rejection_reason = NULL, updated_at = NOW()
       WHERE slug = '${slug}'`,
    `pending-show seed '${slug}' -> ${status}`,
  )
}
