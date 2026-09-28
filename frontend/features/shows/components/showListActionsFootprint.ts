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
 * The footprint for a whole list: the widest set of controls the viewer can see
 * on any of its rows, so every row and the header can share one actions width.
 *
 * `userId` must be the same id the list hands each row, which is what the row
 * checks for its delete control.
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
  const { showAdminActions, showOwnerActions } =
    SHOW_LIST_FEATURE_POLICY.discovery
  if (isAdmin && showAdminActions) return 'admin'
  const canDeleteARow =
    showOwnerActions &&
    shows.some(show =>
      canModerateShow({ submittedBy: show.submitted_by, viewerId: userId, isAdmin })
    )
  return canDeleteARow ? 'owner' : 'viewer'
}
