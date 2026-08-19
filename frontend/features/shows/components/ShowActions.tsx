'use client'

import { Loader2, Pencil, X, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { ShowResponse } from '../types'

interface ShowActionsProps {
  show: ShowResponse
  /** Whether the current viewer is an admin. */
  isAdmin: boolean
  /** Whether the current viewer can delete the show (admin OR owner). */
  canDelete: boolean
  /** Whether the current viewer can manage status flags (admin OR owner). */
  canManageStatus: boolean
  /** Whether the inline edit form is currently open. */
  isEditing: boolean
  /** Toggle the inline edit form. */
  onToggleEdit: () => void
  /** Open the delete confirmation dialog. */
  onOpenDelete: () => void
  /** Toggle the sold-out status. */
  onToggleSoldOut: () => void
  /** Toggle the cancelled status. */
  onToggleCancelled: () => void
  /** Pending state of the sold-out mutation. */
  isSoldOutPending: boolean
  /** Pending state of the cancelled mutation. */
  isCancelledPending: boolean
}

/**
 * ShowDetail-specific ADMIN/OWNER action cluster: edit, delete, and the
 * status toggle row (Mark Sold Out / Mark Cancelled).
 *
 * The public verbs that used to live here — save, collect, share, report —
 * moved with the mock (PSY-1686): save/collect/share into ShowTicketRow's
 * bracket row, report into ShowProvenanceLine. What remains is moderation
 * chrome the mock does not draw, kept in the slot under the ticket module.
 *
 * This is not `EntityHeader.actions` because the admin status toggles form a
 * sub-row that would not fit into `EntityHeader`'s single flex-row slot
 * without either cramming or wrapping awkwardly.
 */
export function ShowActions({
  show,
  isAdmin,
  canDelete,
  canManageStatus,
  isEditing,
  onToggleEdit,
  onOpenDelete,
  onToggleSoldOut,
  onToggleCancelled,
  isSoldOutPending,
  isCancelledPending,
}: ShowActionsProps) {
  // ShowDetail's canModerateShow gate decides whether this cluster mounts
  // at all; the per-control props below select WHICH controls render inside
  // it. Two of them (canDelete, canManageStatus) currently equal the mount
  // gate, so their guards are belt-and-braces for the day they diverge. The
  // Edit button's isAdmin gate is DELIBERATELY narrower than the drawer's
  // admin-or-owner rule: this button is moderation chrome, and a non-admin
  // owner reaches the same drawer through the provenance line's [Edit]
  // (user decision on the Wave 1C byline).
  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {isAdmin && (
          <Button
            variant={isEditing ? 'secondary' : 'outline'}
            size="sm"
            onClick={onToggleEdit}
          >
            {isEditing ? (
              <>
                <X className="h-4 w-4 mr-2" />
                Cancel
              </>
            ) : (
              <>
                <Pencil className="h-4 w-4 mr-2" />
                Edit
              </>
            )}
          </Button>
        )}

        {canDelete && (
          <Button
            variant="outline"
            size="sm"
            onClick={onOpenDelete}
            className="text-destructive hover:text-destructive hover:bg-destructive/10"
          >
            <Trash2 className="h-4 w-4 mr-2" />
            Delete
          </Button>
        )}
      </div>

      {/* Status Flag Buttons (Admin or Submitter) */}
      {canManageStatus && (
        <div className="flex items-center gap-2">
          <Button
            variant={show.is_sold_out ? 'secondary' : 'outline'}
            size="sm"
            onClick={onToggleSoldOut}
            disabled={isSoldOutPending}
            className={
              show.is_sold_out
                ? 'bg-orange-100 text-orange-800 hover:bg-orange-200 dark:bg-orange-900/30 dark:text-orange-400 dark:hover:bg-orange-900/50'
                : ''
            }
          >
            {isSoldOutPending ? (
              <Loader2 className="h-4 w-4 mr-2 animate-spin" />
            ) : null}
            {show.is_sold_out ? 'Unmark Sold Out' : 'Mark Sold Out'}
          </Button>
          <Button
            variant={show.is_cancelled ? 'secondary' : 'outline'}
            size="sm"
            onClick={onToggleCancelled}
            disabled={isCancelledPending}
            className={
              show.is_cancelled
                ? 'bg-destructive/10 text-destructive hover:bg-destructive/20'
                : ''
            }
          >
            {isCancelledPending ? (
              <Loader2 className="h-4 w-4 mr-2 animate-spin" />
            ) : null}
            {show.is_cancelled ? 'Unmark Cancelled' : 'Mark Cancelled'}
          </Button>
        </div>
      )}
    </>
  )
}
