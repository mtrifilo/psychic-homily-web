'use client'

import { Loader2, Pencil, X, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SaveButton, AddToCollectionButton } from '@/components/shared'
import type { ShowResponse } from '../types'
import { AttendanceButton } from './AttendanceButton'
import { ReportShowButton } from './ReportShowButton'

interface ShowActionsProps {
  show: ShowResponse
  /** Display name for the show, used by collection/report flows. */
  showTitle: string
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
 * ShowDetail-specific action cluster. Owns the attendance button, the
 * top row of save/collect/report/edit/delete, and the admin-only status
 * toggle row (Mark Sold Out / Mark Cancelled).
 *
 * This is not `EntityHeader.actions` because shows have two extra rows
 * (attendance on its own line; admin status toggles as a sub-row) that
 * would not fit into `EntityHeader`'s single flex-row slot without either
 * cramming or wrapping awkwardly.
 */
export function ShowActions({
  show,
  showTitle,
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
  return (
    <>
      {/* Attendance (Going/Interested) */}
      <AttendanceButton showId={show.id} compact={false} />

      <div className="flex flex-wrap items-center gap-2">
        <SaveButton showId={show.id} variant="outline" size="sm" />
        <AddToCollectionButton
          entityType="show"
          entityId={show.id}
          entityName={showTitle}
        />
        <ReportShowButton showId={show.id} showTitle={showTitle} />

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
