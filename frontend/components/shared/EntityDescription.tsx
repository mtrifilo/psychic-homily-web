'use client'

import { useState } from 'react'
import { Pencil, Loader2, X, Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

const MAX_DESCRIPTION_LENGTH = 5000

/**
 * Shown in the error slot after a save was refused because the description moved
 * under the editor. It reports the replacement rather than reassuring: the draft
 * the user typed is gone, and the only way forward is to make the change again
 * on top of the text now in the box.
 */
const RESEEDED_NOTE =
  'This description changed while you were editing. Your draft was replaced with the current text, so make your change again and save.'

interface EntityDescriptionProps {
  description: string | null | undefined
  canEdit: boolean
  /**
   * previousDescription is the value THIS editor was composed against: what it
   * was seeded with, or what a conflict re-seeded it with. Callers that submit
   * an edit through the contribution pipeline send it as `old_value`, so the
   * claim describes the box the user was looking at rather than whatever the
   * surrounding query happens to hold when Save is pressed.
   */
  onSave: (description: string, previousDescription: string) => Promise<void>
  /**
   * Reads a rejected save for the value the field holds NOW. A string replaces
   * the draft with it and reports the replacement in the error slot; undefined
   * leaves the draft alone and shows the rejection's own message. Omitted, the
   * editor never re-seeds.
   */
  currentValueOnConflict?: (error: unknown) => string | undefined
}

/**
 * Render a description as simple paragraphs.
 * Splits on double newlines for paragraph breaks, preserves single newlines as <br>.
 */
function DescriptionContent({ text }: { text: string }) {
  const paragraphs = text.split(/\n\n+/)

  return (
    <div className="space-y-3">
      {paragraphs.map((paragraph, i) => {
        const lines = paragraph.split('\n')
        return (
          <p key={i} className="text-sm text-muted-foreground leading-relaxed">
            {lines.map((line, j) => (
              <span key={j}>
                {j > 0 && <br />}
                {line}
              </span>
            ))}
          </p>
        )
      })}
    </div>
  )
}

export function EntityDescription({
  description,
  canEdit,
  onSave,
  currentValueOnConflict,
}: EntityDescriptionProps) {
  const [isEditing, setIsEditing] = useState(false)
  const [editValue, setEditValue] = useState('')
  // The value the open editor is composed against. Seeded from the prop when
  // editing starts and replaced on a conflict, so it stays the text the user is
  // looking at rather than tracking the prop, which a background refetch moves.
  const [baseline, setBaseline] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const hasDescription = !!description && description.trim().length > 0

  const handleStartEdit = () => {
    setEditValue(description || '')
    setBaseline(description || '')
    setError(null)
    setIsEditing(true)
  }

  const handleCancel = () => {
    setIsEditing(false)
    setEditValue('')
    setBaseline('')
    setError(null)
  }

  const handleSave = async () => {
    if (editValue.length > MAX_DESCRIPTION_LENGTH) {
      setError(`Description must be ${MAX_DESCRIPTION_LENGTH} characters or fewer`)
      return
    }

    setIsSaving(true)
    setError(null)

    try {
      await onSave(editValue.trim(), baseline)
      setIsEditing(false)
      setEditValue('')
      setBaseline('')
    } catch (err) {
      // A refusal that reports the current value re-seeds the editor with it,
      // so the next Save is composed against a value the server actually holds
      // rather than repeating a claim it has already rejected.
      const current = currentValueOnConflict?.(err)
      if (current !== undefined) {
        setEditValue(current)
        setBaseline(current)
        setError(RESEEDED_NOTE)
      } else {
        setError(
          err instanceof Error ? err.message : 'Failed to save description'
        )
      }
    } finally {
      setIsSaving(false)
    }
  }

  // Edit mode
  if (isEditing) {
    return (
      <div className="space-y-3">
        <Textarea
          value={editValue}
          onChange={(e) => setEditValue(e.target.value)}
          placeholder="Add a description..."
          rows={6}
          maxLength={MAX_DESCRIPTION_LENGTH}
          disabled={isSaving}
          className="resize-y text-sm"
        />
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">
            {editValue.length.toLocaleString()} / {MAX_DESCRIPTION_LENGTH.toLocaleString()}
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleCancel}
              disabled={isSaving}
            >
              <X className="h-4 w-4 mr-1" />
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleSave}
              disabled={isSaving}
            >
              {isSaving ? (
                <Loader2 className="h-4 w-4 mr-1 animate-spin" />
              ) : (
                <Check className="h-4 w-4 mr-1" />
              )}
              Save
            </Button>
          </div>
        </div>
        {/* role="alert" because a re-seed rewrites the textarea underneath the
            cursor: a reader who cannot see the box change has to be told. */}
        {error && (
          <p role="alert" className="text-xs text-destructive">{error}</p>
        )}
      </div>
    )
  }

  // Display mode with description
  if (hasDescription) {
    return (
      <div className="group relative">
        <DescriptionContent text={description!} />
        {canEdit && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleStartEdit}
            className="mt-2 h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
          >
            <Pencil className="h-3 w-3 mr-1" />
            Edit description
          </Button>
        )}
      </div>
    )
  }

  // Empty state
  if (canEdit) {
    return (
      <button
        onClick={handleStartEdit}
        className="w-full rounded-md border border-dashed border-muted-foreground/25 bg-muted/30 p-4 text-left transition-colors hover:bg-muted/50"
      >
        <p className="text-sm text-muted-foreground">
          No description yet.{' '}
          <span className="text-primary hover:underline">Add description</span>
        </p>
      </button>
    )
  }

  // No description, can't edit — show nothing
  return null
}
