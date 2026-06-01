'use client'

import { useState, useCallback } from 'react'
import { Loader2, Check, X, type LucideIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'

/**
 * Icon keys for a {@link NotesAction}. Kept as a small closed set (rather than
 * accepting an arbitrary LucideIcon) so the row owns the spinner-swap: when a
 * mutation is in flight the confirm button shows a spinner in place of the
 * action's resting icon, and the resting-vs-spinner choice must live here.
 */
type NotesActionIcon = 'check' | 'x'

const ACTION_ICON: Record<NotesActionIcon, LucideIcon> = {
  check: Check,
  x: X,
}

export interface NotesAction {
  /** Stable discriminator the row echoes back to {@link NotesActionRowProps.onConfirm}. */
  key: string
  /** Resting button label, e.g. "Resolve", "Hide Comment". */
  restingLabel: string
  /** Confirm button label after the notes textarea expands, e.g. "Confirm Hide". */
  confirmLabel: string
  /** Button variant for BOTH the resting and confirm button of this action. */
  variant: 'default' | 'outline' | 'destructive'
  /** Resting + confirm icon (swapped for a spinner while the mutation runs). */
  icon: NotesActionIcon
  /** Placeholder shown in the notes textarea when this action is being confirmed. */
  notesPlaceholder: string
  /** When true the resting button is disabled (e.g. CollectionReport's Hide on a deleted collection). */
  disabled?: boolean
  /** Tooltip for the resting button, typically explaining a disabled state. */
  title?: string
}

export interface NotesActionRowProps {
  /**
   * Exactly two actions. The first is the "primary" (rendered with whatever
   * variant it declares — often destructive); the second is the secondary.
   * A tuple rather than a wider array keeps the model honest: every Model-B
   * card has precisely two actions.
   */
  actions: [NotesAction, NotesAction]
  /**
   * Confirm handler. Receives the chosen action's key plus the trimmed notes
   * (empty string when the admin left the optional textarea blank — callers
   * coalesce to `undefined`/a default themselves, preserving each card's
   * existing semantics).
   */
  onConfirm: (actionKey: string, notes: string) => void
  /**
   * True while ANY of the card's mutations are in flight. Disables every
   * button and drives the confirm-button spinner.
   */
  isActioning: boolean
}

/**
 * Model B action row for moderation cards: two actions (one may be
 * destructive — Resolve/Dismiss, Hide/Dismiss) that each expand an OPTIONAL
 * notes textarea before confirming. The confirm button mirrors the chosen
 * action's variant + icon + label, and notes are passed up trimmed.
 *
 * Used by EntityReportCard + CommentReportCard + CollectionReportCard.
 * Behavior-preserving extraction (PSY-920); preserves CollectionReportCard's
 * disabled-Hide + title affordance via the per-action `disabled`/`title`
 * descriptor fields.
 *
 * Notes capture + the chosen-action state live here because both are
 * intrinsic to this interaction model; the card receives only the final
 * (actionKey, trimmedNotes) pair via onConfirm.
 */
export function NotesActionRow({ actions, onConfirm, isActioning }: NotesActionRowProps) {
  const [showNotes, setShowNotes] = useState(false)
  const [notes, setNotes] = useState('')
  const [activeKey, setActiveKey] = useState<string | null>(null)

  const reset = useCallback(() => {
    setShowNotes(false)
    setNotes('')
    setActiveKey(null)
  }, [])

  const startAction = useCallback((key: string) => {
    setActiveKey(key)
    setShowNotes(true)
  }, [])

  const confirm = useCallback(() => {
    if (activeKey === null) return
    onConfirm(activeKey, notes.trim())
    // No local reset here, mirroring the pre-extraction cards: on success the
    // moderation query invalidates and this row unmounts; on error the
    // expanded view + typed notes intentionally persist so the admin can retry.
  }, [activeKey, notes, onConfirm])

  if (showNotes) {
    const active = actions.find(a => a.key === activeKey) ?? actions[0]
    const ActiveIcon = ACTION_ICON[active.icon]
    return (
      <div className="mt-3 space-y-2">
        <textarea
          value={notes}
          onChange={e => setNotes(e.target.value)}
          placeholder={active.notesPlaceholder}
          className="w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
          rows={2}
          autoFocus
        />
        <div className="flex items-center gap-2">
          <Button size="sm" variant={active.variant} onClick={confirm} disabled={isActioning}>
            {isActioning ? (
              <Loader2 className="h-3 w-3 animate-spin mr-1" />
            ) : (
              <ActiveIcon className="h-3 w-3 mr-1" />
            )}
            {active.confirmLabel}
          </Button>
          <Button size="sm" variant="ghost" onClick={reset} disabled={isActioning}>
            Cancel
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="mt-3 flex items-center gap-2">
      {actions.map(action => {
        const Icon = ACTION_ICON[action.icon]
        return (
          <Button
            key={action.key}
            size="sm"
            variant={action.variant}
            onClick={() => startAction(action.key)}
            disabled={isActioning || action.disabled}
            title={action.title}
          >
            <Icon className="h-3 w-3 mr-1" />
            {action.restingLabel}
          </Button>
        )
      })}
    </div>
  )
}
