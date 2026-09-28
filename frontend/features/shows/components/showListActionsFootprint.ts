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
 * on any of its rows. Taken over the LIST rather than per row, so that every
 * row and the header can be given one actions width.
 */
export function actionsFootprintFor({
  shows,
  isAdmin,
  userId,
}: {
  shows: readonly ShowResponse[]
  isAdmin: boolean
  userId?: string
}): ActionsFootprint {
  const { showAdminActions, showOwnerActions } =
    SHOW_LIST_FEATURE_POLICY.discovery
  if (isAdmin && showAdminActions) return 'admin'
  // The same predicate the row gates its delete control on.
  const canDeleteARow =
    showOwnerActions &&
    shows.some(show =>
      canModerateShow({ submittedBy: show.submitted_by, viewerId: userId, isAdmin })
    )
  return canDeleteARow ? 'owner' : 'viewer'
}
