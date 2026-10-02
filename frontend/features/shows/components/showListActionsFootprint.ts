import type { UserIdLike } from '@/features/auth/authUser'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'
import { canModerateShow } from '../utils'
import type { ShowResponse } from '../types'

/**
 * The trailing controls a viewer can see anywhere on one discovery list:
 * `viewer` is expand, save and outbound; `owner` adds the delete control;
 * `admin` adds edit, export where it renders, and delete.
 */
export type ActionsFootprint = 'viewer' | 'owner' | 'admin'

/**
 * Which viewer-dependent controls (admin, delete) one discovery row renders.
 * The row reads this to render them and `actionsFootprintFor` reads it to size
 * the column, so the two agree about those controls on every row.
 */
export function rowActionControls({
  submittedBy,
  viewerId,
  isAdmin,
}: {
  submittedBy: ShowResponse['submitted_by']
  viewerId: UserIdLike
  isAdmin: boolean
}): { admin: boolean; delete: boolean } {
  const { showAdminActions, showOwnerActions } =
    SHOW_LIST_FEATURE_POLICY.discovery
  return {
    admin: showAdminActions && isAdmin,
    delete:
      showOwnerActions && canModerateShow({ submittedBy, viewerId, isAdmin }),
  }
}

/**
 * The footprint for a whole list: the widest set of controls the viewer can see
 * on any of its rows, so every row and the header can share one actions width.
 *
 * `userId` must be the same id the row passes to `rowActionControls`.
 */
export function actionsFootprintFor({
  shows,
  isAdmin,
  userId,
}: {
  shows: readonly ShowResponse[]
  isAdmin: boolean
  userId?: UserIdLike
}): ActionsFootprint {
  // The admin controls do not depend on who submitted a row.
  const anyRow = { submittedBy: undefined, viewerId: userId, isAdmin }
  if (rowActionControls(anyRow).admin) return 'admin'
  const canDeleteARow = shows.some(
    show =>
      rowActionControls({
        submittedBy: show.submitted_by,
        viewerId: userId,
        isAdmin,
      }).delete
  )
  return canDeleteARow ? 'owner' : 'viewer'
}
