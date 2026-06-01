'use client'

import { useState, useCallback } from 'react'
import { Loader2, Check, X } from 'lucide-react'

import { Button } from '@/components/ui/button'

export interface RejectWithReasonRowProps {
  /** Approve handler — fires immediately on click (no confirmation step). */
  onApprove: () => void
  /**
   * Reject handler. Receives the trimmed, non-empty reason. The row guards
   * the empty case itself (Confirm Reject stays disabled until the reason has
   * non-whitespace content), so callers never see an empty string.
   */
  onReject: (reason: string) => void
  /**
   * True while EITHER the approve or reject mutation is in flight. Disables
   * every button so an admin can't double-fire while a request is pending.
   */
  isActioning: boolean
  /**
   * True while specifically the APPROVE mutation is in flight. Drives the
   * spinner on the Approve button (so the spinner only appears on the action
   * that's actually running, matching the pre-extraction behavior).
   */
  isApproving: boolean
  /**
   * True while specifically the REJECT mutation is in flight. Drives the
   * spinner on the Confirm Reject button.
   */
  isRejecting: boolean
  /**
   * Placeholder for the rejection-reason textarea. Card-specific copy
   * (PendingEditCard uses a longer "be specific" prompt; PendingCommentCard
   * uses the terse "Rejection reason (required)").
   */
  rejectPlaceholder: string
}

/**
 * Model A action row for moderation cards: an approve-immediate primary
 * action plus a reject action that expands a REQUIRED-reason textarea before
 * confirming.
 *
 * Used by PendingEditCard + PendingCommentCard. Behavior-preserving
 * extraction (PSY-920) of the two cards' formerly-inline, identically-shaped
 * action rows — the only per-card variation was the reject placeholder copy
 * and which mutation's pending flag drives which spinner, both now props.
 *
 * Reason capture lives here (local state) rather than in the card because the
 * textarea is intrinsic to this interaction model; the card only needs the
 * final trimmed reason via onReject.
 */
export function RejectWithReasonRow({
  onApprove,
  onReject,
  isActioning,
  isApproving,
  isRejecting,
  rejectPlaceholder,
}: RejectWithReasonRowProps) {
  const [rejecting, setRejecting] = useState(false)
  const [rejectionReason, setRejectionReason] = useState('')

  const cancel = useCallback(() => {
    setRejecting(false)
    setRejectionReason('')
  }, [])

  const confirmReject = useCallback(() => {
    const trimmed = rejectionReason.trim()
    if (!trimmed) return
    onReject(trimmed)
    // Reset is intentional whether or not the mutation ultimately succeeds:
    // the card's own onSuccess (which unmounts/clears the row) is the
    // authoritative reset; this keeps the local input from lingering if the
    // row stays mounted (e.g. error path leaves the resting buttons).
  }, [rejectionReason, onReject])

  if (rejecting) {
    return (
      <div className="mt-3 space-y-2">
        <textarea
          value={rejectionReason}
          onChange={e => setRejectionReason(e.target.value)}
          placeholder={rejectPlaceholder}
          className="w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
          rows={2}
          autoFocus
        />
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="destructive"
            onClick={confirmReject}
            disabled={!rejectionReason.trim() || isActioning}
          >
            {isRejecting ? (
              <Loader2 className="h-3 w-3 animate-spin mr-1" />
            ) : (
              <X className="h-3 w-3 mr-1" />
            )}
            Confirm Reject
          </Button>
          <Button size="sm" variant="ghost" onClick={cancel} disabled={isActioning}>
            Cancel
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="mt-3 flex items-center gap-2">
      <Button size="sm" onClick={onApprove} disabled={isActioning}>
        {isApproving ? (
          <Loader2 className="h-3 w-3 animate-spin mr-1" />
        ) : (
          <Check className="h-3 w-3 mr-1" />
        )}
        Approve
      </Button>
      <Button
        size="sm"
        variant="outline"
        onClick={() => setRejecting(true)}
        disabled={isActioning}
      >
        <X className="h-3 w-3 mr-1" />
        Reject
      </Button>
    </div>
  )
}
